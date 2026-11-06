## Context

上游报文的当前状态（已核实）：

- 网关与上游之间只有一个统一 HTTP 出口：`relay/channel/api_request.go:523 doRequest`，唯一一次 `relayClient.Do(req)` 在 `:565`。`DoApiRequest`（`:313`）、`DoFormRequest`（`:343`）、`DoTaskApiRequest`（`:592`）以及导出的 `DoRequest`（`:513`）都收敛到它。
- 该出口使用的 client 来自 `service.GetHttpClientWithProxySettings`（`doRequest:526`），是**跨渠道缓存复用的共享 client**；`doRequest` 只做浅拷贝并覆盖 `CheckRedirect`（`:534-535`）。
- 请求体由各 handler 序列化后传入，类型可能是 `relaycommon.NewOutboundJSONBody` 或可重放的 `common.BodyStorage`/`ReplayableBody`；`ApplyUpstreamBodyMetadata`（`:31-49`）为可重放 body 补齐 `ContentLength` 与 `GetBody`。
- 响应体没有收口：全仓约 69 处 `io.ReadAll(resp.Body)` 分散在各渠道，流式则在 `relay/helper/stream_scanner.go:98 NewStreamScanner(resp.Body)` 内被消费。
- 日志写入走日志库句柄 `LOG_DB`（`model/main.go:68`、`model/log.go:101`），日志库可能配置为 ClickHouse；`Log.RequestId`（`model/log.go:76`）是消费日志与上游报文的天然关联键，由 `relay/common/relay_info.go:607-617` 生成、`model/log.go:93 ensureLogRequestId` 仅在空值时兜底。
- 设置项的新写法是 `setting/config` 注册结构体（范例 `setting/operation_setting/log_setting.go`）；前端系统设置经 `GET/PUT /api/option/` 读写（`web/src/features/system-settings/`）。

## Goals / Non-Goals

**Goals**

- 用**一个**采集挂点覆盖尽可能多的渠道与两种响应形态（非流式 + 流式），而不是逐渠道埋点。
- 采集对转发零影响：不改发往上游的字节，不在默认关闭时引入可观测开销。
- 报文与消费日志靠 `request_id` 一一对应，且随日志的保留期一起清理。

**Non-Goals**

- 不采集上游请求头/响应头（Authorization、API Key 在其中）。
- 不改造 `StreamScannerHandler` 的回调协议，也不动那 12 处直接使用 `NewStreamScanner` 的渠道。
- 不为绕过统一出口的渠道（AWS/Bedrock SDK、replicate 轮询、gemini 局部请求、vertex OAuth、coze/dify 内部请求）造第二套采集机制。
- 不提供按渠道粒度的开关，不做采样。

## Decisions

### D1: 采集挂点选在 `doRequest` 内部包装 body 流，不做 RoundTripper、也不逐 handler 埋点

在 `doRequest` 的 `relayClient.Do(req)` 前后各包一层 tee：

- 请求侧：`req.Body` 换成 tee ReadCloser，转发时顺带累积实际发出的字节，不改字节也不改 `ContentLength`/`GetBody` 语义。
- 响应侧：`resp.Body` 换成 tee ReadCloser，handler 读取时顺带累积；`EOF` 或 `Close()` 时收尾并落库（`doRequest:587` 或调用方的 `defer resp.Body.Close()` 都会触发）。
- 开关关闭时直接跳过包装，零开销。

理由：`doRequest` 是唯一出口，一处覆盖全部走 HTTP 的渠道与流式/非流式两条路径；而共享 client 的 Transport 属于所有调用方，注入采集器会污染连接池并与既有 `shardedRoundTripper` 叠加。

备选：a) 自定义 `http.RoundTripper` 挂在共享 client 的 Transport 上 —— 放弃，共享连接池污染，且开启/关闭开关需要重建缓存 client；b) 在各 handler 序列化点 + 69 处 `io.ReadAll` 处埋点 —— 放弃，改动面 90 处以上，流式路径还在 `NewStreamScanner` 内部无法覆盖。

### D2: 流式原文直接取响应侧 tee 的字节，不改 `StreamScannerHandler`

`StreamScannerHandler`（`relay/helper/stream_scanner.go:83`）的回调签名为 `func(data string, sr *StreamResult)`，它只把 `data:` 之后的 payload 交给回调并丢弃 `event:` 行（证据：`relay/channel/openai/relay_image.go:139-141` 的注释「The scanner delivers only the "data:" payload, so the SSE "event:" line is rebuilt from the JSON "type" field」）。响应侧 tee 在 scanner 之前，拿到的是含 `data:`/`event:` 的原始 SSE 字节，天然满足「原文形态」要求，对 12 处直接 `NewStreamScanner` 的渠道同样生效。

代价：落库不再依赖 scanner 回调，而依赖 `resp.Body` 被读完或关闭。所有调用点都是 `defer resp.Body.Close()` 模式，因此正常与异常路径都能收尾。

备选：给 `StreamScannerHandler` 加一个 raw 帧参数 —— 放弃，要同时改 12 个调用点，且回调协议变更会传导到全部流式渠道。

### D3: 一次请求一条记录，键为 `RequestId`，在响应收尾时同步落库

采集器在 `doRequest` 建立时从 `info.RequestId` 取键（`relay/common/relay_info.go:154-155` 定义，`:607-617` 生成）。它与消费日志行写入的 `Log.RequestId` 同源；`ensureLogRequestId` 只在空值时兜底，正常链路两者相等。

落库时机是响应 body 收尾（EOF 或 Close），此时 body 已完整或已达上限；写入**同步执行并忽略错误**，与既有 `createLog` 同步写日志一致，不引入队列或后台 goroutine（避免进程退出丢数据与协程生命周期管理）。

备选：异步写入 —— 放弃，需要额外的队列与退出排空逻辑，而日志本身也是同步写。

### D4: 新表放日志库，字段与迁移按三库 + ClickHouse 双分支

表 `upstream_payloads`：

| 字段 | 说明 |
| --- | --- |
| `Id` | GORM 生成主键，不使用 `AUTO_INCREMENT`/`SERIAL` |
| `RequestId` | `gorm:"size:64;uniqueIndex;not null"`，与 `Log.RequestId` 同长度同值 |
| `CreatedAt` | `int64` + 索引，与 `Log.CreatedAt` 同类型，供保留期清理 |
| `RequestBody` / `ResponseBody` | 复用既有 `model.LongText`（`model/task_plugin.go:69-76`：MySQL→`longtext`，PG/SQLite→`text`，无长度，AutoMigrate 幂等） |
| `RequestTruncated` / `ResponseTruncated` | 截断标志 |

迁移挂在 `model/main.go:404-412 migrateLOGDB()`，仿 `model/audit_log.go:251-263 MigrateAuditLogs` 的双分支：非 ClickHouse 用 `LOG_DB.AutoMigrate`，ClickHouse 用 `CREATE TABLE IF NOT EXISTS ... ENGINE = MergeTree() ORDER BY (created_at, request_id)`，并按 `clickHouseLogTTLDays()`/`clickHouseLogTTLClause()` 的既有写法带上 TTL。

同一 `request_id` 只保留一条：写入按 `RequestId` 覆盖（先删同键再插入，或 `OnConflict` upsert）。

备选：报文入主库 —— 放弃，使用日志在日志库，跨库关联与统一清理都要应用层补，得不偿失。

### D5: 大小上限用常量，超限截断但不停止转发

请求体上限 256 KiB，响应体上限 1 MiB。达到上限后停止累积（后续字节照常转发给调用方），并置对应 `*_truncated` 标志。tee 只在上限内累积，因此长流不会随响应长度增长内存。

备选：不设上限 —— 放弃，单条流式响应可无界，直接打爆日志库；把上限做成设置项 —— 暂不做，常量后续易调。

### D6: 开关走 `setting/config` 注册结构体，默认关闭

在 `setting/operation_setting/log_setting.go` 的 `LogSetting` 加 `CaptureUpstreamPayload bool`（json tag `capture_upstream_payload`，默认 `false`），经既有 `config.GlobalConfig.Register("log_setting", ...)` 持久化，前端以 `log_setting.capture_upstream_payload` 为键写入。

备选：老式 `common.XxxEnabled` 包级变量 + `model/option.go` 双向登记 —— 放弃，新设置应走注册结构体模式。

### D7: 报文用独立的管理员接口按需查询，不并入日志列表

新增 `GET /api/log/upstream_payload?request_id=<id>`，走 `AdminAuth`，返回报文与截断标志；无记录时返回成功且字段为空（前端显示空占位）。

备选：把报文并入 `/api/log` 列表响应 —— 放弃，列表体积与序列化开销随报文大小暴涨，而面板是默认收起的。

### D8: 前端在日志详情弹窗新增两个仅 admin 可见的面板，复用既有 `CodeBlock`

`web/src/features/usage-logs/components/dialogs/details-dialog.tsx` 新增「上游请求体」「上游响应体」两个 `DetailSection`（沿用 `./log-detail-layout` 的 `DetailSection`/`DetailRow`），仅 `props.isAdmin` 为真时渲染，默认收起、展开时才请求 D7 接口。内容用既有 `CodeBlock`（`web/src/components/ai-elements/code-block.tsx:469`，项目已装 shiki），JSON 用 `json` 语言高亮，流式原文用纯文本；用 `collapsedLines`/`maxExpandedLines` 限制首屏高度。截断时在面板内显示一行提示。

备选：复用 `RawResponseCollapsible`（`web/src/features/channels/components/dialogs/usage/raw-response-collapsible.tsx`）—— 放弃，它只渲染无高亮的 `<pre>`，与「JSON 格式化才好看」的诉求不符。

### D9: 保留期复用消费日志清理路径

`service/system_task.go:338 runLogCleanupTask` 已按 `model.CountOldLog`（`:359`）与 `model.DeleteOldLogBatch`（`:379`）分批删除过期日志。报文表用同一 `TargetTimestamp` 截止时间删除同期记录（配套的 count/batch 删除函数）；ClickHouse 分支由 D4 的建表 TTL 承担，并同步 TTL 变更。

备选：给报文表单独的保留期设置 —— 放弃，多一个用户需要理解的旋钮，且两者数据生命周期本就一致。

## Risks / Trade-offs

- [报文以明文落库，含 prompt 与模型输出，不做脱敏] → 默认关闭；仅 `AdminAuth` 接口可读；随日志保留期清理；在 proposal 与 AI 项目文档中明确该风险由使用者承担。
- [同一 `request_id` 可能对应多次上游重试，唯一键导致只保留最后一次] → 接受该取舍，排查重试链路时以最终尝试为准；不为此改成多行表。
- [响应体 tee 对大流占内存] → 达到上限即停止累积，转发不受影响。
- [绕过统一出口的渠道无记录] → 属 spec 已声明的豁免；面板显示空占位而非报错。
- [响应 body 被上游压缩时，记录的是 transport 之后、handler 实际读到的字节] → 这正是排查需要的形态；不做额外解压或还原处理。
- [异常路径下 body 未读完即 Close，记录内容不完整且无截断标志] → 记录的是已读部分；面板为空占位与不完整无法严格区分，接受。
- [报文表放大日志库体积] → 默认关闭 + 随日志清理；上限常量进一步限制单条体积。
- [MySQL 上 `type:text` 容量不足] → 复用 `model.LongText`（MySQL `longtext`），并在三库实测。
- [在 body 上套 tee 可能破坏流式时序或重试语义] → tee 只做透传读取，Close 语义转发给底层一次；`GetBody`/`ContentLength` 不变；流式与非流式回归测试覆盖。

## Migration Plan

1. 新表由启动迁移自动创建，覆盖 SQLite、MySQL、PostgreSQL 与 ClickHouse 分支。
2. 开关默认 `false`，功能不开启时无任何行为变化，无需数据回填。
3. 回滚：关闭开关即停止采集与展示；表可保留，亦可手工删除。

## Open Questions

- 上限常量的具体数值（256 KiB / 1 MiB）是否合适，可在审视时调整，不影响方案与任务拆分。

## 1. 数据模型与迁移

- [x] 1.1 新增 `model/upstream_payload.go`：定义 `UpstreamPayload` 模型（`Id`、`RequestId size:64 uniqueIndex not null`、`CreatedAt int64+index`、`RequestBody`/`ResponseBody` 使用既有 `model.LongText`、`RequestTruncated`/`ResponseTruncated`），以及 `SaveUpstreamPayload`（按 `RequestId` 覆盖写）、`GetUpstreamPayloadByRequestId`、`CountOldUpstreamPayload`、`DeleteOldUpstreamPayloadBatch`；验证：`go build ./...` 通过，且四个函数均被后续任务实际调用（无死代码）。 <!-- comet-task:c6f53926-8d94-4af2-94cb-d66deb696672 -->
- [x] 1.2 新增 `MigrateUpstreamPayloads()` 并注册进 `model/main.go` 的 `migrateLOGDB()`：非 ClickHouse 分支走 `LOG_DB.AutoMigrate`，ClickHouse 分支按 `model/audit_log.go:251-263` 的写法执行 `CREATE TABLE IF NOT EXISTS upstream_payloads (...) ENGINE = MergeTree() ORDER BY (created_at, request_id)` 并带上与日志表同一套 `LOG_SQL_CLICKHOUSE_TTL_DAYS` TTL 子句；验证：`model/clickhouse_log_test.go` 中新增断言，确认 CH 分支生成含 `ENGINE = MergeTree()` 与 TTL 的建表语句、非 CH 分支不执行 raw SQL。 <!-- comet-task:3357d2e3-ec2e-4cda-be36-453d075488a1 -->

## 2. 采集器与统一出口接入

- [x] 2.1 新增 `service/upstream_payload_capture.go`：实现 tee ReadCloser（读时透传并把字节累积到上限，`Close` 只向底层转发一次）、请求体 256 KiB / 响应体 1 MiB 上限、达到上限置 truncated 标志后停止累积但继续透传、响应收尾（EOF 或 `Close`）时以 `info.RequestId` 同步落库并忽略写入错误，且只在开关开启时构造；验证：`service/upstream_payload_capture_test.go` 断言透传字节与原文逐字节相同、超限内容被截断且标志为真、未超限不置标志、同 `request_id` 二次写入后只有一条记录、开关关闭时不产生任何记录。 <!-- comet-task:134c5dea-c98c-4530-94d8-162f4dd01794 -->
- [x] 2.2 在 `relay/channel/api_request.go` 的 `doRequest` 内接入采集器：开关开启时分别包装 `req.Body` 与 `resp.Body`，不修改请求字节、不改动 `ContentLength` 与 `GetBody` 语义，也不替换共享 client 或其 `CheckRedirect`；验证：既有 `relay/channel/api_request_test.go`、`api_request_getbody_test.go`、`api_request_redirect_test.go` 全部通过，并新增用例断言开关开启前后发往上游的请求字节完全一致。 <!-- comet-task:79572b04-65c8-4043-b53c-47c2e2d03f07 -->
- [x] 2.3 在 `service/system_task.go` 的 `runLogCleanupTask` 中，用同一 `TargetTimestamp` 追加报文清理（先 `CountOldUpstreamPayload`，再分批 `DeleteOldUpstreamPayloadBatch`，批大小沿用 `logCleanupBatchSize`）；验证：新增用例断言清理执行后 `created_at` 早于截止时间的报文被删除、晚于截止时间的保留。 <!-- comet-task:84665981-5d4f-4e7d-a7fa-e38c5c04f09a -->

## 3. 全局开关

- [x] 3.1 在 `setting/operation_setting/log_setting.go` 的 `LogSetting` 中新增 `CaptureUpstreamPayload bool`（json tag `capture_upstream_payload`，默认 `false`）；验证：`go test ./setting/...` 通过，且 `GET /api/option/` 返回 `log_setting.capture_upstream_payload` 为 `false`。 <!-- comet-task:fc534d04-ff0e-4724-b6aa-baff52bf1217 -->
- [x] 3.2 在 `web/src/features/system-settings/maintenance/log-settings-section.tsx` 增加该开关（沿用既有 `SettingsSwitchItem` + `Switch` 与 `useUpdateOption`）；验证：`bun run typecheck` 通过，且开关切换后 `PUT /api/option/` 提交键名与 3.1 的 json tag 完全一致。 <!-- comet-task:8e54e1d9-2fb4-4e2d-a179-ff725658809d -->

## 4. 管理员查询接口

- [x] 4.1 新增 `GET /api/log/upstream_payload`（`AdminAuth`，查询参数 `request_id`）：按 `request_id` 返回 `request_body`/`response_body`/两个 truncated 标志；无记录时返回成功且字段为空；空 `request_id` 返回参数错误；验证：`go build ./...` 通过，且新增用例断言未登录/非管理员请求被拒绝、管理员请求返回报文、未知 `request_id` 返回成功且字段为空。 <!-- comet-task:81679baa-eba2-4a33-8e44-2132344f72a4 -->

## 5. 前端展示

- [x] 5.1 在 `web/src/features/usage-logs/api.ts` 增加报文查询函数，在 `types.ts` 增加响应类型；验证：`bun run typecheck` 通过。 <!-- comet-task:108c032b-d494-438b-b3f3-e96b332fb905 -->
- [x] 5.2 在 `web/src/features/usage-logs/components/dialogs/details-dialog.tsx` 新增「上游请求体」「上游响应体」两个 `DetailSection`：仅 `props.isAdmin` 为真时渲染、默认收起、展开时才发起查询；内容用既有 `CodeBlock`（JSON 用 `json` 语言高亮，流式原文按纯文本），并用 `collapsedLines`/`maxExpandedLines` 限制首屏高度；截断时在面板内显示提示；无报文时显示空占位而不报错；验证：`bun run typecheck` 通过，并在浏览器中确认上述四种情形（admin 有数据 / admin 无数据 / 非 admin / 截断）各自呈现符合预期。 <!-- comet-task:e99c08f3-b731-4955-9967-f8abb2af3133 -->

## 6. 国际化

- [x] 6.1 两个新面板的标题、空占位、截断提示、设置页开关的名称与说明，按英文源串为键写入 `web/src/i18n/locales/` 下全部 7 个语言文件；验证：`bun run i18n:sync` 后无缺失键告警，且切换语言时这些文案随语言变化。 <!-- comet-task:fbff117b-4931-4efa-b806-f3aafdad8dc3 -->

## 7. 测试

- [x] 7.1 新增 `service/upstream_payload_capture_test.go`：使用 `testify/require` 与 `testify/assert`，用 SQLite 建表，覆盖 2.1 与 2.3 的全部断言（透传一致、上限截断、覆盖写、开关关闭、清理保留期）；验证：`go test ./service/ -run UpstreamPayload -v` 全部通过。 <!-- comet-task:814f1229-0967-471e-b917-4e8de90e7c8f -->
- [x] 7.2 在既有 `model/clickhouse_log_test.go` 中扩展断言，覆盖 1.2 的 CH 与非 CH 两条建表分支；验证：`go test ./model/ -run ClickHouse -v` 通过。 <!-- comet-task:95eb2d0c-34bb-4b2c-9cee-5ec0cd746877 -->

## 8. 数据库兼容性验证

- [x] 8.1 在真实 SQLite、MySQL（≥5.7.8）、PostgreSQL（≥9.6）上各验证一次：全新库启动迁移建表成功，以及用最新发布版本建的库升级后表与唯一索引存在、既有日志数据未变；每种库连续启动两次证明迁移幂等；验证：三库均无 `ALTER TABLE` 反复执行、无报错，`RequestId` 唯一约束生效。 <!-- comet-task:68091913-ecc0-4052-8cee-c72fc36afdd8 -->
- [x] 8.2 将日志库配置为 ClickHouse 验证：`upstream_payloads` 表按分支 SQL 创建成功、TTL 子句随 `LOG_SQL_CLICKHOUSE_TTL_DAYS` 生效，重启不重复建表；验证：`SHOW CREATE TABLE upstream_payloads` 输出与设计一致。 <!-- comet-task:8488b72c-b17a-431c-8465-2a67034846e2 -->
- [x] 8.3 记录 8.1 与 8.2 实际使用的数据库版本、启动命令、迁移输出与查询结果，写入最终交付说明；验证：交付说明中存在可复核的版本号、命令与结果。 <!-- comet-task:9d710ca8-a2ce-4ba2-8f12-262b63cb886a -->

## 9. 验证与构建

- [x] 9.1 后端：`gofmt` 无差异，`go build ./...`、`go test ./service/... ./model/... ./relay/...` 全部通过（本 change 不触及 `relaykit/` 与 `tokenkit/`，无需 `GOWORK=off` 模块构建）；验证：命令输出留档。 <!-- comet-task:4573995f-0e7e-4c6f-9c76-5ca3b91a3377 -->
- [x] 9.2 前端：`bun run typecheck`、`bun run test`、`bun run lint`、`bun run build` 全部通过；验证：命令输出留档。 <!-- comet-task:20e4a6ea-d365-4044-b693-11559e0315d2 -->
- [x] 9.3 端到端手工验证：开启开关后分别发起一次非流式与一次流式请求，管理员在「使用日志」详情的两个面板中看到格式化后的请求体与完整 SSE 原文，截断场景出现截断提示；非管理员账号看不到面板；关闭开关后新请求不再产生报文记录，面板显示空占位；验证：按上述步骤逐项确认并记录结果。 <!-- comet-task:de3f508c-1362-48f9-8e4c-1d1fe8c0c655 -->

## 10. 验证阶段修复（verify-fail 回合 1）

- [x] 10.1 后端 fail-soft 与确定性读取：`service/system_task.go` 的报文清理阶段任何错误只记日志（含 “upstream payload cleanup failed”）并继续执行日志清理，不再 `failSystemTask`；报文清理 `rowsAffected == 0` 时补含截止时间与剩余行数的 warn；收紧过强注释；`model/upstream_payload.go` 的 ClickHouse 读取分支去掉无效的 `Order("id DESC")` 只保留确定性 `created_at DESC`（非 ClickHouse 分支不变）；补 I-2 回归锁（构造报文清理失败仍 Succeeded 且过期日志被删）、M-4 两条断言（`log-old` 已删、任务终态 Succeeded）与 M-3 并发用例的可观察不变量断言；验证：`go test ./service/ ./model/ -count=1` 通过，`go test -race ./service/ -run 'Capture|Wrapper|UpstreamPayload|RunLogCleanupTask' -count=1` 无 race。 <!-- comet-task:5b743590-b9db-4e2d-acc9-eabfc8116c06 -->
- [x] 10.2 前端面板体验修复：`upstream-payload-panels.tsx` 仅在 `payload === null` 时渲染「无记录」占位，记录存在但该侧为空串时渲染 `-`（不新增 i18n key）；`api.ts` 的 `getUpstreamPayload` 加 `skipErrorHandler: true` 避免全局 toast 与面板内错误态重复；两个面板按钮改为可区分的可访问名；验证：定向 vitest（面板测试文件）通过、`bun run typecheck` 通过、定向 `bunx oxlint` 无输出。 <!-- comet-task:2293e07e-fa46-47d9-821c-0a3db12930ae -->
- [x] 10.3 规格措辞与实现对齐：delta spec 将 JSON「语法高亮」改为「格式化（缩进重排、可读且可复制）」；将「同一 `request_id` MUST 唯一」改为「MUST 只保留一条可读取记录，在无 upsert 的日志库（ClickHouse）上按写入追加 + 读取取最新实现」；验证：`comet classic openspec -- validate upstream-raw-payload --strict` 通过。 <!-- comet-task:dc7399e5-4d04-44e6-ba57-4f15bb490419 -->

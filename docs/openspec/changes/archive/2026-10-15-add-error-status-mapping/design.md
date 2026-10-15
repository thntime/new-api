---
comet_change: add-error-status-mapping
role: technical-design
canonical_spec: openspec
archived-with: 2026-10-15-add-error-status-mapping
status: final
---

# 状态映射（Error Status Mapping）技术设计

需求与行为规格见 `proposal.md` 与 `specs/error-status-mapping/spec.md`，本文不重复描述。

## Context

- 下游客户端（opencode v2）按响应头 `x-should-retry: true` > HTTP `429` > HTTP `5xx` 的优先级判定「可重试」，且 `429` 的响应体若含额度类文案会被判为「账号额度上限」而放弃重试。
- 现状：上游错误原样透传状态码。上游以 `200` 返回错误对象时，OpenAI 兼容适配器已识别出错误（`relay/channel/openai/relay-openai.go:282-284`，`types.WithOpenAIError(*oaiError, resp.StatusCode)` 中 `resp.StatusCode` 仍为 `200`），随后 `service/error.go:140-163 ResetStatusCode` 对 `200` 直接返回，最终由 `controller/relay.go:104` 输出 `HTTP 200 + {"error":{...}}`。客户端把该响应当作成功，限额错误无法触发重试。
- 错误对象承载上游信息：`relaykit/types/error.go:90-99` 的 `NewAPIError` 中 `Err` 保存上游原始 message（`Error()` 返回 `Err.Error()`，`:123-132`），`StatusCode` 保存状态码；`errorType`/`errorCode` 不导出且没有导出的 setter。
- 统一出口唯一：`controller/relay.go:90-109` 的 defer 决定最终状态码、响应头与响应体；非流式响应在提交前由 `c.JSON` 一次性写出。
- 同层已有先例：`ResetStatusCode`（渠道级 `status_code_mapping`，仅按状态码改写）、`service/violation_fee.go:30-71`（按报文子串改写 `code`）。
- 流式边界：`relay/helper/common.go:45-58 SetEventStreamHeaders` 只设置响应头，帧由 `c.Render(-1, ...)` 写出（`-1` 表示不改状态码）；一旦有帧写出，状态码即锁定为 `200`。

## Goals

- 提供全局「状态映射规则」：匹配条件为上游状态码 + 错误报文正则，输出为状态码 + 响应头 + 响应报文。
- 在统一错误出口生效，覆盖上游以 `200` 返回错误对象的形态。
- 无启用规则时输出与现状完全一致。

## Non-Goals

- 不改动已提交响应头的流式路径，也不为流式响应追加可重试响应头。
- 不改变 new-api 自身的内部重试决策、计费与错误日志中的状态码。
- 不新增上游 `200` 错误体的嗅探判定，沿用各适配器既有的 `GetXxxError() != nil && Type != ""` 模式。
- 不覆盖 WebSocket realtime 分支（`types.RelayFormatOpenAIRealtime`）与任务提交（task）输出。
- 不做按渠道、按用户、按模型的条件，也不做规则分组与优先级排序以外的排序方式。

## Decisions

### D1 规则求值点放在 `ResetStatusCode` 首行

新增 `service.ApplyErrorStatusMapping(newApiErr *types.NewAPIError)`，在 `service/error.go` 的 `ResetStatusCode` 开头调用。

- 该函数是 12 条 relay 错误路径共同经过的点（OpenAI/Claude/Gemini/embedding/responses/audio/rerank/image 等），无需改动调用方。
- 在此处 `newApiErr.StatusCode` 仍是上游状态码：早于 `ResetStatusCode` 对 `200` 的提前返回，也早于渠道级 `status_code_mapping` 的改写。
- 未选「在 `controller/relay.go` 出口求值」：出口处的状态码可能已被渠道级映射改写，Claude 非流式路径更是硬编码 `500`（`relay/channel/claude/relay-claude.go:349`），拿不到上游状态码。
- 未选「在 12 个调用点分别求值」：重复且易漏。

### D2 改写结果由 `relaykit/types` 新增承载字段传递

在 `NewAPIError` 上新增一个导出字段（`json:"-"`），携带改写后的状态码、附加响应头与报文三元组（message / type / code）：

```go
type ErrorResponseOverride struct {
    StatusCode int
    Headers    map[string]string
    Message    string
    Type       string
    Code       string
}

// NewAPIError 新增字段
ResponseOverride *ErrorResponseOverride `json:"-"`
```

- 原因：报文与错误类型无处安放。`errorType`/`errorCode` 不导出且无 setter，宿主无法通过改写错误对象来控制 `ToOpenAIError()`/`ToClaudeError()` 的输出。
- 原因：该字段必须跨 `service`（求值）与 `controller`（渲染）传递，而 `ResetStatusCode` 只接收错误对象与映射字符串，拿不到 `gin.Context`。
- 未选「新增 relaykit setter」：仍需另一个字段承载响应头，收益更低。
- `relaykit/` 必须保持独立可构建，改后以 `GOWORK=off go build ./...` 验证。

### D3 出口渲染按协议直接构造固定结构

`controller/relay.go` 的 defer 中，`ResponseOverride != nil` 且 `c.Writer.Written()` 为假时：先写入规则的附加响应头与 `Content-Type: application/json`，再按 `relayFormat` 输出规则报文并结束该分支。

- Claude 协议：`{"type":"error","error":{"type":"<Type>","message":"<Message>"}}`。
- 其他协议：`{"error":{"message":"<Message>","type":"<Type>","code":"<Code>"}}`。
- 命中时不执行 `SetMessage(common.MessageWithRequestId(...))`，使报文与规则配置完全一致；错误日志仍打印原始错误文本（`controller/relay.go:93`）。
- 未选「改写错误对象后复用 `ToOpenAIError()`/`ToClaudeError()`」：无法设置错误类型，且出参经过 `kitutil.MaskSensitiveInfo`，不能保证与规则配置逐字一致。规则报文由管理员配置，不来自上游，明文输出不引入敏感信息泄露。
- 响应已提交（`Written()` 为真）时完全走原有分支，符合规格中「已提交响应不可改」的要求。
- WebSocket realtime 分支保持不变。

### D4 规则以全局选项 `ErrorStatusMappingRules` 存储

值为 JSON 字符串数组，元素结构：

```json
{
  "name": "Upstream rate limit",
  "enabled": false,
  "match": { "status_code": 0, "message_regex": "exhausted|tpm/rpm|rps|rate limit" },
  "output": {
    "status_code": 429,
    "message": "rate limit exceeded",
    "type": "rate_limit_error",
    "code": "rate_limit_exceeded",
    "headers": [ { "key": "x-should-retry", "value": "true" }, { "key": "Retry-After", "value": "30" } ]
  }
}
```

- `match.status_code` 为 `0` 或缺省表示匹配任意状态码；`match.message_regex` 为空表示只按状态码匹配。
- 落点沿用全局设置项的既有范式（参照 `AutomaticRetryStatusCodes`）：默认值注册在 `model/option.go`，DB 载入在 `updateOptionMap` 分支，解析与匹配逻辑在 `setting/operation_setting/`，保存校验在 `controller/option.go`。
- 正则在解析配置时预编译，编译失败即视为配置非法，保存被拒绝（见规格「规则配置校验」）。
- 未选「走 `/api/option/request_policy` 通道」：该通道需要额外登记选项白名单、且采用全量选项写入，改动面更大且不带来额外收益。

### D5 匹配语义

- 大小写不敏感且为包含匹配：正则编译时统一加 `(?i)` 前缀，不做首尾锚定。
- 匹配文本取 `newApiError.Error()`，即上游原始错误报文（写入时早于出口的 `SetMessage`）。
- 规则按数组顺序求值，首个命中即生效，后续规则不参与。

### D6 预置一条限额规则且默认关闭

默认值仅含一条 `enabled: false` 的规则：正则 `exhausted|tpm/rpm|rps|rate limit`，输出 `429`、附加头 `x-should-retry: true` 与 `Retry-After: 30`、报文 `rate limit exceeded`（`type` 为 `rate_limit_error`、`code` 为 `rate_limit_exceeded`）。

- 输出报文与错误码取自 proposal 中客户端可识别的取值，不含任何额度类文案。
- 匹配状态码留空（任意）：上游限额既可能以 `429`、`5xx` 透传，也可能以 `200` 携带错误体返回，只按文案匹配才能统一覆盖。
- 默认关闭即默认不生效，升级后行为不变；管理员启用后该规则对所有渠道生效。

### D7 前端在「请求策略」下新增「状态映射」子页

- 在 `web/src/features/system-settings/request-policies/section-registry.tsx` 的 `POLICY_SECTIONS` 中，于 `routing` 之后追加 `{ id: 'status-mapping', titleKey: 'Status mapping', build: () => <StatusMappingSection /> }`；`POLICY_SECTION_IDS`、侧边栏导航与 `$section` 路由校验由此自动生效，无需新增路由文件。
- 新 section 通过系统选项通道读写：`useSystemOptions()` + `getOptionValue('ErrorStatusMappingRules')` 读取，`useUpdateOption({ key: 'ErrorStatusMappingRules', value: JSON.stringify(rules) })` 保存。
- 规则表复用共享组件：`@/components/data-table` 的 `StaticDataTable` 与行操作菜单、`@/components/dialog` 的编辑弹窗、`settings-form-layout`/`settings-section` 的布局；骨架参考 `general/channel-affinity/session-rules-table.tsx` 与 `rule-editor-dialog.tsx`，但不复用其业务组件与类型。
- 所有新增文案以英文源串作为 key 加入 7 个 locale 文件。

## 测试策略

- **规则解析与匹配（Go 单元测试）**：`setting/operation_setting/error_status_mapping_test.go` 表驱动覆盖：命中/不命中、`match.status_code` 缺省或 `0` 表示任意状态码、报文匹配大小写不敏感、`tpm/rpm` 字面量按包含匹配、非法正则被拒绝、按数组顺序首个命中生效。
- **服务层改写**：`service/error_status_mapping_test.go` 覆盖 `ApplyErrorStatusMapping`：命中时只设置 `ResponseOverride`、不改动 `StatusCode`/`Err`（保证内部重试判定与日志仍看到上游错误）；未命中时错误对象完全不变。
- **出口渲染**：以 `httptest` 驱动 `controller.Relay` 的错误出口，覆盖三种情形——命中且响应未提交（断言状态码、`x-should-retry`、`Retry-After`、`Content-Type` 与 body 结构）、未命中（断言与改写前行为一致）、响应已提交（断言不改状态码、不追加可重试响应头）。
- **前端**：扩展 `web/src/features/system-settings/request-policies/__tests__/settings.test.tsx` 覆盖新 section 的注册与规则保存；`bun run build` 通过。
- **端到端（按 proposal 的验收方式）**：用 `curl -i` 直打 `/v1/chat/completions`，分别构造「上游 429」与「上游 200 携带错误体」两种形态，人工核对状态码、响应头与 body 不含额度类文案；随后在 opencode v2 中触发同限额，确认日志中重复出现 `stream error` 且不再出现 `Failed to drain Session`。
- **数据库兼容**：本次无 schema 与迁移改动，仍按项目要求验证三种数据库上的启动迁移幂等与选项读写。

## Risks / Trade-offs

- [规则与渠道级 `status_code_mapping` 同时命中] → 规则按上游状态码求值、渠道映射在其后改写状态码，两者不冲突但可能叠加；已记录为已知交互，运维可用状态码条件区隔。
- [上游 `200` 错误体缺少 `type` 字段] → 适配器的 `GetOpenAIError() != nil && Type != ""` 判定不会识别该响应，规则无法生效。本次不放宽该判定（影响全部渠道）。
- [正则过宽导致误改写] → 默认关闭；启用后命中即整体替换响应体，不再返回上游原文。启用与调整需针对实际模型验证。
- [替换报文后丢失上游错误细节] → 服务器日志仍记录原始错误文本与上游状态码，排障信息不丢失。
- [新增 relaykit 导出字段] → 以 `GOWORK=off go build ./...` 验证模块独立构建；字段标记 `json:"-"`，不进入任何序列化输出。
- [Claude 非流式错误路径硬编码 `500`] → 规则在上游状态码上匹配，出口状态码由规则输出决定，不受该硬编码影响。

## Migration Plan

- 无数据库 schema 与迁移变更；选项未设置时使用代码内默认值（一条关闭的预置规则）。
- 回滚：将规则全部置为关闭，或清空/删除 `ErrorStatusMappingRules` 选项即可恢复原有行为，无数据清理需求。
- 上线即安全：默认关闭保证升级后行为不变，仅对显式启用的规则生效。

## Open Questions

无。菜单名（「状态映射」/`Status mapping`）、规则表形态（通用规则表 + 预置规则默认关闭）、流式边界（已提交的流不改）均已确认。

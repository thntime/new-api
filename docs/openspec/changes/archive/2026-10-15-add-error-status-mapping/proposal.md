# Proposal

## Why

上游网关的 TPM/RPM/RPS 限额错误目前被 new-api 原样透传（上游常返回 `200` 或 `4xx`，错误文本只出现在 body 里），下游客户端 opencode v2 无法把它判定为「限流、应重试」，请求直接终态失败。运维只能手工改上游或让用户重发，缺少一种可配置的方式把「特定上游错误」重写成「下游能识别的可重试响应」。

## What Changes

- 新增一套可配置的**状态映射规则**：每条规则由「匹配条件（上游状态码 + 报文正则）」和「输出（状态码 + 响应报文）」组成。
- 新增请求策略下的「**状态映射**」设置页（`Status mapping`），提供通用规则表，并预置四条限额规则（内容默认给出、开关默认关闭）：
  - 匹配上游错误文本含 `exhausted` / `tpm/rpm` / `rps` / `rate limit`（不区分大小写）；
  - 命中后输出 `429`，响应头 `x-should-retry: true`、`Retry-After: 30`、`Content-Type: application/json`；
  - 响应体按客户端协议给出 OpenAI 兼容格式（默认、OpenAI、Gemini）或 Claude 格式（Claude 协议请求）。
- 规则可覆盖上游以 `200` 返回错误 body 的情形，使这类错误不再被判为成功。
- 改写发生在非流式响应与「上游 `200` 但 body 为错误」的路径上。
- **不改变**已经提交响应头（SSE 已开始输出）后的错误处理方式：流式请求保持现状，不追加任何可重试标记。
- 默认配置不影响行为：未启用任何规则时，响应与改写前完全一致。

## Capabilities

### New Capabilities

- `error-status-mapping`: 上游错误响应的状态码与报文改写能力，包括规则的配置模型、匹配语义（状态码 + 报文正则）、输出形态（状态码、响应头、OpenAI / Claude 兼容报文）、生效边界（非流式，含上游 200 携带错误体），以及默认关闭与无规则时零影响的行为约束。

### Modified Capabilities

（无。现有 specs 均未覆盖 relay 错误改写行为。）

## Impact

- **后端设置**：新增规则设置项（JSON 数组），默认值、getter/setter、字符串序列化与校验位于 `setting/operation_setting/`；默认值注册与从数据库载入应用位于 `model/option.go`；HTTP 更新入口与入参校验位于 `controller/option.go`。
- **后端改写逻辑**：在 `service/` 新增规则匹配与响应改写入口，仿照既有 `service/error.go` 的 `ResetStatusCode`，在 `service/error.go` 的各 `RelayErrorHandler` 调用点之后应用；规则需同时能处理 `NewAPIError.StatusCode == 200` 但报文命中规则的情形。
- **后端出口**：`controller/relay.go` 的错误总出口按改写结果决定状态码、响应头（`x-should-retry`、`Retry-After`）与 body；流式路径不得改动已提交的响应头。
- **后端接口**：遵循现有系统设置读写通道（`GET/PATCH /api/option/request_policy` 或 `PUT /api/option/`），新增 key 需登记到相应白名单/校验分支。
- **前端**：在 `web/src/features/system-settings/request-policies/` 下新增 `Status mapping` section（`section-registry.tsx` 注册 + 新 section 组件 + 规则表与行编辑弹窗），复用既有共享组件；文案需补齐 7 个 locale 文件。
- **数据与权限**：规则为全局设置，仅超级管理员可修改；不新增数据库表，不涉及迁移。
- **数据库兼容**：仅使用现有 GORM 设置项读写路径，无 schema 变更，需在 SQLite / MySQL / PostgreSQL 下确认设置项读写正常。

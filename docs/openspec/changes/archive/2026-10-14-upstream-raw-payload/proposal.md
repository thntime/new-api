## Why

网关当前完全不保留「真实上游报文」：发往上游的请求体在序列化后立即被丢弃，上游返回的响应体在各渠道中被直接读取解析后丢弃，流式 SSE 帧逐帧消费后丢弃。消费日志只存计费与耗时诊断，错误日志只存脱敏后的错误文案。管理员排查「请求参数是否被改写」「上游到底返回了什么」「报错时上游原始响应长什么样」时，只能依赖 `DEBUG=true` 打印服务端日志——覆盖渠道不全、只进 stdout、无检索入口，且在生产环境等于不脱敏输出全量 prompt。

本次变更新增一张表保存每次请求的真实上游请求体与响应体，并在使用日志详情中仅对管理员展示，按 JSON 格式化渲染。

## What Changes

- 新增一张报文表（与消费日志同库，即 `LOG_DB`），按日志 `request_id` 关联，保存：发往上游的请求体、上游返回的响应体。
- 流式响应保存拼接后的完整 SSE 原文（含 `data:` / `event:` 行），而非逐帧结构。
- 采集挂在统一的上游 HTTP 出口与响应体读取处，覆盖走该出口的 chat/completions/responses/embeddings/images/audio/rerank 等渠道；绕过统一出口的渠道（AWS Bedrock SDK 调用、Replicate/Coze 轮询等）不在本次范围。
- 新增一个全局开关，默认关闭；关闭时采集与写入不产生任何开销。
- 新增管理员专用的报文查询接口，按 `request_id` 取单条报文。
- 使用日志详情弹窗新增「上游请求」与「上游响应」两个可折叠面板，仅管理员可见，复用项目已有的代码高亮组件格式化展示。
- 不脱敏：报文按原文保存、原文展示（仅管理员可访问）。这是本变更明确接受的风险。
- 报文随消费日志的清理策略一并删除。

## Capabilities

### New Capabilities

- `upstream-raw-payload`: 真实上游请求体/响应体的采集、存储、保留与管理员查询行为，覆盖全局开关、采集范围与豁免、关联键、流式原文形态、大小上限与保留策略。

### Modified Capabilities

- `usage-logs`: 使用日志详情新增仅管理员可见的上游请求体/响应体展示面板，含取值来源、空值占位与权限规则。

## Impact

- **后端采集**：`relay/channel/api_request.go`（统一上游出口的请求/响应字节采集），`relay/helper/stream_scanner.go`（流式响应原文）。
- **后端存储与迁移**：新增报文模型与查询（`model/`）、迁移注册（`model/main.go`，需覆盖 SQLite/MySQL/PostgreSQL 与日志库为 ClickHouse 的分支）、复用既有日志清理路径。
- **后端接口**：新增管理员专用路由（`router/api-router.go`）与控制器，按 `request_id` 查询。
- **后端开关**：新增全局开关注册与默认值（`setting/`）。
- **前端**：`web/src/features/usage-logs/`（详情弹窗面板、类型定义、i18n 七个语言文件、测试）。
- **数据与权限**：报文以明文保存，含 prompt 等业务内容，不脱敏；仅管理员可读。不含请求头与响应头，避免 Authorization/API Key 入库。
- **数据库兼容**：新表必须同时兼容 SQLite、MySQL、PostgreSQL；日志库为 ClickHouse 时走 ClickHouse 建表分支。

# Tasks

## 1. 后端规则配置与匹配

- [x] 1.1 新增 `setting/operation_setting/error_status_mapping.go`：定义规则结构（`name`、`enabled`、`match.status_code`、`match.message_regex`、`output.status_code|message|type|code|headers`）、包级变量、`ErrorStatusMappingRulesToString()` / `ErrorStatusMappingRulesFromString(s string) error`（解析时预编译正则，统一加 `(?i)` 前缀）与 `MatchErrorStatusMapping(statusCode int, message string) *ErrorStatusMappingOutput`（按顺序首个命中）。验证：新增唯一测试文件 `setting/operation_setting/error_status_mapping_test.go`，表驱动覆盖「命中/不命中」「状态码 0 表示任意」「大小写不敏感」「含 `/` 的 `tpm/rpm` 字面量」「非法正则被拒绝」「顺序优先」，`go test ./setting/operation_setting/` 通过。 <!-- comet-task:06ee7cfc-62d9-42ab-a775-69935b1606fe -->
- [x] 1.2 在 `model/option.go` 注册默认值 `ErrorStatusMappingRules`（一条 `enabled: false` 的预置限额规则：正则 `exhausted|tpm/rpm|rps|rate limit`，输出 `429` + `x-should-retry: true` + `Retry-After: 30` + `rate limit exceeded`/`rate_limit_error`/`rate_limit_exceeded`），并在 `updateOptionMap` 的 switch 中加入载入分支。验证：`go test ./model/ -run TestOption` 通过，且启动后 `GET /api/option/` 返回的 `ErrorStatusMappingRules` 可解析为一条 `enabled=false` 的规则。 <!-- comet-task:033ecc9a-373c-4fff-8730-a73a879ece68 -->
- [x] 1.3 在 `controller/option.go` 的校验 switch 中为 `ErrorStatusMappingRules` 增加校验（JSON 结构非法、正则不可编译、状态码越界均拒绝）。验证：扩展现有 `controller/option_test.go`（存在时）或用最小 httptest 用例提交非法正则，断言返回错误且 `model.OptionMap`/DB 中该键值未被修改。 <!-- comet-task:840da3cd-3398-46df-8490-9d12d8703a02 -->

## 2. 后端改写与统一出口

- [x] 2.1 在 `relaykit/types/error.go` 新增导出结构 `ErrorResponseOverride{StatusCode int; Headers map[string]string; Message, Type, Code string}` 与 `NewAPIError.ResponseOverride *ErrorResponseOverride` 字段（标签 `json:"-"`）。验证：`cd relaykit && GOWORK=off go build ./...` 与仓库根 `go build ./...` 均通过；`go vet ./relaykit/...` 无新增告警。 <!-- comet-task:57d1d4e8-23aa-480c-9bd3-e87ddfc04bd3 -->
- [x] 2.2 新增 `service/error_status_mapping.go` 的 `ApplyErrorStatusMapping(newApiErr *types.NewAPIError)`，并在 `service/error.go` 的 `ResetStatusCode` 首行调用（早于 `StatusCode == http.StatusOK` 提前返回）；命中时只设置 `ResponseOverride`，不改动 `StatusCode`、`Err` 与内部重试判定使用的字段。验证：新增单测（与 1.1 同文件或 `service` 内既有测试文件）断言：未启用规则时错误对象零改动；命中时 `ResponseOverride` 三元组与配置一致且 `StatusCode` 仍为上游值。 <!-- comet-task:e03b2653-924b-4299-af49-c6a0f792ecf9 -->
- [x] 2.3 在 `controller/relay.go` 的错误出口 defer 中增加渲染分支：`ResponseOverride != nil` 且 `c.Writer.Written()` 为假时写入 `Content-Type: application/json` 与规则附加响应头，按 `relayFormat` 输出 `{"type":"error","error":{"type":...,"message":...}}`（Claude）或 `{"error":{"message":...,"type":...,"code":...}}`（其他），并跳过 `SetMessage(MessageWithRequestId(...))`；`Written()` 为真或 realtime 分支保持原行为。验证：httptest 用例覆盖「命中改写」「未命中不变」「已提交后不改写」三种情形，断言状态码、`x-should-retry`/`Retry-After` 与响应体。 <!-- comet-task:67577c6b-7c00-4b8f-8c16-0c6cb2a8ff9c -->

## 3. 前端「状态映射」设置页

- [x] 3.1 先读 `web/AGENTS.md` 与 shadcn-ui 技能，确认可复用组件；新增 `web/src/features/system-settings/request-policies/status-mapping-section.tsx`，并在 `section-registry.tsx` 的 `POLICY_SECTIONS` 中于 `routing` 之后注册 `{ id: 'status-mapping', titleKey: 'Status mapping', build: () => <StatusMappingSection /> }`。验证：`cd web && bun run build` 通过，且 `/system-settings/request-policies/status-mapping` 可访问、侧边栏出现该项。 <!-- comet-task:9f32ec04-da8a-4206-a4d3-26bf7aa8c122 -->
- [x] 3.2 实现规则表与编辑弹窗（复用 `@/components/data-table` 的 `StaticDataTable` 与行操作菜单、`@/components/dialog`），支持增删改、启用开关、匹配状态码/正则输入、输出状态码/报文/类型/错误码与附加响应头列表；读写走 `useSystemOptions()`/`getOptionValue('ErrorStatusMappingRules')` 与 `useUpdateOption()`。验证：扩展 `web/src/features/system-settings/request-policies/__tests__/settings.test.tsx`，断言新增、编辑、删除后提交的 `ErrorStatusMappingRules` JSON 内容正确。 <!-- comet-task:e9d01586-57f7-4247-8bdd-326691f13b39 -->
- [x] 3.3 在 `web/src/i18n/locales/` 的 7 个 locale 文件补齐新增文案 key（含 `Status mapping` 与表单/弹窗文案）。验证：`cd web && bun run i18n:sync` 与 `bun run lint` 通过，且 7 个文件均包含新 key。 <!-- comet-task:77aa9ab6-de32-4665-99eb-cdc0639e7cfc -->

## 4. 集成验证

- [x] 4.1 端到端改写验证：启用一条匹配 `(?i)tpm/rpm` 的规则后，分别模拟上游返回 `200` 携带 OpenAI 错误体与返回 `429` 限额报文，断言客户端收到 `429`、响应头含 `x-should-retry: true` 与 `Retry-After: 30`、响应体不含 `insufficient_quota`/`quota exceeded`/`usage limit` 等文案。验证：`go test ./controller/ -run TestRelayErrorStatusMapping` 通过并记录实际输出。 <!-- comet-task:64a6e29e-ef95-4972-ae4a-beabaf7cbec7 -->
- [x] 4.2 流式边界回归：SSE 已开始输出后发生的限额错误不得改写状态码或追加可重试头，且响应头提交前的流式失败仍按规则改写。验证：流式测试用例断言已提交响应状态码仍为 `200` 且无 `x-should-retry`。 <!-- comet-task:a2e149b5-aeaf-45b2-9ac2-be2f3cab070e -->
- [x] 4.3 数据库兼容：在 SQLite、MySQL、PostgreSQL 三种数据库上启动两次并读写 `ErrorStatusMappingRules`，确认无 schema 变更、启动迁移幂等且选项读写正常。验证：记录三种数据库的版本、命令与输出；若某一数据库无法运行，明确报告阻塞而不宣称兼容。**【2026-10-15 实测阻塞】**本机无 MySQL ≥ 5.7.8 / PostgreSQL ≥ 9.6 实例：无 3306/5432 监听、未安装 `mysql`/`psql` 客户端、无 `docker`，故 MySQL 与 PostgreSQL 的启动两次幂等与选项读写**未验证，不宣称三库兼容**；SQLite 路径已由 `model/frontend_option_migration_test.go`、`controller/error_status_mapping_test.go` 的真实 sqlite 夹具间接覆盖。本 change 无表结构/索引/迁移变更，仅新增 option 键 `ErrorStatusMappingRules`。详见 `.comet/rulings.md`。 <!-- comet-task:0e09de80-2eda-41d9-9d1f-3f155de9be2b -->

## Workflow follow-up

- 按项目 review 要求完成独立审查后归档该 change，并验证归档结果。

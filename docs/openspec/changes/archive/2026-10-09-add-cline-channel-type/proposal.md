## Why

new-api 已内置 40+ 上游渠道类型，但缺少 Cline 这一 AI 编码服务提供方。Cline 同时提供按量（Cline）与计划（ClinePass / Coding Plan）两种使用方式，管理员目前无法在网关中接入、按方式取模型或查看计划用量。

## What Changes

- 新增渠道类型 `Cline`（类型 ID 94），默认 API 地址 `https://api.cline.bot/api`。
- 新增渠道级「计费方式」选择：`Cline`（按量）与 `ClinePass`（计划），复用现有 Access Mode / `EndpointProfile` 机制承载。
- 支持三个转发路由：`/v1/chat/completions`、`/v1/responses`、`/v1/messages`。
- 模型获取按方式分流：按量方式请求 `{base}/v1/models`；计划方式请求 `{base}/v1/ai/cline/recommended-models` 并**仅取 `clinePass` 数组**，用其 `id` 作为模型名。
- 计划方式提供 Coding Plan 用量查询：`{base}/v1/users/me/plan/usage-limits`，包含后端接口与前端用量弹窗。
- 新建 Cline 渠道时预填默认请求体参数覆盖（`providerOptions` / `provider`），用户可在渠道设置中编辑。
- 前端新增 Cline 品牌图标。
- 不改变 ClinePass 的计费语义：仍按 token 扣减用户额度，仅端点与模型来源/计划用量不同。

## Capabilities

### New Capabilities

- `cline-provider`: 定义 Cline 渠道类型的接入行为，包括渠道类型注册与默认地址、`Cline`/`ClinePass` 计费方式、三路由转发、按方式的模型获取、计划用量查询、默认请求体参数覆盖与图标展示。

### Modified Capabilities

<!-- 无：现有 channels spec 仅描述渠道列表序号排序，本次不改动其需求。 -->

## Impact

- 后端：`constant/channel.go`、`constant/api_type.go`、`common/api_type.go`、`common/endpoint_type.go`、`relay/relay_adaptor.go`、新增 `relay/channel/cline/`、`controller/channel_upstream_update.go`、新增计划用量 service/controller、`router/channel-router.go`、`relaykit/dto/channel_settings.go`（`EndpointProfile` 取值）。
- 前端：`web/src/features/channels/constants.ts`、`channel-utils.ts`、`channel-type-config.ts`、`channel-type-defaults.ts`、`channel-form.ts`、`channel-mutate-drawer.tsx`、`channels-columns.tsx`、`web/src/lib/lobe-icon.tsx`、`web/src/assets/custom/`、`web/src/i18n/locales/*.json`。
- 数据库：不改动 schema，复用现有 `OtherSettings` / `Setting` JSON 字段持久化方式选择与覆盖配置。

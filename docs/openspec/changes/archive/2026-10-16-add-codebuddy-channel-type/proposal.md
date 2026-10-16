## Why

new-api 已内置 40+ 上游渠道类型，但缺少腾讯 CodeBuddy 这一 AI 编码服务提供方。CodeBuddy 按国内/国际两个区域提供接入，两个区域的 API 地址不同且当前无法由网关推断；同时它需要兼容 chat completions、responses 与 messages 三种客户端协议。管理员目前无法在网关中接入该提供方。

## What Changes

- 新增渠道类型 `CodeBuddy`（类型 ID 93）；默认 API 地址留空，由管理员按所选区域自行填写。
- 新增渠道级「接入方式」选择：`国内` / `国际`。该选择 MUST 仅作为区域标记持久化到渠道设置，MUST NOT 推断、预填或改写 API 地址，也 MUST NOT 影响上游请求内容。
- 支持三个转发路由：`/v1/chat/completions`、`/v1/responses`、`/v1/messages`；messages 请求按 Anthropic 协议原样转发，不被降级为 chat completions。
- 模型获取复用默认行为：`GET {base}/v1/models`，解析标准 OpenAI 模型列表，无需按渠道类型特判。
- 前端新增 CodeBuddy 渠道类型选项与品牌图标。
- 不引入新的计费算法，不改动数据库 schema。

## Capabilities

### New Capabilities

- `codebuddy-provider`: 定义 CodeBuddy 渠道类型的接入行为，包括渠道类型注册与默认地址留空、`国内`/`国际` 接入方式区域标记、三路由转发、从上游获取模型列表与图标展示。

### Modified Capabilities

<!-- 无：现有 channels spec 仅描述渠道列表序号排序与亲和渠道重试，本次不改动其需求。 -->

## Impact

- 后端：`constant/channel.go`、`constant/api_type.go`、`common/api_type.go`、`common/endpoint_type.go`、`relay/relay_adaptor.go`、新增 `relay/channel/codebuddy/`、`relay/common/relay_info.go`、`relaykit/dto/channel_settings.go`（接入方式字段与校验）、`model/channel.go`（接入方式取值校验调用）。
- 前端：`web/src/features/channels/constants.ts`、`web/src/features/channels/lib/channel-form.ts`、`web/src/features/channels/components/drawers/channel-mutate-drawer.tsx`、`web/src/features/channels/lib/channel-utils.ts`、`web/src/lib/lobe-icon.tsx`（复用 lobe-icons 内置 CodeBuddy 图标，不新增自定义图标资产）、`web/src/i18n/locales/*.json`。
- 数据库：不改动 schema，复用现有 `OtherSettings` JSON 字段持久化接入方式。
- 不改动：`controller/channel_upstream_update.go` 与 `controller/model.go`（模型获取走默认 `/v1/models` 分支）。

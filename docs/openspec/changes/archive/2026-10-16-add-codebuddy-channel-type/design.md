---
comet_change: add-codebuddy-channel-type
role: technical-design
canonical_spec: openspec
archived-with: 2026-10-16-add-codebuddy-channel-type
status: final
---

# Design

## Context

动机与范围见 proposal.md，行为契约见 specs/codebuddy-provider/spec.md。现有仓库的相关约束：

- 渠道 adaptor 按 **APIType** 选择（`common.ChannelType2APIType` → `relay/relay_adaptor.go` 的 `GetAdaptor`），endpoint type 只用于模型定价/能力声明与渠道测试端点选择。
- `openai.Adaptor` 在 Claude 格式下会把 messages 请求转成 chat completions 并指向 `{base}/v1/chat/completions`，因此「messages 原样转发」无法靠映射到 `APITypeOpenAI` 实现。
- 渠道类型专属设置存放在 `relaykit/dto.ChannelOtherSettings`（先例：`commandcode_cookie`、`aws_api_type`、`endpoint_profile`），字段级校验以 `ValidateXxx()` 方法挂在同一结构体上（先例：`ValidateToolLossPolicy`），由 `model.Channel.ValidateSettings` 调用。
- 上游模型列表获取的默认分支即 `GET {base}/v1/models`，只有需要特殊协议或专属解析的渠道类型才登记分支。
- 前端已有「Access Mode」选择器形态（Cline 的 `pay-as-you-go`/`clinepass` 分支明确不改写 base_url），可直接作为「国内/国际」选择器的模板。
- `@lobehub/icons` 已内置 `CodeBuddy` 图标（toc 中 `hasColor` 为 true），无需自定义图标资产。

## Goals / Non-Goals

**Goals:**

- 以最小改动面新增 CodeBuddy 渠道类型（ID 93），复用既有渠道注册、协议转换与模型获取范式。
- 保证三种客户端协议各自以对应协议转发，messages 不被降级。
- 接入方式区域标记可持久化、可回显，且不产生任何地址或请求行为副作用。

**Non-Goals:**

- 不引入新的计费算法或用量查询入口。
- 不为国内/国际分别预置或推断 API 地址。
- 不改动数据库 schema，不改动用户级套餐系统。

## Decisions

### D1：区域标记使用独立的渠道设置字段 `codebuddy_region`

在 `relaykit/dto.ChannelOtherSettings` 新增 `CodeBuddyRegion string json:"codebuddy_region,omitempty"`，取值 `cn`（国内）、`intl`（国际）或空（未选择），并新增 `ValidateCodeBuddyRegion()` 做取值白名单校验，由 `model.Channel.ValidateSettings` 调用；前端表单以同名字段序列化到渠道 settings。

- 理由：区域标记不改变上游端点，而 `EndpointProfile` 的语义是「特殊套餐端点」，其解析链路（`ResolveSpecialPlan` + `ChannelSpecialBases`）会参与地址改写；复用它需要登记没有地址的魔法键，并依赖「魔法键缺失 → 不改写地址」这一隐式行为，语义晦涩、易被后续改动破坏。独立字段与既有类型专属字段（`commandcode_cookie`、`aws_api_type`）一致，且校验方式沿用 `ValidateToolLossPolicy` 先例。
- 备选（否决）：复用 `EndpointProfile` 并登记 `ChannelSpecialPlanProfiles[ChannelTypeCodeBuddy] = {"cn": ..., "intl": ...}`（Cline 的 `clinepass` 做法）。否决原因：Cline 的两个方式确实对应不同上游端点，而 CodeBuddy 的区域标记不对应任何端点差异，复用会把「套餐端点」概念扩张成「任意渠道子模式」。

### D2：新增独立 adaptor 包 `relay/channel/codebuddy`

渠道类型到 APIType 的映射必须显式登记，缺一不可：

- `constant/api_type.go`：新增 `APITypeCodeBuddy`（放在 `APITypeDummy` 之前）。
- `common/api_type.go` `ChannelType2APIType`：新增 `ChannelTypeCodeBuddy` 分支。此步不可省——`relay/common/relay_info.go:275` 调用时丢弃返回的 `ok`，未登记会静默回落到 `APITypeOpenAI`，messages 仍被降级，D2 整体失效。
- `relay/relay_adaptor.go` `GetAdaptor`：把 `APITypeCodeBuddy` 映射到新 adaptor（含 import）。

adaptor 实现 `relay/channel/adapter.go` 接口的全部方法：`GetRequestURL` 按 relay format/mode 拼接 `{base}/v1/chat/completions`、`{base}/v1/responses`、`{base}/v1/messages`（其余 mode 返回错误）；chat completions 与 responses 复用 openai 转换器，messages 复用 claude 转换器（`ConvertClaudeRequest` / `DoResponse`）；`DoRequest` 委托 `channel.DoApiRequest`；`ConvertRerankRequest` / `ConvertEmbeddingRequest` / `ConvertAudioRequest` / `ConvertImageRequest` / `ConvertGeminiRequest` 返回 not implemented（参照 `relay/channel/cline/adaptor.go`）。

- 理由：这是满足「messages 原样转发」的最小实现路径，且与 Cline 渠道的既有实现形态一致，便于对照维护。
- 备选（否决）：把 93 映射为 `APITypeOpenAI` 且不新增包。否决原因：`openai.Adaptor` 会把 messages 请求转换为 chat completions 并指向 `/v1/chat/completions`，静默改变客户端协议语义。

### D3：端点能力与流式支持登记

在 `common/endpoint_type.go` 声明 CodeBuddy 支持 `EndpointTypeOpenAI`、`EndpointTypeOpenAIResponse`、`EndpointTypeAnthropic`，并在 `relay/common/relay_info.go` 的 `streamSupportedChannels` 登记 93。

- 理由：端点能力影响模型定价/能力展示与渠道测试端点选择，必须与 adaptor 实际支持的协议一致。

### D4：模型获取复用默认分支

不修改 `controller/channel_upstream_update.go`：93 落入 `default` 分支即 `GET {base}/v1/models` 并走标准解析；仅在前端把 93 加入 `MODEL_FETCHABLE_TYPES` 以显示「获取模型」入口。

- 理由：需求指定的获取方式与默认行为完全一致，新增分支属于无效代码。
- 取舍：API 地址为空时该请求会拼成 `/v1/models` 并失败，由「未填写 API 地址时给出提示」场景覆盖。

### D5：前端复用既有 Access Mode 选择器形态

在 `channel-mutate-drawer.tsx` 参照 Cline 分支新增 CodeBuddy 的「接入方式」选择器，绑定 `codebuddy_region`，切换时只更新字段、不改写 base_url；同步更新 zod schema、表单默认值、编辑回显白名单与 `buildSettingsJSON` 的字段白名单。

- 理由：与既有 Access Mode 交互一致，避免引入新的表单机制。
- 备选（否决）：把区域做成渠道名称或备注的约定文本。否决原因：无法结构化回显与校验。

### D6：图标复用 lobe-icons 内置 CodeBuddy

在 `channel-utils.ts` 的 `TYPE_TO_ICON` 登记 `93 → 'CodeBuddy'`，由 `getLobeIcon` 通过 lobe-icons toc 元数据解析（`hasColor` 为 true，支持 `.Color` 变体）。

- 理由：图标已随依赖提供，新增自定义资产属于冗余。

### D7：API 地址为空时返回明确错误

`GetRequestURL` 在解析后的 base 为空时直接返回错误（如 `codebuddy channel base url is not configured`），不沿用 Cline 的「回落到 `constant.GetChannelBaseURL`」写法。

- 理由：`ChannelBaseURLs[93]` 为空（需求要求默认地址留空），回落只会拼出相对路径 `/v1/...`。而 `channel.DoApiRequest` 会把 `GetRequestURL` 的错误包装后上抛（`relay/channel/api_request.go:315`），三个 handler 均以 `ErrorCodeDoRequestFailed` 返回，属于既有契约，无需额外机制。
- 取舍：配置缺失时错误信息指向渠道配置，而不是难懂的 URL 解析失败。

### D8：按客户端协议使用各自的标准鉴权头

messages（Claude 格式）使用 `x-api-key: {api_key}` + `anthropic-version`（缺省 `2023-06-01`）并执行 claude 的通用头处理；chat completions 与 responses 使用 `Authorization: Bearer {api_key}`。

- 理由：两种客户端协议各自的标准头不同，与既有按格式分头的实现一致（`relay/channel/newapi/adaptor.go`、`ollama/adaptor.go`、`advancedcustom/adaptor.go` 均对 Claude 格式使用 `x-api-key`）。
- 待核实：真实上游对 messages 的鉴权头偏好，端到端验证时确认；若上游只接受 Bearer，改动局限在 adaptor 的头部设置方法内。

## Risks / Trade-offs

- [CodeBuddy 上游是否原生支持 `/v1/messages`、`/v1/responses`] → 按需求以 `/v1/*` 路径拼接；真实上游兼容性留待用户环境端到端验证，若不支持只需调整 `GetRequestURL` 中的路径常量。
- [messages 透传后的用量解析与计费] → 复用 claude 转换器的既有用量解析路径，不新增计费逻辑，保持现有按 token 扣减语义。
- [区域标记不参与任何上游行为，可能被误读为会影响路由] → 字段注释、设计文档与 spec 场景共同固定「不改变 API 地址」的语义。
- [修改 `relaykit/dto` 影响独立模块构建] → 改动后在 `relaykit/` 内执行 `GOWORK=off go build ./...` 验证。
- [前端新增字段遗漏序列化白名单会导致选择丢失] → 在 `channel-form.ts` 的回显与 payload 白名单同步登记，并以前端表单往返测试覆盖。

## Migration Plan

1. 后端：登记渠道类型（常量、`ChannelBaseURLs` 下标 93、`ChannelTypeNames`）与 APIType（`constant/api_type.go`、`common/api_type.go`）、端点能力、stream 支持；新增 adaptor 包；新增区域字段与校验。
2. 前端：登记渠道类型常量与展示信息、表单字段与选择器、图标映射、i18n 文案。
3. 无数据库迁移：区域标记复用 `OtherSettings` JSON 字段，历史渠道该字段为空（未选择），行为不变。回滚方式为回退代码版本。

## Open Questions

- 区域取值 token 定为 `cn` / `intl`（空 = 未选择）；若后续要与 GLM 的 `coding` / `coding-intl` 命名对齐，可在本 change 内调整，不影响 spec 与任务划分。
- CodeBuddy 上游是否在同一地址同时提供 `/v1/chat/completions`、`/v1/responses`、`/v1/messages`，以及 messages 的鉴权头偏好（见 D8）：留待用户环境端到端验证；不改变本设计的结构与任务划分，只影响路径常量或头部设置。

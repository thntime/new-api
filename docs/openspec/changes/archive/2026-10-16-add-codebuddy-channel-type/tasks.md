# Tasks

## 1. 后端渠道类型注册

- [x] 1.1 在 `constant/channel.go` 新增 `ChannelTypeCodeBuddy = 93`，补齐 `ChannelBaseURLs` 下标 93（空字符串），并在 `ChannelTypeNames` 登记 `ChannelTypeCodeBuddy: "CodeBuddy"`；验证 `go test ./constant/...`（在 `constant/channel_test.go` 中断言 93 的显示名为 CodeBuddy 且默认地址为空）与 `go build ./...` 通过。 <!-- comet-task:dd95ce0d-7ee2-4c77-93b1-deda5c5d5448 -->
- [x] 1.2 在 `constant/api_type.go` 新增 `APITypeCodeBuddy`，并在 `common/api_type.go` 的 `ChannelType2APIType` 增加分支；验证 `go test ./common/ -run ChannelType2APIType`（在既有 `common/api_type_test.go` 中补充 CodeBuddy 断言）通过。 <!-- comet-task:373e7115-5857-4500-a2c0-53692182b28a -->

## 2. 接入方式区域标记字段与校验

- [x] 2.1 在 `relaykit/dto/channel_settings.go` 的 `ChannelOtherSettings` 新增 `CodeBuddyRegion string json:"codebuddy_region,omitempty"`（取值 `cn`/`intl`/空）与 `ValidateCodeBuddyRegion() error`（沿用 `ValidateToolLossPolicy` 的 nil 安全与 trim 写法）；验证 `relaykit/` 内 `GOWORK=off go build ./...` 通过，并在 `relaykit/dto/channel_settings_test.go` 中覆盖 nil、空、`cn`、`intl` 与非法取值。 <!-- comet-task:2431ad6c-8cf6-499f-9164-178022687a1d -->
- [x] 2.2 在 `model/channel.go` 的 `ValidateSettings` 中调用 `ValidateCodeBuddyRegion()`（仅当渠道类型为 CodeBuddy 时校验，其他渠道类型保持既有行为）；验证 `go test ./model/ -run ValidateSettings`（在既有 `model/channel_settings_test.go` 中断言非法取值被拒、`cn`/`intl`/空被接受）通过。 <!-- comet-task:8cf4878f-1d13-4ba5-8f93-232d127649ad -->

## 3. CodeBuddy adaptor 与协议路由

- [x] 3.1 新增 `relay/channel/codebuddy/constants.go`（`ChannelName = "CodeBuddy"`、空 `ModelList`）与 `relay/channel/codebuddy/adaptor.go`，实现 `relay/channel/adapter.go` 接口的全部方法（未用到的 `ConvertRerankRequest`/`ConvertEmbeddingRequest`/`ConvertAudioRequest`/`ConvertImageRequest`/`ConvertGeminiRequest` 返回 not implemented，`DoRequest` 委托 `channel.DoApiRequest`）：`GetRequestURL` 按 `RelayFormat`/`RelayMode` 拼接 `{base}/v1/chat/completions`、`{base}/v1/responses`、`{base}/v1/messages`，base 取 `info.ChannelBaseUrl` 去尾斜杠，为空时返回明确错误而不拼出相对路径；`SetupRequestHeader` 对 messages（Claude 格式）使用 `x-api-key` + `anthropic-version`，chat completions 与 responses 使用 `Authorization: Bearer`；chat completions 与 responses 复用 `openai.Adaptor` 转换器，messages 复用 `claude.Adaptor` 的 `ConvertClaudeRequest`/`DoResponse`；验证 `go test ./relay/channel/codebuddy/...`（`adaptor_test.go` 覆盖三种格式的 URL、空地址报错、按协议鉴权头、渠道名）通过。 <!-- comet-task:26079557-be0a-4d36-a81a-5a2f3eecdd57 -->
- [x] 3.2 在 `relay/relay_adaptor.go` 注册 `APITypeCodeBuddy` → `codebuddy.Adaptor`；验证 `go build ./...` 通过，并确认 `common.ChannelType2APIType(93)` 在 `GetAdaptor` 中命中该分支（由 1.2 与 3.1 的测试共同覆盖）。 <!-- comet-task:6254c2b3-9cb2-4b01-865e-9586db797d31 -->

## 4. 端点能力与流式支持登记

- [x] 4.1 在 `common/endpoint_type.go` 为 CodeBuddy 声明 `EndpointTypeOpenAI`、`EndpointTypeOpenAIResponse`、`EndpointTypeAnthropic`，并在 `relay/common/relay_info.go` 的 `streamSupportedChannels` 登记 `ChannelTypeCodeBuddy`；验证 `go test ./common/ -run Endpoint` 与 `go build ./...` 通过（在既有端点类型测试中断言 93 同时包含三种端点）。 <!-- comet-task:7e52f902-71ef-4e53-bd84-ecdb406a7fcc -->

## 5. 前端渠道类型与图标登记

- [x] 5.1 在 `web/src/features/channels/constants.ts` 新增 `CHANNEL_TYPE_CODEBUDDY = 93`，并在 `CHANNEL_TYPES`、`CHANNEL_PROVIDER_PRESENTATION`、`CHANNEL_TYPE_DISPLAY_ORDER`、`MODEL_FETCHABLE_TYPES` 登记；在 `web/src/features/channels/lib/channel-utils.ts` 的 `TYPE_TO_ICON` 登记 `93 → 'CodeBuddy'`（复用 lobe-icons 内置图标，不新增图标资产）；验证 `bun run typecheck` 通过。 <!-- comet-task:a0b6ad89-84a1-409a-9c28-cbd1f5e51d0a -->

## 6. 前端表单字段与接入方式选择器

- [x] 6.1 在 `web/src/features/channels/lib/channel-form.ts` 的 zod schema、表单默认值、编辑回显白名单与 `buildSettingsJSON` 字段白名单中加入 `codebuddy_region`（仅 CodeBuddy 类型序列化，其他类型不得带出该字段）；验证 `bun run test src/features/channels` 中的表单往返用例通过。 <!-- comet-task:1fbfa7ed-1ef9-441e-8eb8-66bd11aa570a -->
- [x] 6.2 在 `web/src/features/channels/components/drawers/channel-mutate-drawer.tsx` 参照 Cline 的 Access Mode 分支新增 CodeBuddy 的「接入方式」选择器（国内/国际/未选择），切换时只更新 `codebuddy_region` 且不改写 base_url；验证 `bun run typecheck` 通过，且既有渠道类型的表单行为不变（`bun run test src/features/channels`）。 <!-- comet-task:408e54e9-3dc5-44f7-b52a-c87e2e352fe6 -->

## 7. 前端回归测试与文案

- [x] 7.1 新增 `web/src/features/channels/lib/__tests__/codebuddy-channel.test.ts`（镜像 `cline-channel.test.ts` 的组织方式，文件头版权声明与既有文件一致）：覆盖类型登记与排序、默认地址为空、`codebuddy_region` 的表单默认值/回显/创建与更新 payload 往返、`MODEL_FETCHABLE_TYPES` 包含 93、`getChannelTypeIcon(93) === 'CodeBuddy'`；验证 `bun run test src/features/channels` 与 `bun run copyright:check` 通过。 <!-- comet-task:4eb5526f-0dc0-40f3-8aea-c6fffa445df5 -->
- [x] 7.2 在 `web/src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json` 补齐渠道类型描述与接入方式选项文案；验证 `bun run i18n:sync` 无未同步 key，且 `bun run typecheck` 通过。 <!-- comet-task:8798145d-a573-4a43-ac79-192b672631c9 -->

## 8. 集成与验证

- [x] 8.1 后端集成检查：`go build ./...`、`go vet ./...`、`go test ./constant/... ./common/... ./model/... ./relay/channel/codebuddy/...`，以及 `relaykit/` 内 `GOWORK=off go build ./...`；确认无编译告警且全部通过。 <!-- comet-task:ef7986ae-0382-4636-9b29-da6098f892ff -->
- [x] 8.2 前端集成检查：`web/` 内 `bun run typecheck`、`bun run test src/features/channels`、`bun run build`；确认构建产物生成且既有渠道类型测试无回归。 <!-- comet-task:0f7b2176-07f3-45c5-9ab2-6a4d8ccc5230 -->
- [x] 8.3 记录真机端到端验证清单（需用户自行启动服务后执行）：新建 CodeBuddy 渠道并分别以 chat completions、responses、messages 三种客户端协议各发起一次请求（含一次流式请求），确认上游路径与协议未被改写、messages 的鉴权头被上游接受；把接入方式设为国内/国际后重新打开渠道确认回显，并确认 API 地址未被改写；在未填写 API 地址时确认「获取模型」给出提示且不写入模型；本次不执行，仅记录待验证项。 <!-- comet-task:0264f6bb-1a84-4582-b81d-7784082ecfcf -->

## 1. 后端渠道类型注册

- [x] 1.1 在 `constant/channel.go` 新增 `ChannelTypeCline = 94`、`ChannelTypeNames[94]`，并填充 `ChannelBaseURLs[94] = "https://api.cline.bot/api"`；验证 `go test ./constant/...` 通过且 `GetChannelBaseURL(94)` 返回该地址
- [x] 1.2 在 `constant/api_type.go` 新增 `APITypeCline`，并在 `common/api_type.go` 的 `ChannelType2APIType` 增加 `ChannelTypeCline` 映射；验证 `go build ./...` 通过且新增单测断言映射结果
- [x] 1.3 在 `common/endpoint_type.go` 的 `GetEndpointTypesByChannelType` 声明 Cline 支持 chat completions / responses / messages；按需在 `relay/common/relay_info.go` 的 `streamSupportedChannels` 登记；验证相关单测或编译通过

## 2. 计费方式（EndpointProfile）

- [x] 2.1 在 `constant/channel.go` 的 `ChannelSpecialPlanProfiles` 定义 Cline 计划方式 `EndpointProfile` 取值 `clinepass`（复用既有子模式机制，不新增 relaykit dto 常量）；验证 `go build ./...` 通过且注册单测断言该映射
- [x] 2.2 在 `constant/channel.go` 的 `ChannelSpecialBases` 增加 `cline-pass` 条目（两 base 均为 Cline 默认地址），使计划方式可解析到 Cline 端点；验证注册单测断言条目存在
- [x] 2.3 确认 `model/channel.go` 的 `ValidateSettings` 通过 `ChannelSpecialPlanProfiles` 自动接受 Cline 的 `EndpointProfile=clinepass` 并拒绝未登记取值（复用既有校验，无需 Cline 特判）

## 3. Cline adaptor 与三路由转发

- [x] 3.1 新建 `relay/channel/cline/adaptor.go` 实现渠道适配器接口：`GetRequestURL`、`SetupRequestHeader`、`ConvertOpenAIRequest`、`ConvertOpenAIResponsesRequest`、`ConvertClaudeRequest`、`DoRequest`、`DoResponse`、`GetModelList`、`GetChannelName`；chat/responses 复用 openai 转换器、messages 复用 claude 转换器；本任务若涉及从上游响应派生用量，先按 AGENTS.md 完整读取 `.agents/rules/billing.md`；验证 `go build ./...` 与新增 adaptor 单测通过
- [x] 3.2 在 `relay/relay_adaptor.go` 的 `GetAdaptor` 注册 `APITypeCline` 返回 Cline adaptor；验证 `go test ./relay/...` 或编译通过
- [x] 3.3 让 Cline adaptor 按 `EndpointProfile` 选择上游基础端点，保证 `{base}/v1/chat/completions`、`{base}/v1/responses`、`{base}/v1/messages` 三种路由拼接正确；验证针对三种 relay format 的 `GetRequestURL` 单测断言完整 URL

## 4. 按计费方式获取模型

- [x] 4.1 在 `controller/channel_upstream_update.go` 的取模型分派中新增 Cline 分支：按量请求 `{base}/v1/models` 走标准解析；计划请求 `{base}/v1/ai/cline/recommended-models` 并仅解析 `clinePass[].id`；验证新增 controller 单测覆盖两种方式与空/异常响应
- [x] 4.2 在前端 `web/src/features/channels/constants.ts` 的 `MODEL_FETCHABLE_TYPES` 加入 Cline（94）；验证前端相关测试通过

## 5. 计划用量查询（ClinePass）

- [x] 5.1 新增 Cline 计划用量 service（请求 `{base}/v1/users/me/plan/usage-limits`）与 `controller`，并在 `router/channel-router.go` 注册路由；仅当 `EndpointProfile` 为计划时可用；验证新增后端单测覆盖成功、非计划拒绝、上游失败三种情况
- [x] 5.2 前端 `web/src/features/channels/api.ts` 增加 Cline 用量请求，新增用量弹窗并在 `channels-columns.tsx` 增加入口；验证前端测试通过且弹窗展示 limits 的类型/已用百分比/重置时间

## 6. 前端类型、表单、覆盖与图标

- [x] 6.1 在 `web/src/features/channels/constants.ts` 注册 Cline 类型常量、`CHANNEL_TYPES[94]`、展示顺序/提供方呈现，并在 `channel-utils.ts` 的 `TYPE_TO_ICON` 登记 94；验证前端类型测试通过
- [x] 6.2 新增 `web/src/assets/custom/icon-cline.tsx` 并在 `web/src/lib/lobe-icon.tsx` 的 `CUSTOM_ICONS` 注册；验证渠道类型徽标与选择器展示 Cline 图标
- [x] 6.3 在 `web/src/features/channels/lib/channel-type-defaults.ts` 为 Cline 预填默认 API 地址与默认请求体参数覆盖（`providerOptions` / `provider`），并确认可编辑/清空；验证新建 Cline 渠道时覆盖预填，保存后按修改值生效
- [x] 6.4 让 Access Mode / `EndpointProfile` 选择器支持 Cline 的 `Cline` 与 `ClinePass`，并在 `channel-form.ts` 增加校验与序列化；验证选择/回显/保存往返一致
- [x] 6.5 在 `web/src/i18n/locales/*.json` 增加 Cline 相关文案键；验证 `bun run i18n:sync` 或前端构建无缺失键报错

## 7. 集成与验证

- [x] 7.1 静态/单元级集成校验：`go build ./...`、`go test ./constant/... ./common/... ./relay/channel/cline/... ./controller/... ./service/...`、前端 `bun run typecheck` 与 `bun run test src/features/channels`（44 文件 435 用例）全通过；真机端到端（新建渠道/两种方式取模型/三路由/计划用量/覆盖注入）需在用户环境启动服务后执行，本次未执行（遵循不主动启动服务规则）
- [x] 7.2 运行后端 `go build ./...`（BUILD_OK）、`go vet`（constant/common/cline/controller/service/router，exit 0）与前端 `bun run build`（exit 0）；未改动 `relaykit/`/`tokenkit/`，无需模块独立构建

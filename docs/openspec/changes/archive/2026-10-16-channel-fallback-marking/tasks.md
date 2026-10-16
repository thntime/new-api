# Tasks

## 1. 渠道标记与配置（数据层）

- [x] 1.1 在 `model/channel.go` 的 `ChannelInfo` 新增 `IsFallback bool`（json `is_fallback`），并让渠道新增/更新接口能够读写该字段（复用既有渠道更新链路，不新开接口）。验证：新增用例断言渠道创建与更新后可回读该标记，且 `channel_info` 的 JSON 往返不丢字段。 <!-- comet-task:45958e82-8801-4664-88e2-d52b54d1b7f8 -->
- [x] 1.2 在 `setting/operation_setting/channel_affinity_setting.go` 新增 `FallbackEnabled bool`（json `fallback_enabled`，结构体初始化处显式默认 `true`），并在 `model/request_policy.go` 白名单登记该键、保留 `fallback_channel_ids` 条目但不再读取。验证：`model/request_policy_test.go` 新增用例断言该键被接受、非法取值被拒绝、往返保持；并断言旧键仍被接受（不报 `unknown affinity option`）。 <!-- comet-task:5ad56a02-a089-470b-a0dd-6d98446738e9 -->

## 2. 池成员判定迁移（模型层）

- [x] 2.1 在 `model` 新增 `IsFallbackChannel(channelID int) bool` 与 `HasFallbackChannels() bool`（内存缓存路径读渠道缓存对象；`MemoryCacheEnabled=false` 时按候选渠道 ID 批量读取渠道行），并把 `model/channel_cache.go` 的排除/池过滤与 `model/ability.go` 的 DB 直连路径改为读渠道标记。验证：新增用例断言被标记渠道退出正常优先级层并出现在兜底阶段、同层按权重随机不变、DB 路径与内存路径结果一致。 <!-- comet-task:214e5c2b-9ee7-47fd-9685-20a46765e726 -->
- [x] 2.2 把 `service/channel_affinity.go` 的亲和缓存与 `descent` 锚点隔离守卫改为读 `model.IsFallbackChannel`，`service/channel_select.go` 的 `HasFallbackChannels` 委托 `model`，并移除 `operation_setting` 中作为池来源的旧判定（保留旧字段本身）。验证：既有 service 用例在新夹具下全通过，新增用例断言被标记渠道成功时不写亲和缓存、不作锚点。 <!-- comet-task:5852135c-ec4e-45a3-816d-ef732dfb8bc3 -->

## 3. 总开关生效范围

- [x] 3.1 总开关关闭时使 `HasFallbackChannels()` 为 false、`IsFallbackChannel` 恒 false、不记录任何 `fallback_retry` / `fallback_exhausted` 事件，行为与「未标记任何渠道」逐字等价（含 `strict` 分支与正常候选池为空的路径）。验证：controller 与 service 用例断言关闭总开关后的 `use_channel` 顺序与策略事件序列与改动前一致。 <!-- comet-task:1fdf8d9c-85ae-429d-b5a4-d5c94ba0cf8d -->

## 4. 前端渠道列表

- [x] 4.1 在 `web/src/features/channels/components/channels-columns.tsx` 的 `priority` 列之前新增「兜底渠道」列（`size 72` / `minSize 56`，默认可见，随既有列可见性机制可隐藏）：绿色实心圆表示兜底、空心圈表示非兜底，点击调用渠道更新接口写入相反标记，成功后失效渠道列表查询、失败提示并保持原状态；同时把名称 / 类型 / 分组三列 `size` 各下调约 10%（`minSize` 同步下调）。验证：前端用例断言点击发送的载荷与状态切换、失败时不改变显示，且该列出现在列可见性下拉中。 <!-- comet-task:d12e81e7-0215-4513-aaa0-4409ae76c72f -->

## 5. 前端渠道编辑与设置页

- [x] 5.1 在渠道新增/编辑对话框的「路由策略」区块「排序号」字段之后新增「兜底模型」开关与下方说明（说明文案加入 `FIELD_DESCRIPTIONS` 并同步 7 个语言包），表单值经 `channel_info.is_fallback` 随渠道载荷提交。验证：前端用例断言提交载荷含该标记、编辑态回显一致。 <!-- comet-task:6612429f-2e91-4357-90a1-528da16562fb -->
- [x] 5.2 在「会话与重试」页的兜底卡片新增「启用兜底渠道」总开关，移除渠道多选控件及其 delta 特判，同步 `routing-form.ts` / `defaults.ts` / `types.ts` 与 7 个语言包，并在卡片说明中写明兜底池由渠道标记决定、关闭即整体不生效。验证：前端用例断言保存载荷只包含 `fallback_enabled`、多选控件不再渲染、说明文案存在。 <!-- comet-task:352cf4ff-4044-46eb-8cb8-a15c1d0b4b52 -->

## 6. 回归与集成验证

- [x] 6.1 把既有的兜底测试夹具（`withFallbackChannelIDs`、`setFallbackChannelPool` 及直接写 `FallbackChannelIDs` 的约 30 处）改为「标记指定渠道为兜底」的 helper，使夹具与新的数据来源一致。验证：`go test ./controller/ ./service/ ./model/ ./relay/ -count=1` 全通过、`go build ./...` 通过、前端 `typecheck` 与 `request-policies`/`channels` 测试通过。 <!-- comet-task:17b8a255-b354-431e-ab88-00f27148ed4e -->
- [x] 6.2 端到端验收：正常层 10/20/30/40/50、重试次数 5、被标记的兜底渠道 66（优先级 6）与 55（优先级 5）全部失败时，尝试顺序为 50、40、30、20、10、50、66、55；关闭总开关后同一场景不出现兜底渠道；列表/对话框切换标记后下一请求即按新状态选路。验证：controller 端到端用例断言 `use_channel` 与策略事件，并在手工联调中核对 `use_channel` 重试链路日志。 <!-- comet-task:42de1c69-4eee-4a98-8d0f-31f5038960ab -->

## Workflow follow-up

- 完成 review 后归档本 change。
- 验证归档结果。

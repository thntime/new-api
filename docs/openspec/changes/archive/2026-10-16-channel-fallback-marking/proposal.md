# Proposal

## Why

兜底渠道池目前只能通过全局「渠道 ID 列表」配置：管理员必须在设置页手工查渠道 ID、维护一份与渠道列表脱节的数组，渠道侧看不出自己是否兜底，新增或停用渠道时这份列表很容易漂移。

把「是否兜底」变成渠道自身的属性——渠道列表里一眼可见、点击即可切换，渠道编辑时随手开关——既符合管理直觉，也让兜底池随渠道生命周期自动保持一致。同时补一个总开关，使兜底能力可以整体启停而不必清空配置。

## What Changes

- 渠道新增/编辑的「路由策略」区块新增「兜底模型」开关（含下方说明文案）：开启表示该渠道只参与兜底阶段。
- 渠道列表「优先级」左侧新增「兜底渠道」列：绿色实心圆表示兜底，空心圈表示非兜底，点击即切换；列宽默认较小（72px），并随渠道列表既有的列可见性机制支持显示/隐藏（localStorage 持久化）。为给新列腾出宽度，名称/类型/分组三列宽度各下调约 10%。
- 兜底池成员改为由渠道自身标记决定：`ChannelInfo`（`channel_info` JSON 列）新增 `is_fallback` 布尔字段，**零 DB 迁移**；内存缓存路径读缓存中的渠道对象，DB 直连路径（`MemoryCacheEnabled=false`）按候选渠道 ID 批量查询渠道行。
- 「会话与重试」页的兜底卡片保持独立卡片位置不变，新增「启用兜底渠道」总开关（后端 `channel_affinity_setting.fallback_enabled`，默认开启）：关闭时即使存在被标记的渠道，也完全不进入兜底阶段。
- **BREAKING（配置语义）**：全局 `channel_affinity_setting.fallback_channel_ids` 不再作为池来源，前端移除其多选控件；该 option 键保留在白名单中以兼容已保存的配置值（不再生效）。不做旧列表到渠道标记的自动迁移。
- 既有兜底阶段语义全部不变：独立于正常重试循环、不计入 `RetryTimes`、按兜底渠道自身优先级降序逐层推进且层内按权重加权随机、不写亲和缓存、不作 descent 锚点、渠道 pin 请求不进兜底。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `channel-affinity`: 兜底池的成员来源从全局渠道 ID 列表改为渠道自身的兜底标记，并新增兜底功能总开关；配置、退出正常候选池、进入兜底阶段的判定条件随之调整。

## Impact

- 后端：`model/channel.go`（`ChannelInfo.IsFallback`）、`model/channel_cache.go` 与 `model/ability.go`（池成员判定与两条选择路径）、`service/channel_affinity.go`（亲和/锚点隔离守卫改读渠道标记）、`setting/operation_setting/channel_affinity_setting.go`（新增 `fallback_enabled`，渠道标记判定迁出该包）、`model/request_policy.go`（白名单新增 `fallback_enabled`，保留 `fallback_channel_ids`）。
- 前端：`web/src/features/channels/components/channels-columns.tsx`（新增列）、`web/src/features/channels/components/drawers/channel-mutate-drawer.tsx` 与 `web/src/features/channels/constants.ts`（路由策略开关与说明）、`web/src/features/system-settings/request-policies/{routing-section.tsx,routing-form.ts,defaults.ts}`（总开关、移除多选）、`general/channel-affinity/types.ts`、7 个语言包。
- 测试：既有直接设置 `FallbackChannelIDs` 的夹具（`controller/`、`service/`、`model/`、`relay/` 共约 30 处）改为设置渠道标记；新增总开关、列表列切换、对话框开关的用例。
- 数据库：无 schema 变更（复用 `channel_info` JSON 列），无需三库矩阵的迁移验证；DB 直连路径新增一次按 ID 批量读取渠道的查询。
- API：渠道新增/更新载荷多一个 `channel_info.is_fallback` 字段；request-policy 多一个扁平 option 键 `channel_affinity_setting.fallback_enabled`，`channel_affinity_setting.fallback_channel_ids` 仍被接受但不再生效。

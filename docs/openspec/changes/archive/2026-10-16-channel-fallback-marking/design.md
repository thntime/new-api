---
comet_change: channel-fallback-marking
role: technical-design
canonical_spec: openspec
archived-with: 2026-10-16-channel-fallback-marking
status: final
---

# Design

## Context

参见 `proposal.md`。本次改动建立在上一个 change `channel-fallback-pool`（已归档）的既有实现之上，当前状态：

- 池成员由全局 option `channel_affinity_setting.fallback_channel_ids`（渠道 ID 数组）决定；判定入口是 `setting/operation_setting/channel_affinity_setting.go` 的 `GetFallbackChannelIDs()` 与 `IsFallbackChannel(channelID)`。
- 生产消费者全部经这两个函数（无旁路）：`model/channel_cache.go`（`excludeFallbackChannels` / `fallbackPoolMembers` / `GetFallbackChannel`）、`model/ability.go`（DB 直连路径的排除与池过滤）、`service/channel_affinity.go`（亲和缓存与 `descent` 锚点隔离）、`service/channel_select.go`（`HasFallbackChannels`）。
- 渠道的渠道级设置已有一个 JSON 列 `channel_info`（`ChannelInfo`，带 `Value()`/`Scan()`），其中已有布尔字段先例 `IsMultiKey`。
- 渠道列表列定义在 `web/src/features/channels/components/channels-columns.tsx` 的 `useChannelsColumns()`；列可见性机制（工具栏下拉 + localStorage 持久化）已存在。
- 渠道级布尔字段的前端先例：渠道编辑对话框「路由策略」区块的 `auto_ban` / `tls_insecure_skip_verify` 开关 + `FormDescription` 说明。

## Goals / Non-Goals

**Goals:**

- 把「是否兜底」从全局 ID 列表改为渠道自身的持久化标记，并提供渠道侧与列表侧的编辑入口。
- 提供兜底功能总开关，关闭后整体不生效且行为与既有实现一致。
- 保持既有兜底阶段语义（层序、加权随机、不计预算、不写亲和、不作锚点、pin 不进兜底、事件名）完全不变。

**Non-Goals:**

- 不做旧 ID 列表到渠道标记的自动迁移（该特性刚交付，无生产配置需要搬迁）。
- 不做「按模型」的兜底标记（标记是渠道级）。
- 不新增渠道列表新列的排序、筛选。
- 不改动数据库 schema、不引入新的 GORM tag 或迁移。
- 不改动 `channel_affinity_setting` 之外的请求策略字段。

## Decisions

### D1: 标记落在 `ChannelInfo.IsFallback`（`channel_info` JSON 列）

新增 `model.ChannelInfo.IsFallback bool`（json `is_fallback`），复用既有 JSON 列与其 `Value()`/`Scan()` 实现，**零 schema 变更、零迁移**；先例是同结构体里的 `IsMultiKey`。

被否决的方案：(a) 新增顶层列 `is_fallback bool` —— 需要 `AutoMigrate`，按项目规则会强制引入三库矩阵的迁移验证成本，收益仅是「可被 SQL 直接过滤」；(b) 写入 `setting` / `settings` 字符串 JSON —— 无类型保障、需手工解析。

### D2: 池成员判定迁到 `model` 包，内存与 DB 两条路径各自实现

`setting/operation_setting` 不能 import `model`（既有依赖方向是 `model → operation_setting`），而判定现在需要渠道数据，因此判定入口迁到 `model`：

- `model.IsFallbackChannel(channelID int) bool`：内存缓存启用时读 `channelsIDM[channelID].ChannelInfo.IsFallback`（持 `channelSyncLock.RLock`）；`MemoryCacheEnabled=false` 时按 ID 读取渠道行后判断。
- `model.HasFallbackChannels() bool`：总开关开启且存在被标记的渠道（内存路径统计缓存；DB 直连路径按查询结果判断）。
- `model/channel_cache.go` 的 `excludeFallbackChannels` / `fallbackPoolMembers` / `GetFallbackChannel` 与 `model/ability.go` 的对应过滤改为读渠道标记；DB 直连路径在 `filterAbilitiesByConstraints` 之后按候选渠道 ID 批量读取渠道行（一次 `IN` 查询，候选集由分组 + 模型限定）。

被否决的方案：维护一份「从渠道标记派生的全局 ID 集合」作为现有函数的输入 —— 引入第二份状态，渠道被编辑后存在缓存不同步的风险，且没有收益。

### D3: 总开关 `channel_affinity_setting.fallback_enabled`（默认开启）

该字段留在 `setting/operation_setting`（它是与渠道无关的全局配置）。默认值必须在结构体初始化处显式写成 `true`，否则新装环境会因 Go 零值变成「默认关闭」，与当前已交付行为不一致。

关闭时的语义：`HasFallbackChannels()` 返回 false、`IsFallbackChannel` 恒 false、不记录任何 `fallback_retry` / `fallback_exhausted` 事件 —— 即与「未标记任何渠道」逐字等价。

渠道侧的开关/列表圆点在总开关关闭时**仍可编辑**（配置保留，重新打开即生效），关闭状态的影响在设置卡片的说明文案中说明；不做跨页面的禁用联动。

### D4: 旧 `channel_affinity_setting.fallback_channel_ids` 的兼容处理

保留 Go 字段与 `model/request_policy.go` 的白名单条目，但不再作为池来源读取；前端移除多选控件、`types.ts` / `defaults.ts` / `routing-form.ts` 中的对应项与 delta 特判一并清理。

理由：白名单直接决定 `BuildRequestPolicy` 是否报 `unknown affinity option`，移除条目会让已保存该键的部署在保存/刷新请求策略时失败。不做数据迁移，只在提案/设计里记录语义变更。

### D5: 渠道列表「兜底渠道」列

列插入 `priority` 列之前，`id: 'fallback'`，读取 `row.original.channel_info.is_fallback`，`enableSorting: false`、`size: 72`、`minSize: 56`，默认可见（不写入 `initialColumnVisibility`），隐藏能力复用既有列可见性机制。

单元格为按钮：兜底 → 绿色实心圆；非兜底 → 空心圈。状态同时通过 `aria-pressed` / `aria-label`（含渠道名）表达，不依赖颜色单独传达信息。点击调用既有渠道更新接口写入相反的标记，成功后失效渠道列表查询；失败弹出 toast 并保持原状态（沿用仓库既有「点击改字段 + invalidate」范式，不做乐观更新）。

宽度：桌面端列宽是固定像素、超宽为横向滚动而非压缩邻列，因此按需求把名称 / 类型 / 分组三列 `size` 各下调约 10%（260→234、220→198、150→135，`minSize` 同步下调）为新列腾出空间。

### D6: 渠道编辑对话框「兜底模型」开关

放在「路由策略」区块「排序号」（`sort`）字段之后，复用既有 `Switch` + `FormDescription` 形态，说明文案加入 `FIELD_DESCRIPTIONS` 并同步 7 个语言包。表单值采用嵌套路径 `channel_info.is_fallback`，提交时随渠道载荷写入，保证列表列与对话框读取同一字段。

为让两条前端入口与后端一致，渠道的新增/更新接口需要能读写该字段（在既有 `UpdateChannel` 处理链上补充，不新开接口）。

### D7: 测试夹具从「设置 ID 列表」改为「标记渠道」

既有 `withFallbackChannelIDs`、`setFallbackChannelPool` 以及约 30 处直接写 `FallbackChannelIDs` 的用例改为「标记指定渠道为兜底」的 helper（内存路径改缓存中的渠道对象，DB 路径改渠道行），使夹具与新的数据来源一致。

### D8: 兜底语义不变

兜底阶段仍是正常重试循环之后的独立循环、不计入 `RetryTimes`、按标记渠道自身优先级降序逐层、层内加权随机、不写亲和缓存、不作 `descent` 锚点、pin 请求不进兜底、事件仍为 `fallback_retry` / `fallback_exhausted`（Source `global`）。细节以 `specs/channel-affinity/spec.md` 为准，本设计不重复。

## Risks / Trade-offs

- [DB 直连路径每次选渠多一次按 ID 批量读取渠道的查询] → 仅发生在 `MemoryCacheEnabled=false`，候选集由分组 + 模型限定、查询可索引；不为该模式引入派生缓存，避免缓存不同步。
- [`channel_info` 是 JSON 列，无法用 SQL 过滤标记] → 内存路径读缓存对象、DB 路径在 Go 侧判断；两条路径各有一组对等用例保证行为一致。
- [约 30 处既有测试夹具需要改造，漏改会掩盖回归] → 先改 helper，再逐包跑测试，最后全量 `go test ./...`；夹具集中到两个 helper 以避免散落。
- [旧 option 键保留但不再生效，可能让管理者困惑] → 设置卡片说明文案明确「兜底池由渠道标记决定」；该键仅作兼容保留。
- [列宽下调影响既有视觉] → 只下调三列 10%、新列 72px；若视觉不合适只需调整 `size` 常量。
- [总开关默认开启容易被误认为「未启用」] → 说明文案写明默认开启，且关闭时给出「整体不进入兜底阶段」的后果说明。

## 验证策略

- 配置与持久化：渠道创建/更新后回读标记；`channel_info` JSON 往返不丢字段；请求策略接受 `fallback_enabled`、拒绝非法取值、旧键 `fallback_channel_ids` 仍被接受（不报 `unknown affinity option`）。
- 池成员判定：内存路径与 DB 直连路径各一组对等用例，断言被标记渠道退出正常优先级层、出现在兜底阶段、层内仍按权重随机、不被约束过滤放过。
- 总开关：关闭时 `use_channel` 顺序与策略事件序列与「未标记任何渠道」一致（含 `strict` 分支与正常候选池为空的路径），且不记录任何 `fallback_*` 事件。
- 隔离语义：被标记渠道成功时不写亲和缓存、不作 `descent` 锚点；pin 请求仍不进兜底。
- 前端：列表列点击产生的请求载荷与状态切换（失败时显示不变）；对话框提交载荷含标记且编辑态回显一致；设置页保存载荷只含 `fallback_enabled` 且渠道多选控件不再渲染；既有「兜底池在会话默认卡片之外」测试保持通过。
- 端到端：正常层 10/20/30/40/50 + 重试 5 + 标记兜底 66（优先级 6）/55（优先级 5）全失败 → 尝试顺序 `50、40、30、20、10、50、66、55`；关闭总开关后同一场景不出现兜底渠道。
- 回归：`go test ./controller/ ./service/ ./model/ ./relay/ -count=1`、`go build ./...`、前端 `typecheck` 与 `request-policies`/`channels` 测试。

## Failure Paths

- DB 直连路径的批量读渠道失败 → 视为无合格兜底渠道（沿用既有「无可用渠道」失败语义），不记录 `fallback_retry`。
- 渠道更新接口拒绝写入标记 → 前端保持原显示并提示，缓存与列表不被污染。
- JSON 列中已存在未知字段 → `ChannelInfo` 的 `Value()`/`Scan()` 必须原样保留，避免渠道设置被整体覆盖。

## Migration Plan

- 无 schema 迁移、无数据搬迁：部署后即按渠道标记生效。
- 旧 `fallback_channel_ids` 配置值被忽略；如需继续使用，管理员在渠道上重新开启标记即可。
- 回滚：恢复池成员判定为读取 `fallback_channel_ids` 即可（字段与白名单条目仍在）；渠道 JSON 中的 `is_fallback` 标记对其它功能无影响。

## Open Questions

无。

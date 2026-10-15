---
comet_change: channel-affinity-descent-session-mode
role: technical-design
canonical_spec: openspec
archived-with: 2026-10-15-channel-affinity-descent-session-mode
status: final
---

# 设计

## 背景

动机见 `proposal.md`。以下现状事实决定实现方式（均已核对源码）：

- 候选池选择只有一个汇聚点：`service.CacheGetRandomSatisfiedChannelWithPriority(param, priorityRetry)`。它的调用方是 `service.SelectChannelForRequest`（分发中间件与 Responses WebSocket 的首轮兜底、以及 WS 的重试）和 `controller/relay.go getChannel`（主 relay 与 task relay 的重试）。
- `model.GetRandomSatisfiedChannel(group, model, retry, filters)` 把 `retry` 当作**降序优先级层数组的下标**（0 = 最高层），越界时 clamp 到最低层；层内按渠道 weight 加权随机。内存缓存关闭时走 `model.GetChannel`，下标语义相同。
- 亲和命中的首轮由 `MarkChannelAffinityUsed` 打上 gin key 标记，`service.AffinityAdjustedRetry` 据此把 retry 前移一位，使 layer 0 参与重试。这就是「原渠道失败后跳到最高层」的来源。
- `RequestPolicy(c).SessionMode` 在首轮由 `EffectiveSessionMode` 写入，整个请求生命周期内可读（`controller/relay.go` 的 task 路径已有先例）。
- `common.RetryTimes` 是可变全局量，重试循环条件为 `retry <= common.RetryTimes`。

## 数据流

```
请求进入
  -> middleware/distributor.go: SelectChannelForRequest(c, model, retryParam)   [retry=0]
       |-- 渠道 pin 命中 -------------> 直接返回 pin 渠道（不记录锚点）
       |-- 亲和命中 -----------------> 返回亲和渠道（usedGroup = 该渠道所属分组）
       `-- 兜底 CacheGetRandomSatisfiedChannel(retry)
                                        `-> CacheGetRandomSatisfiedChannelWithPriority
  -> SetupContextForSelectedChannel，relay handler 实际发起请求

失败进入重试（controller/relay.go 或 responses_websocket.go 的 retry 循环）
  -> getChannel -> [retry=0] 用上下文里的 channel_id 组成合成 Channel
                ` [retry>=1] AffinityAdjustedRetry -> CacheGetRandomSatisfiedChannelWithPriority
       `-> 模式为 descent 且锚点分组 == 当前候选分组
              -> model.GetRandomSatisfiedChannelDescending(..., retry)
                     `-> descentLayerIndex: (锚点层下标 + retry) mod 层数
          否则 -> model.GetRandomSatisfiedChannel(..., retry)   [历史语义]
```

锚点在首次成功选中渠道时记录（亲和命中或候选池皆可），此后整个请求内只读；跨 retry 生命周期不变。

## 目标 / 非目标

**目标：**

- 新增会话模式 `descent`：首轮复用原渠道，原渠道失败后按优先级层逐层向下，到最低层回绕最高层形成闭环。
- 锚点层取首轮实际使用渠道所在的优先级层；层内保持既有加权随机。
- 后端配置白名单、前端全部枚举与 7 个语言包同时支持该取值。
- 其它会话模式与渠道 pin 语义保持不变。

**非目标：**

- 不引入「排除本轮已失败渠道」的机制（用户已决定）。
- 不以单个渠道为推进单位（用户已决定）。
- 不在 auto 分组之间保持环形游标（用户已决定）。
- 不重构 `AffinityAdjustedRetry` 对现有模式的含义。
- 不改数据库结构、不做数据迁移。

## 决策

### D1：环形算术放在 model 层，用独立包装函数暴露

**为什么：** `(锚点层下标 + retry) mod 层数` 需要「层数」与「锚点层在候选集中的位置」，这两者只有 `model.GetRandomSatisfiedChannel`（缓存路径）和 `model.GetChannel`（数据库路径）内部知道。

**做法：** 把两处函数体改为带锚点参数的未导出核心，各加一个导出包装，现有调用方（2 个生产调用点、12 个测试调用点）完全不动：

```go
func GetRandomSatisfiedChannel(group, model string, retry int, filters []dto.ChannelFilter) (*Channel, error)
func GetRandomSatisfiedChannelDescending(group, model string, anchorPriority int64, retry int, filters []dto.ChannelFilter) (*Channel, error)

func GetChannel(group, model string, retry int, filters []dto.ChannelFilter) (*Channel, error)
func GetChannelDescending(group, model string, anchorPriority int64, retry int, filters []dto.ChannelFilter) (*Channel, error)
```

两条路径共用同一个层下标解析函数（把 `sortedUniquePriorities` 与 `priorities` 统一为 `[]int64`）：

```go
// descentLayerIndex 返回目标优先级层下标。anchorPriority 为 nil 时保持历史语义
// （retry 即降序层下标，越界 clamp 到最低层）；否则以第一个不高于 anchorPriority
// 的层为锚点，按 retry 环形向下推进。
func descentLayerIndex(prioritiesDesc []int64, anchorPriority *int64, retry int) int {
	if anchorPriority == nil {
		return min(retry, len(prioritiesDesc)-1)
	}
	above := sort.Search(len(prioritiesDesc), func(i int) bool { return prioritiesDesc[i] <= *anchorPriority })
	return (above + retry) % len(prioritiesDesc)
}
```

**被否方案：**

- *在 service 层算好下标再当作 retry 传入* —— 需要 model 额外暴露层数，且「读层数」与「按下标选渠道」之间候选集可能变化，会选到相邻层。
- *直接给 `GetRandomSatisfiedChannel` 加参数* —— 要改 14 个调用点，收益为零。
- *只改缓存路径，不管数据库路径* —— 内存缓存关闭是受支持的部署方式，那里会静默退化成「饱和在最低层」。

### D2：锚点记录为「分组 + 优先级」，先写者胜

在 service 包新增一对内部函数与一个 gin key：

```go
// rememberDescentAnchor 记录本次请求的环形锚点层。仅当会话模式为 descent 且尚未
// 记录时生效；其余情况为无副作用的空操作，因此调用点无需分支。
func rememberDescentAnchor(c *gin.Context, group string, channel *model.Channel)

// descentAnchor 读取已记录的锚点层。
func descentAnchor(c *gin.Context) (group string, priority int64, ok bool)
```

调用点只有两处，覆盖全部首轮来源：

- `service.CacheGetRandomSatisfiedChannelWithPriority` 选中渠道之后（所有候选池选择的唯一汇聚点，同时覆盖 WS 重试与 task relay 的首次候选池选择）。
- `service.SelectChannelForRequest` 最终 `return channel, selectGroup, nil` 之前（覆盖亲和命中这条绕过候选池的路径；兜底分支会先经上一处记录，此处为首写者胜的空操作）。

**分组字段的一致性（已核对）：** `SelectChannelForRequest` 内 `usingGroup := retry.TokenGroup`，非 auto 分支记录的 `selectGroup` 就是 `retry.TokenGroup`，auto 分支记录的是实际命中的具体 autoGroup；`CacheGetRandomSatisfiedChannelWithPriority` 在 auto 循环里比较的也正是具体 autoGroup、非 auto 比较 `param.TokenGroup`。两处同源，因此可以直接做字符串相等比较。

**渠道 pin 交互：** pin 命中时 `SelectChannelForRequest` 会提前 return，不记录锚点；`PinRetrySingleAttempt` 本就抑制重试，`PinRetrySameChannel` 固定同一渠道，环形对二者都没有意义，保持现状。

**被否方案：**

- *复用 `use_channel` 首个元素* —— 那是给日志用的 string 切片，不含优先级，语义也不是选路锚点。
- *在 `getChannel` 里记录* —— 该分支的主 relay 首轮渠道是只带 Id/Type/Name 的合成结构，没有 priority。

### D3：环形只在锚点所属分组内生效

`CacheGetRandomSatisfiedChannelWithPriority` 在候选分组确定后，仅当 `锚点分组 == 候选分组` 才把锚点传给 model 层，否则按普通层序。auto 分组切换时 `priorityRetry` 已被重置为 0，锚点同时被丢弃，因此新分组从最高层重新开始，符合既定语义。

### D4：`AffinityAdjustedRetry` 增加 descent 保护

```go
func AffinityAdjustedRetry(c *gin.Context, retry int) int {
	if retry > 0 && AffinityConsumedFirstTry(c) && RequestPolicy(c).SessionMode != "descent" {
		return retry - 1
	}
	return retry
}
```

**为什么：** descent 的层下标是 `锚点层 + retry`，`retry=1` 必须落在锚点层的下一层；沿用 `retry-1` 会重选原层。该函数生产调用点只有 `controller/relay.go` 一处，改动面最小。

### D5：枚举与白名单同步新增取值

- 后端 `model/request_policy.go` 的全局与规则级两个 `switch` 白名单加入 `descent`。该处是 fail-closed：`BuildRequestPolicy` 返回错误后刷新的快照不会被写入，不同步会导致含该取值的配置保存被拒。
- 前端：`channel-affinity/types.ts` 的 `SessionMode` union、`request-policies/routing-form.ts` 的 zod `enum`、`routing-section.tsx` 的全局单选项列表、`rule-editor-dialog.tsx` 的规则下拉选项、`policy-label.ts` 的文案分支。
- `session-rules-table.tsx` 的图标分支不新增，复用默认图标（切换语义，无需新图形）。

### D6：文案与语言包

英文源串 `Prefer the original channel, then descend through priority layers`，中文「优先原渠道，失败后逐层降级」；7 个语言包按同键补齐。`Global default: {{mode}}` 处的 `{{mode}}` 必须传已翻译标签，不能是原始取值。

### D7：测试落点

- 后端环形行为：扩展现有 `service/channel_select_auto_groups_test.go`（已具备多分组、多优先级与 `CacheGetRandomSatisfiedChannelWithPriority` 的夹具），加一条表驱动用例，断言「起点层 = 锚点层、每次下降一层、从最低层回绕到最高层」。
- 后端白名单：扩展现有 `model/request_policy_test.go`，断言全局与规则级均接受 `descent`、未知取值仍被拒。
- 前端：扩展现有 `web/src/features/system-settings/request-policies/__tests__/routing.test.tsx`，加 `descent` 的读写往返用例。
- 不新建测试文件。

## 风险 / 权衡

- [走完一整环后回到锚点层，可能再次选中本轮已失败的渠道] → 这是明确取舍，不引入失败集；重试上限仍由 `common.RetryTimes` 兜底。
- [task relay 与钉住渠道路径的首轮不经候选池，锚点由首次候选池选择记录] → 这些路径的环形起点是首次候选池所在层，而不是首轮尝试层；已在非目标与文档中说明。
- [跨分组不保持环游标] → 换取「锚点层在目标分组中不存在时不会产生无意义起点」。
- [把数据库路径一并纳入] → 多一处算术调用点，换来内存缓存关闭时行为一致。
- [既有 `channels` 需求与新模式的语义冲突] → 通过该能力下的 MODIFIED delta 把既有需求限定为「非 descent 路径」。
- [回滚时旧版本会拒绝含 `descent` 的配置] → 旧版本 `BuildRequestPolicy` 失败后保留旧快照，不会崩溃，但亲和性配置将停止更新；回滚属于可接受的人工操作。

## 迁移方案

1. 纯代码变更，无 schema 变更、无数据迁移、不触碰线上数据。
2. 建议后端先于前端发布：前端新增选项写入的取值需要后端白名单接受；反过来前端单发不会产生该取值。
3. 回滚 = 回退代码。配置中残留的 `descent` 在新版本中仍合法；在旧版本中会使亲和性配置停止生效（fail-closed），需人工改回其它取值。

## 未决问题

- `descent` 在会话规则表格中是否需要独立图标（当前复用默认切换图标）。纯展示层，不影响规格、方案与任务拆分。

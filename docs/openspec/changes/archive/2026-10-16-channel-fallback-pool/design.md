---
comet_change: channel-fallback-pool
role: technical-design
canonical_spec: openspec
archived-with: 2026-10-16-channel-fallback-pool
status: final
---

# Design

## Context

动机与范围见 `proposal.md`，行为契约见 `specs/channel-affinity/spec.md`。本文件只记录实现路径与取舍。

当前链路（已逐行核对）：

- 主重试循环 `controller/relay.go` 的 `for ; retryParam.GetRetry() <= common.RetryTimes; retryParam.IncreaseRetry()`：每轮调用 `getChannel` 取渠道，失败后由 `service.DecideRelayRetry(c, newAPIError, common.RetryTimes-retryParam.GetRetry())` 决定是否继续；`retryTimes <= 0` 时返回停止原因 `attempt_budget_exhausted`。
- 任务/异步路径有同构的第二条循环（`controller/relay.go` 的 `executeTaskSubmissionWith`），使用 `decideTaskRetry`，且 `retryTimes <= 0` 的判定次序与 HTTP 路径不同。
- WS Responses 路径（`relay/responses_websocket.go`）复用 `service.DecideRelayRetry`，但取渠道走 `service.SelectChannelForRequest`。
- `getChannel`：首轮直接用中间件已选渠道；重试轮走 `service.AffinityAdjustedRetry(c, retry)` → `service.CacheGetRandomSatisfiedChannelWithPriority(retryParam, effectiveRetry)`。
- 候选选择收敛在 `model.getRandomSatisfiedChannel`：候选 ID → 优先级去重降序 → `descentLayerIndex` 取层（`descent` 模式带锚点，环形回绕）→ 层内按权重加权随机。`strict` 的亲和渠道失败由 `service/relay_error.go` 的 `ShouldSkipRetry` 直接停止；`strict` 且首轮亲和不可用时由 `service/channel_select.go` 的 `SelectChannelForRequest` 返回 503 `strict_session_binding_unavailable`。
- 亲和写回发生在 `middleware/distributor.go` 的成功路径与 WS 的成功事件路径（`RecordChannelAffinity`），`descent` 锚点由 `rememberDescentAnchor` 写入 gin context。
- 配置为 `ChannelAffinitySetting`，扁平 option 键 `channel_affinity_setting.<field>`，后端白名单在 `model/request_policy.go`，持久化走 option 快照，无独立表。

## Goals / Non-Goals

**Goals:**

- 把「正常候选阶段」与「兜底阶段」表达为同一请求内串联的两段尝试序列，使尝试顺序可预测、可观测。
- 让兜底段复用正常段既有的「按优先级分层 + 层内加权随机」选择语义，而不复用其环形回绕与锚点语义。
- 让兜底段与重试预算解耦：正常段用满预算后兜底段必然执行。

**Non-Goals:**

- 不改 `descent` 环形回绕与锚点行为（既有 spec 场景必须继续成立）。
- 不做 per-rule 兜底渠道列表（全局一份）。
- 不改亲和缓存 key 结构、缓存统计与清理接口。
- 不改渠道 pin 的语义。
- 不新增数据库表或迁移。

## Decisions

### D1. 兜底段是正常重试循环之后的独立循环，不切分外层重试索引

正常重试循环的循环条件、预算入参与停止判定完全不动；正常段终止后（`DecideRelayRetry` / `decideTaskRetry` 返回 `stop`，或取渠道返回无可用渠道）不再直接结束，而是进入独立的兜底循环：按兜底优先级层从高到低各尝试一次，层内按权重加权随机，全部失败才返回错误。

否决「用外层 `retryParam.GetRetry()` 切分正常段/兜底段」的原因（已核对代码）：

- `retryParam.GetRetry()` 并不是优先级层的单调索引：`auto` 分组分支在 `service/channel_select.go` 的「本组无渠道」与「跨组重试耗尽」两处会 `param.SetRetry(0)`（后者还会 `param.ResetRetryNextTry()`），外层计数会被重置，`auto` 下正常段总尝试数可远超 `RetryTimes+1`。
- `AffinityAdjustedRetry` 只调整传给选择函数的 `priorityRetry`，不改外层计数——外层计数与「当前优先级层」本就解耦。
- `attempt_budget_exhausted` 在 `auto` 下会在每个分组各触发一次，不能作为兜底入口。

正常段终止后仍由现有逻辑决定「是否继续正常重试」，兜底阶段只在正常段真正结束后进入，因此验收例 `50→40→30→20→10→50→66→55` 中前 6 次仍由现有循环原样产生。

代价：一次渠道尝试的主体（`AppendUsedChannel`、计费准备、`GetBodyStorage` 与 `Request.Body` 重置、按 `relayFormat` 分发、成功标记、失败归一化与策略记录、`processChannelError`）需要抽成正常段与兜底段共用的执行单元，HTTP 主循环与任务循环共用同一形态。这是本 change 最大的一处结构重构，必须保持既有行为不变。

### D2. 兜底候选走独立的模型层入口，不复用 `descentLayerIndex`

在 `model/channel_cache.go` 增加兜底专用选择入口（输入请求分组、模型、兜底层索引、渠道约束与兜底渠道 ID 列表；输出该兜底优先级层内加权随机选中的渠道）：

- 候选 = 兜底渠道池 ∩ 该分组的该模型可用渠道 ∩ 请求渠道约束；层序按兜底渠道自身优先级降序，层索引直接取兜底段内的尝试序号，**不取模回绕**。
- 层内选择复用正常段既有的权重平滑与加权随机语义。
- 必须同时实现内存缓存路径与 DB 路径（`common.MemoryCacheEnabled == false` 时走 `model/ability.go` 的 `getChannel`），两条路径行为一致。

理由：把兜底层追加进 `sortedUniquePriorities` 再复用同一索引，会让环形回绕跨越正常层与兜底层，破坏既有 spec 场景「原渠道失败后逐层向下并回绕」；锚点也可能落到兜底层。

### D3. 兜底渠道在候选构建阶段就退出正常候选池

在正常候选收集处从候选 ID 集合中减去兜底渠道池；内存路径与 DB 路径都要做。该处是初始选择与重试选择的公共漏斗。

理由：两个集合不相交后，「兜底渠道不参与渠道亲和缓存」与「兜底渠道不作为正常候选」都变成结构性事实，而不是运行期判断。即使配置上重叠，以兜底池优先。

### D4. 亲和与锚点隔离采用「身份判定」

`service/channel_affinity.go` 的 `RecordChannelAffinity` 与 `rememberDescentAnchor` 在入口判断本次渠道是否属于兜底池，属于则直接返回，不写亲和缓存、不写 descent 锚点。

理由：写回点在中间件与 WS 事件路径，读不到 `RetryParam`，也无法依赖「设置/清除上下文标记」的时序（`SwitchOnSuccess` 分支会用 `c.GetInt("channel_id")` 覆盖传入的 channelID）。身份判定不依赖时序，HTTP、WS 与将来新增的调用点自动一致。兜底池是静态整数列表，用 map 做 O(1) 成员判定；`setting/operation_setting` 不依赖 `model`，可在此直接读取。

### D5. `strict` 的两个出口都改为「有兜底则进兜底，无兜底则保持现状」

- `service/relay_error.go` 的 strict 分支（位于预算判定之前）：存在可用的兜底层时改为返回继续尝试并标注进入兜底阶段；不存在时保持现有 `strict_session` 停止。
- `service/channel_select.go` 的 strict 且首轮亲和不可用分支：改为优先使用兜底渠道；无兜底渠道时保持原有 503 `strict_session_binding_unavailable`。

理由：`strict` 的原意是「不再尝试其它正常候选渠道」，兜底渠道不属于正常候选池，因此不违背绑定语义；这也是需求已确认的行为（strict 亲和失败后直接进兜底）。未配置兜底池时两条出口逐字符等价于现状。

### D6. `auto` 分组与兜底的可观测性（用户已拍板）

- 兜底阶段在 `auto` 的**所有分组按现有逻辑跑完之后只执行一次**；兜底渠道按「进入兜底阶段时的当前分组」过滤分组、模型与请求约束。单个分组预算耗尽不单独触发兜底。
- 兜底池为空、或没有任何兜底渠道满足当前分组/模型/请求约束（含已全部尝试过）时：行为等价于未配置兜底，并记录一条 reason 为 `fallback_exhausted` 的策略决策事件（复用既有 `RecordPolicyFailure` 机制）。设置页无法提前校验渠道是否匹配具体请求，因此不在配置时拒绝。

### D7. WS Responses 路径在本 change 内一并覆盖

WS 路径取渠道走 `service.SelectChannelForRequest`（即 D5 要改的 503 分支所在），重试判定复用 `service.DecideRelayRetry`。只改 HTTP 会导致 `strict` + WS 仍 503 而 HTTP 走兜底，出现同一配置两套行为。

### D8. 渠道 pin 不启用兜底

pin 表达「本次只允许该渠道」，兜底渠道不满足该约束，因此 pin 请求失败后仍直接终止（`SuppressesRetry` 分支保持原样）。此取舍已在 Open 阶段向用户确认。

### D9. 配置字段与前端

`ChannelAffinitySetting` 新增兜底渠道 ID 列表字段，option 键 `channel_affinity_setting.fallback_channel_ids`（JSON 数组字符串）：

- 后端：`model/request_policy.go` 的亲和字段白名单与默认值登记；值走既有 `json.RawMessage` 解码分支；负数或非法 ID 在请求期按无匹配处理。
- 前端：类型、表单读写、默认值与保存 delta 比较同步；选择控件复用现有通用多选组件并把渠道 ID 映射为字符串值，渠道列表取自既有渠道接口；7 个语言包补齐文案。

### D10. 未配置兜底池时行为逐字符等价现状

兜底池为空时：不进兜底阶段、不改变任何停止原因、不产生兜底相关决策事件。既有 `descent` 环形回绕、`strict` 503、pin 语义全部保持。

## Risks / Trade-offs

- [抽离一次尝试的主体可能改变 HTTP 与任务两条循环的既有行为] → 重构后先跑既有 `controller/channel_pin_retry_test.go`、`relay/responses_websocket_test.go` 与 relay 相关测试；把「抽取前后行为一致」作为独立验证项，不靠新增测试掩盖。
- [独立兜底循环让失败路径变长，可能撞上游超时] → 兜底段长度受兜底层数约束（每层一次尝试），且仅在配置了兜底池时存在；日志与决策事件保留完整尝试序列。
- [兜底渠道退出正常池后，正常阶段可用渠道减少，可能提前耗尽正常候选] → 有意为的行为变化，需求已确认；未配置兜底池时不受影响。
- [内存缓存与 DB 路径行为不一致] → D2/D3 两条路径同步实现，并各加一组用例（`MemoryCacheEnabled` 两种取值）。
- [兜底渠道被同时列入正常池与兜底池] → 以兜底池优先：进入兜底池即退出正常候选池。
- [前端误填无效渠道 ID] → 请求期按分组、模型与约束跳过，不报错；以 `fallback_exhausted` 决策事件提示确实未命中。

## 验证策略

- `model` 层：兜底候选入口的层序（降序、不回绕）、层内加权随机、正常候选排除兜底渠道；覆盖内存缓存与 DB 两条路径。
- `service` 层：`strict` 有兜底/无兜底两种出口、正常段预算用尽后进入兜底、兜底不可用时 `fallback_exhausted`、未配置兜底时既有停止原因不变。
- 隔离：兜底渠道成功不写亲和缓存、不写 descent 锚点（HTTP 与 WS 各一例）。
- 集成验收：按 spec 的验收场景断言 `use_channel` 重试链路日志为 `50->40->30->20->10->50->66->55`。
- 既有回归：`service/channel_select_auto_groups_test.go`、`service/channel_affinity_template_test.go`、`service/relay_error_test.go`、`model/request_policy_test.go` 与前端 `request-policies` 测试全部通过。

## Migration Plan

无数据库 schema 变更。新增配置字段默认空，部署后行为与现状一致；回滚方式为清空兜底渠道池或回退代码。

## Open Questions

无。`auto` 分组下的兜底时机与兜底不可用时的可观测性已由用户拍板（见 D6）。

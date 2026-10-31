# Design: fix-upstream-sync-affinity-conflict

## 修复方案

以「上游机制为唯一偏移实现，本地语义作为例外条件并入」融合冲突，不并列保留两套偏移逻辑。

### 1. `service/channel_affinity.go`

- 常量块取并集：保留上游 `ginKeyChannelAffinityRetryOffset`（int，命中亲和置 1）与本地 `ginKeyChannelAffinityDescent`（descent 锚点层）；删除本地 `ginKeyChannelAffinityRetryBase`（被 offset 取代，避免同一事实两份状态）。
- `MarkChannelAffinityUsed`：保留上游 `c.Set(ginKeyChannelAffinityRetryOffset, 1)` 与注释「Affinity consumes an attempt, but does not visit a normal priority tier.」。
- 新增包内辅助 `affinityRetryOffset(c *gin.Context) int`：`c == nil` 或 `RequestPolicy(c).SessionMode == "descent"` 时返回 0，否则返回 `c.GetInt(ginKeyChannelAffinityRetryOffset)`。descent 模式的层下标是「锚点层 + retry」，减偏移会重选原层，因此必须恒为 0。
- `AffinityConsumedFirstTry`：改为 `affinityRetryOffset(c) > 0`，保留函数名与既有测试。
- `AffinityAdjustedRetry`：改为 `max(retry-affinityRetryOffset(c), 0)`，保留函数名与既有测试；`retry=0`、无亲和、nil ctx 均不偏移，行为与本地现状一致。

### 2. `service/channel_select.go`

保留本地 `CacheGetRandomSatisfiedChannel`（包装，传 `param.GetRetry()`）+ `CacheGetRandomSatisfiedChannelWithPriority(param, priorityRetry)` 的结构与 descent 下行分支，采用上游的「循环计数 / 层索引」双变量：

- 函数入口取 `affinityRetryOffset := affinityRetryOffset(param.Ctx)`（descent 时为 0），并按本地逻辑计算 `downward` / `anchorGroup`。
- auto 循环内：
  - `groupRetry := param.GetRetry()`，只驱动跨组状态机（`crossGroupRetry && groupRetry >= common.RetryTimes` 等），与上游一致；
  - `i > startGroupIndex` 时同时把 `groupRetry` 与 `priorityRetry` 归零（新分组从最高优先级开始）；
  - 层索引 `priorityRetry = max(priorityRetry-affinityRetryOffset, 0)`；
  - 切换分组时 `param.Ctx.Set(ginKeyChannelAffinityRetryOffset, 0)` 并把局部 `affinityRetryOffset` 归零（上游语义：新分组不受亲和偏移影响，`want: [27,24,34,37]` 依赖此点）。
- 非 auto 分支：层索引 `priorityRetry = AffinityAdjustedRetry(param.Ctx, priorityRetry)`，等价于上游 `max(param.GetRetry()-offset, 0)`，同时保留 descent 例外。
- descent 分支仍调用 `model.GetRandomSatisfiedChannelDescending(..., anchorPriority, priorityRetry, ...)`。

### 3. `controller/relay.go`

删除 controller 层预偏移，改为直接调用包装：

```go
channel, selectGroup, err := service.CacheGetRandomSatisfiedChannel(retryParam)
```

偏移只在上面的 service 内部发生一次，避免 controller 与 service 各减一次。

### 4. 7 个 `web/src/i18n/locales/*.json`

按两侧 key 并集合并（本地新增 key + 上游新增 key，保持文件内既有排序），不删除任何一侧 key；合并后用 JSON 解析与「两侧 key 并集」逐文件核对。

### 5. rerere 种子

合并提交生成后执行 `cp -r .git/rr-cache/* .github/rerere-cache/`，`git add .github/rerere-cache` 后用 `git commit --amend --no-edit` 折入合并提交，保证 CI 恢复种子后能直接重放本轮解决方案。

## 风险与权衡

- 上游新增的 `TestChannelAffinityRetryPriorities` 是跨数据库矩阵测试（sqlite 默认跑，MySQL/PostgreSQL 由环境变量启用），它同时覆盖 affinity hit / auto group / cross group / strict session 等场景，是本方案的主要回归护栏；本地 `TestAffinityAdjustedRetry`、`TestAffinityRetrySelectsTopPriorityChannel`、`TestCacheGetWithPriorityAffinityOffsetKeepsStateMachine`、`TestCacheGetRandomWithPriorityKeepsCrossGroupRetryState` 用于确认本地 descent/兜底语义未回归。
- 不采用「保留两套偏移」：会导致 controller 与 service 双重减 1，亲和命中后的首次重试直接跳到第 2 优先级层。
- 不采用「只保留本地 controller 预偏移」：上游测试直接调用 service 函数，且跨组切换后本地布尔标志不会归零，会重复选中同一层。

## 验证

- `GOWORK=off go vet ./...`（root）、`(cd relaykit && GOWORK=off go vet ./...)`、`(cd tokenkit && GOWORK=off go vet ./...)`。
- `go test ./service/... ./controller/...`（覆盖上游新测试与本地亲和/兜底测试）。
- `bun --cwd web run typecheck`；如改动仅涉及 locale JSON，另跑一次 `bun --cwd web run i18n:check`（若脚本存在）。
- 根因核对：`grep` 确认 `AffinityAdjustedRetry` 只在 service 内使用、controller 不再预偏移；`git diff --name-only --diff-filter=U` 与 `git diff --check` 无残留冲突。

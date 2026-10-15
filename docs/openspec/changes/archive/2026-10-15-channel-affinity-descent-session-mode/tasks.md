# Tasks

> 行为测试落点遵循项目 `AGENTS.md`：同一小功能不跨层新建多个测试文件，环形行为统一在 service 层现有测试文件中验证。

## 1. model 层：环形层下标解析（对应设计 D1）

- [x] 1.1 把 `model/channel_cache.go` 的 `uniquePriorities/sortedUniquePriorities` 与 `model/ability.go` 的 `priorities` 统一为 `[]int64`，并新增包内函数 `descentLayerIndex(prioritiesDesc []int64, anchorPriority *int64, retry int) int`：`anchorPriority == nil` 时保持历史语义（`min(retry, len-1)`）；否则用 `sort.Search` 求出「严格高于锚点层的层数」`above`，返回 `(above+retry) % len(prioritiesDesc)`。验证：`go build ./model/...` 通过，且两条路径的降序排列与原有 clamp 行为不变（阅读 diff 确认仅等价改写）。 <!-- comet-task:4a5a66ef-ff8b-4934-be15-907b1dd01ad0 -->
- [x] 1.2 把 `GetRandomSatisfiedChannel` 与 `GetChannel` 的函数体改为接收锚点参数的未导出核心，并各新增一个导出包装 `GetRandomSatisfiedChannelDescending(group, model string, anchorPriority int64, retry int, filters []dto.ChannelFilter)`、`GetChannelDescending(...)`；原有两个导出函数保留原签名，内部以 `nil` 锚点调用核心。验证：`go build ./...` 与 `go vet ./model/...` 通过，且 `git diff --stat` 中不出现任何已有调用点（2 个生产 + 12 个测试）的改动。 <!-- comet-task:0d73c86b-3e0d-4013-9872-19c0827ac2a3 -->

## 2. service 层：锚点记录、retry 语义与环形接线（对应设计 D2/D3/D4）

- [x] 2.1 在 `service/channel_affinity.go` 新增 gin key 常量与 `rememberDescentAnchor(c *gin.Context, group string, channel *model.Channel)`、`descentAnchor(c *gin.Context) (group string, priority int64, ok bool)`：仅在 `RequestPolicy(c).SessionMode == "descent"` 且尚未记录时写入，其余情况为空操作。验证：`go build ./service/...` 通过，且函数体不含日志以外的副作用（阅读确认其它模式下不写 gin key）。 <!-- comet-task:50d520bf-51ef-4b03-9ce4-7ee23121dd88 -->
- [x] 2.2 在两个调用点接入 `rememberDescentAnchor`：`service/channel_select.go` 的 `SelectChannelForRequest` 唯一 return 之前，以及 `CacheGetRandomSatisfiedChannelWithPriority` 选中渠道之后。验证：`go build ./...` 通过，且两个调用点均无新增分支（helper 自身按模式短路）。 <!-- comet-task:929abcca-5c69-4c78-9b74-90ea178cf201 -->
- [x] 2.3 修改 `service/channel_select.go` 的 `CacheGetRandomSatisfiedChannelWithPriority`：读取模式与已记录锚点，仅在「模式为 descent」且「锚点分组等于当前候选分组」时调用 `...Descending`（auto 分组循环中逐组判断），否则走原有普通调用。验证：`go build ./...` 通过；非 descent 模式与跨分组的调用参数与原实现逐字一致（阅读 diff 确认）。 <!-- comet-task:2a968a78-475e-41d9-81b9-5bc58ba93bcf -->
- [x] 2.4 在 `service/channel_affinity.go` 的 `AffinityAdjustedRetry` 条件中加入 `RequestPolicy(c).SessionMode != "descent"`。验证：`go test ./service/ -run AffinityAdjustedRetry` 通过，且现有 prefer/strict 断言不变。 <!-- comet-task:75c84329-8b1a-46fd-a474-43f79eca2ebe -->
- [x] 2.5 扩展 `service/channel_select_auto_groups_test.go`：新增表驱动用例，构造至少 3 个优先级层（含锚点层位于中间与位于最低层两种输入），断言环序为「retry=0 落在锚点层 → 每次 retry 下降一层 → 到最低层后回绕到最高层 → 回到锚点层」。同一张表在 `common.MemoryCacheEnabled=true` 与 `false` 下各跑一次以覆盖缓存路径与数据库路径；若夹具无法安全切换该全局量，则改为在 `model/task_plugin_channel_select_test.go` 补一条 `GetChannelDescending` 的顺序断言。验证：`go test ./service/ -run 'ChannelSelect|Descent'` 通过。 <!-- comet-task:bda21a37-ebcf-483f-b75c-56e46771b8a8 -->
- [x] 2.6 在同一测试文件中补 `descent` 模式下 `AffinityAdjustedRetry` 不做 `retry-1` 前移、以及无亲和命中时环序退化为常规降序的用例。验证：`go test ./service/ -run 'Descent|AffinityAdjusted'` 通过。 <!-- comet-task:2e5a52c4-0acd-47c3-914c-6f4b0fc4e011 -->

## 3. 后端配置白名单（对应设计 D5）

- [x] 3.1 在 `model/request_policy.go` 的全局与规则级两个 `switch` 取值列表中加入 `"descent"`，并同步 `setting/operation_setting/channel_affinity_setting.go` 中列举会话模式的注释。验证：`go build ./...` 通过；`grep -n 'descent' model/request_policy.go setting/operation_setting/channel_affinity_setting.go` 两个文件均命中。 <!-- comet-task:027b3a56-5029-4a8b-89ed-db9de4d1e6cc -->
- [x] 3.2 扩展 `model/request_policy_test.go`：断言全局与规则级均接受 `descent` 并原样保留取值，且未知取值仍返回错误。验证：`go test ./model/ -run RequestPolicy` 通过。 <!-- comet-task:90c66bb7-1036-45c0-a6cd-4de41703106d -->

## 4. 前端枚举与文案（对应设计 D5/D6）

- [x] 4.1 在 `web/src/features/system-settings/general/channel-affinity/types.ts` 的 `SessionMode` union 与 `request-policies/routing-form.ts` 的 zod `enum` 中加入 `descent`。验证：`cd web && bun run typecheck` 通过且无新增报错。 <!-- comet-task:d8c95072-57f1-4ea7-aa76-a178b43af94a -->
- [x] 4.2 在 `request-policies/routing-section.tsx` 的全局会话模式单选项列表与 `rule-editor-dialog.tsx` 的规则下拉中新增 `descent` 选项（复用现有 `NativeSelect` 结构，不新增组件）。验证：`cd web && bun run lint` 无 error，且下拉与单选组均可见该选项（`grep -n descent` 命中两个文件）。 <!-- comet-task:1e55a395-73f1-4195-87cd-7690aaba0f5a -->
- [x] 4.3 在 `request-policies/policy-label.ts` 增加 `descent` 的文案分支，并确认渲染全局默认值处传给 `Global default: {{mode}}` 的是已翻译标签而非原始取值。验证：`cd web && bun run typecheck` 通过；`grep -n 'Global default' web/src/features/system-settings/general/channel-affinity/rule-editor-dialog.tsx` 确认传参来源。 <!-- comet-task:049b64b8-f441-494b-b6ed-8906563760eb -->
- [x] 4.4 在 `web/src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json` 补齐英文源串 `Prefer the original channel, then descend through priority layers` 的翻译（中文「优先原渠道，失败后逐层降级」），可先运行 `cd web && bun run i18n:sync` 生成键位再填值。验证：7 个文件中该键均存在且非空，非英文语言包不出现与英文完全相同的占位值。 <!-- comet-task:d78fab63-bd72-48a8-9434-072a019783c5 -->
- [x] 4.5 扩展 `web/src/features/system-settings/request-policies/__tests__/routing.test.tsx`：新增 `descent` 的保存与回显往返用例（含全局默认与规则级各一条）。验证：`cd web && bun run test` 通过。 <!-- comet-task:7c1efba8-f4d3-48f1-a456-693a5e897fa4 -->

## 5. 集成验收

- [x] 5.1 后端全量构建与测试：`go build ./...` 与 `go test ./service/... ./model/... ./controller/... ./relay/...` 全部通过，记录实际输出。 <!-- comet-task:7285a088-43ea-4271-ac08-2815da974ba8 -->
- [x] 5.2 `relaykit/`、`tokenkit/` 独立性不受影响（本改动未触碰，仍需 `GOWORK=off go build ./...` 各跑一次确认未误改）。 <!-- comet-task:b9db2e22-25f9-42ec-9674-870697bb47c7 -->
- [x] 5.3 前端整体检查：`cd web && bun run typecheck && bun run lint && bun run build:check && bun run test` 全部通过，记录实际输出。 <!-- comet-task:0ca9f69e-7df9-4b17-8967-484ce654e8b2 -->
- [x] 5.4 规范核对：逐条比对 `specs/channel-affinity/spec.md` 的 5 条 Requirement 与 `specs/channels/spec.md` 的 MODIFIED 需求，确认 `prefer`/`strict`/channel pin 行为未改变，且既有「亲和渠道失败后的重试回归正常优先级选择」在非 descent 路径仍然成立。 <!-- comet-task:7a4025a8-9812-4bb5-b60d-0e5fb2b358b0 -->

## Workflow follow-up

- 在项目评审要求满足后归档该 change，并按用户确认的交付方式处理提交。

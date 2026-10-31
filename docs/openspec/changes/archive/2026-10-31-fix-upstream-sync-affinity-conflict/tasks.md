# Tasks: fix-upstream-sync-affinity-conflict

## 1. 复现（RED 证据）

- [x] 1.1 记录失败证据：issue #18 对应 run `41001911802`（2026-10-30T04:15:58Z）在 step 7「Merge upstream into main (three-way + rerere)」以 `conflict` 失败；`git fetch upstream` 后 `git merge-tree --write-tree --name-only HEAD upstream/main` 复现冲突，冲突文件为 `service/channel_affinity.go`、`service/channel_select.go` 与 7 个 `web/src/i18n/locales/*.json`（比 issue 记录多 7 个，因上游在 issue 生成后新增 24 个提交）。实测：以 `a2defd2a8` 为目标重跑 `merge-tree` 只冲突两个 Go 文件，确认 i18n 冲突是上游后续提交新增；交付物是合并结果本身，无独立单测可先写，回归证据改用 `merge-tree` 预演 + 上游新增测试 `TestChannelAffinityRetryPriorities` 作为 RED 基线。

## 2. 执行合并并解决冲突

- [x] 2.1 执行 `git merge --no-edit upstream/main`，确认冲突文件集合与 `merge-tree` 预演一致。实测：`git diff --name-only --diff-filter=U` 为 9 个文件（2 Go + 7 i18n），与预演一致；rerere 为 9 个文件记录 preimage。
- [x] 2.2 融合解决 `service/channel_affinity.go`：常量块并集（保留上游 `ginKeyChannelAffinityRetryOffset` 与本地 `ginKeyChannelAffinityDescent`，删除 `ginKeyChannelAffinityRetryBase`）、`MarkChannelAffinityUsed` 采用上游 offset、新增 `affinityRetryOffset` 辅助并让 `AffinityConsumedFirstTry`/`AffinityAdjustedRetry` 基于它（descent 恒为 0）。实测：`grep ginKeyChannelAffinityRetryBase` 无结果；`gofmt -l` 无输出。
- [x] 2.3 融合解决 `service/channel_select.go`：auto 分支采用上游 `groupRetry`/`priorityRetry` 双变量 + 跨组归零 offset，保留本地 `WithPriority` 与 descent 下行分支；非 auto 分支用 `AffinityAdjustedRetry` 计算层索引；`controller/relay.go` 删除预偏移改为 `service.CacheGetRandomSatisfiedChannel(retryParam)`。实测：`grep` 确认 controller 无 `AffinityAdjustedRetry`，偏移只在 service 内应用一次。
- [x] 2.4 解决 7 个 `web/src/i18n/locales/*.json`：并集合并两侧 key。实测：逐文件解析成功，每个文件 7650 个 key，与「ours ∪ theirs」完全一致；7 个文件 key 集合相同；两侧同 key 不同值数量为 0。
- [x] 2.5 确认无残留冲突标记：`git diff --name-only --diff-filter=U`、`git diff --check`、`git grep '^<<<<<<< '` 均为空后 `git add` 10 个交付文件。

## 3. 验证

- [x] 3.1 `GOWORK=off go vet ./...`（root）、`(cd relaykit && GOWORK=off go vet ./...)`、`(cd tokenkit && GOWORK=off go vet ./...)` 全部通过。实测：三条命令退出码均为 0。
- [x] 3.2 `go test ./service/... ./controller/...` 通过。实测：退出码 0（`service` 1.402s、`service/authz` 0.069s、`controller` 45.751s）；上游 `TestChannelAffinityRetryPriorities` 的 sqlite 子用例（含 affinity hit / auto group / cross group / strict session）全部 PASS，mysql/postgres 子用例因未配置 `TEST_MYSQL_DSN`/`TEST_POSTGRES_DSN` 跳过；本地 `TestAffinityAdjustedRetry`、`TestAffinityRetrySelectsTopPriorityChannel`、`TestCacheGetWithPriorityAffinityOffsetKeepsStateMachine`、`TestCacheGetRandomWithPriorityKeepsCrossGroupRetryState` 全部 PASS。
- [x] 3.3 `bunx tsc -b web/tsconfig.json` 通过。实测：退出码 0。
- [x] 3.4 根因核对：`git merge-base --is-ancestor upstream/main HEAD` 为真；`controller/relay.go:434` 直接调用包装函数；偏移仅在 `service/channel_affinity.go` 置位、在 `service/channel_select.go` 内消费一次；i18n 7 个文件 key 集合一致（7650）。

## 4. 提交与 rerere 种子

- [x] 4.1 生成合并提交。实测：首次 `git commit --no-edit` 得到 `7ce82680f`，parents 为 `a4f0715ea` + `ea343c4a8`。
- [x] 4.2 回填 `.github/rerere-cache/` 并折入合并提交。实测：`comm` 对比得到 9 个新 hash 目录，均含 `preimage`/`postimage`；9 个 `postimage` 的 `git hash-object` 与提交内对应文件的 blob 一一相等；amend 后合并提交为 `8961e0015`，parents 不变，149 files changed。

## 5. 交付（归档阶段执行）

push 到 `origin/main` 并在 `/comet-archive` 的归档前最终确认中选择交付方式后执行；push 后触发 Sync Upstream（`workflow_dispatch`）确认走短路分支成功，再关闭 issue #18。

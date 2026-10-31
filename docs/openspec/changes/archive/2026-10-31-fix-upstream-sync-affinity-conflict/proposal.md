# Proposal: fix-upstream-sync-affinity-conflict

## 问题描述

`.github/workflows/sync-upstream.yml` 每日把上游 `QuantumNous/new-api` 的 `main` 三方合并进本 fork（`thntime/new-api`）的 `main`。run [41001911802](https://github.com/thntime/new-api/actions/runs/41001911802)（更新于 2026-10-30T04:15:58Z）在 step 7「Merge upstream into main (three-way + rerere)」失败，失败类型 `conflict`，workflow 自动创建 issue #18 要求人工处理。issue 记录的冲突文件为 `service/channel_affinity.go`、`service/channel_select.go`。

## 背景与现状

- `HEAD` = `origin/main` = `a4f0715ea`（上一个 change 归档提交）；`upstream/main` = `ea343c4a8`；merge-base = `34003e9c2`，`HEAD..upstream/main` 共 32 个提交。
- issue 创建时上游顶端是 `a2defd2a8`。此后上游又推进 24 个提交（`a2defd2a8..ea343c4a8`，含 `c59f6fd55 feat(plugins): serve task plugins at the vendors' own paths` 等插件/文案改动），因此本地 `git merge-tree --write-tree --name-only HEAD upstream/main` 预演出的冲突集合比 issue 记录更大：
  - `service/channel_affinity.go`
  - `service/channel_select.go`
  - `web/src/i18n/locales/{en,fr,ja,ru,vi,zh-TW,zh}.json`（7 个）
  - 其余 `controller/relay.go`、`router/api-router.go`、`web/src/hooks/use-sidebar-data.ts`、`web/bun.lock` 等可自动合并。
- 以 `a2defd2a8` 为目标重跑 `merge-tree` 只冲突上述两个 Go 文件，可确认：Go 冲突是 issue 记录的原冲突，7 个 i18n 冲突是上游在 issue 生成后新增的。
- `.git/rr-cache` 中没有这两个 Go 文件本轮的 postimage 种子；workflow 有意不做 `-X ours` 整文件兜底（会静默丢弃上游新增符号），因此合并中止并交人工处理。

## 根因分析

| 冲突 | 上游改动 | 本地改动 | 冲突成因 |
|------|----------|----------|----------|
| `service/channel_affinity.go` | `fcc241cdf fix: start affinity fallback at the highest channel priority (#8078)` 新增 `ginKeyChannelAffinityRetryOffset`（int，命中亲和时置 1） | 本地亲和/兜底定制新增 `ginKeyChannelAffinityRetryBase`（bool）、`ginKeyChannelAffinityDescent`，以及 `AffinityConsumedFirstTry` / `AffinityAdjustedRetry` 辅助函数（在 controller 层做 `retry-1`，descent 模式不偏移） | 两侧都在同一个常量块与 `MarkChannelAffinityUsed` 的同一位置写入「亲和消耗了首轮尝试」这一事实，键名与类型不同 |
| `service/channel_select.go` | 同一提交在 `CacheGetRandomSatisfiedChannel` 内引入 `groupRetry`（循环计数）与 `priorityRetry = max(groupRetry-offset, 0)`（层索引），跨组切换时把 offset 归零 | 本地把该函数拆成 `CacheGetRandomSatisfiedChannel` 包装 + `CacheGetRandomSatisfiedChannelWithPriority(param, priorityRetry)`，并新增 descent 下行分支；偏移由 controller 预先算好再传入 | 同一段循环的层索引计算被两侧以不同机制改写：上游在函数内减 offset，本地在函数外减 retry |
| 7 个 `web/src/i18n/locales/*.json` | 插件原生路由等改动新增若干 key | 本地新增功能也新增若干 key | flat JSON 相邻行插入，按行三方合并产生冲突；无同 key 不同值 |

两个 Go 文件的冲突是语义重叠而非纯文本重叠：若两侧机制都保留，偏移会作用两次；若只保留本地机制，上游新增测试 `controller/channel_pin_retry_test.go::TestChannelAffinityRetryPriorities` 会失败——它直接调用 `service.CacheGetRandomSatisfiedChannel`，并断言跨组场景（`want: [27,24,34,37]`）在切换分组后 offset 归零、新分组从最高优先级重新开始。

## 修复目标

- 融合解决 9 个冲突文件，两侧语义都不丢：
  - 以「上游函数内 offset + 跨组归零」作为层索引偏移的唯一实现；
  - 把本地 descent 例外并入 offset 计算（descent 模式 offset 恒为 0）；
  - 删除 controller 层预偏移，避免双重偏移。
- `GOWORK=off go vet ./...`（root、`relaykit`、`tokenkit`）通过；`service`、`controller` 相关 Go 测试通过；前端 `bun --cwd web run typecheck` 通过。
- 提交合并结果并回填 rerere 种子；push 后 Sync Upstream 重跑走短路分支成功，据此关闭 issue #18。

## 非目标

- 不修改合并策略、不回退到 `-X ours`、不重构 `sync-upstream.yml`。
- 不重构本地 descent 会话模式、兜底池、`WithPriority` 解耦等既有功能。
- 不新增功能、公共 API 或结构化数据格式变更（i18n 只做两侧 key 并集）。
- 不顺手升级依赖、格式化无关文件或处理与本次合并无关的仓库脏文件（`docs/openspec/changes/archive/**` 归档产物、`.worktrees/`）。

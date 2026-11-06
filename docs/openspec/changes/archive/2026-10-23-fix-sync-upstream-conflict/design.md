---
comet_change: fix-sync-upstream-conflict
role: technical-design
canonical_spec: openspec
archived-with: 2026-10-23-fix-sync-upstream-conflict
status: final
---

# 设计：解决 sync-upstream 合并冲突

## 背景与目标

`Sync Upstream` workflow（run 39894144682 / issue #16）在 `git merge --no-edit upstream/main` 步骤因 8 个冲突文件失败，每日自动同步停摆。本次设计的目标是：在本地 `main`（`304a68e`）上完成 `upstream/main`（`7fb6e57a5`）的三方合并，冲突按两侧语义融合，验证通过后推送并恢复 workflow。需求与验收以 `proposal.md` 为准。

## 冲突一：`controller/ratio_sync_test.go`（修改/删除）

### 事实

- 该文件由上游在本地分叉之后新增；本地 main 的 first-parent 链上从未存在过它，因此三方合并始终表现为「我方删除」。此前上游未再修改该文件，合并不报冲突；本次上游 `eb169e74d` 为 `TestPricingSyncCompleteSourcesAndArrayFormats` 追加 `include_unchanged` 断言，触发修改/删除冲突。
- 上游 `IncludeUnchanged` 实现位于 `controller/ratio_sync.go`，本次合并中**已自动并入**本地（无冲突）。
- 本地 `e0880740a` 改造了 `FetchUpstreamRatios`：新增 `model.GetAllModelNames()` 过滤，只保留模型库中已存在的模型。上游测试没有初始化 `model.DB`，直接取回上游版本会 panic；即使补上 DB 初始化，测试数据中的模型名不在模型库中也会断言失败（已实测）。

### 候选方案

| 方案 | 做法 | 代价 |
| --- | --- | --- |
| A. 恢复上游版本 + 本地 fixture（采用） | 取 `upstream/main` 版本，在请求前补 `setupRatioSyncTestDB(t)` 与 5 个模型种子 | 上游文件带一处本地适配，未来上游再改该文件会再冲突；但保留上游全部回归断言 |
| B. 沿用删除 | 冲突按删除解决 | 零维护成本，但 `include_unchanged` 与表达式优先级等上游行为失去回归保护 |
| C. 在包级 `TestMain` 统一初始化 DB | 不改上游文件 | 会给整个 `controller` 包所有测试强加 SQLite 初始化，影响面远大于本问题 |

### 采用方案

方案 A：

- `git show upstream/main:controller/ratio_sync_test.go` 恢复文件。
- 在 `TestPricingSyncCompleteSourcesAndArrayFormats` 发起请求前：
  - 调用 `setupRatioSyncTestDB(t)`（同包 `ratio_sync_existing_models_test.go` 提供，初始化 `model.DB` 与 `model.Model` 表）；
  - 种入断言期望保留的模型名：`sync-token`、`sync-expression`、`sync-array-expression`、`sync-free`、`sync-already`。
- **不修改任何断言**。`sync-unpriced`、`sync-invalid-expression` 期望缺席，无需种入。

验证：`GOWORK=off go test ./controller -count=1` 全绿（43.9s）。

## 冲突二：7 个 locale JSON（内容冲突）

### 事实

- 冲突文件：`web/src/i18n/locales/{en,fr,ja,ru,vi,zh-TW,zh}.json`。
- 三方比对结果：本地新增 278 个键，上游新增 22 个键，**同名键取值冲突为 0**，冲突纯粹由两侧在同一锚点插入造成。
- `web/scripts/sync-i18n.mjs` 以叶子键最多的 `en.json` 为基准顺序，重排其余语言；因此键的物理顺序不是语义契约，但键集完整性是（`missingCount`/`extrasCount`）。

### 采用方案：键集并集

1. 用 `git show :1:/:2:/:3:` 取 base/ours/theirs 的 `translation` 对象。
2. 合并：两侧都改的键取实际改动侧；仅一侧新增的键全部保留；断言无同名键取值冲突（脚本会在有冲突时报错）。
3. `en.json` 顺序：保留本地既有键序，把上游新增键插入到其在 `upstream/main` 中的前一个键之后。
4. 其余 6 个语言：键集与 `en.json` 完全对齐（7637 键），顺序按新的 `en.json` 归一——与 `sync-i18n.mjs` 的归一行为一致。
5. 序列化复用 `sync-i18n.mjs` 的 `stableStringify`（2 空格缩进、末尾换行、`footer.new\u0061pi.projectAttributionSuffix` 混淆写法），保证与工具产物字节一致。

未采用「按 en 全量重排 ours」：会把 278 个本地键的位置全部打散，产生与语义无关的 diff 噪声。

验证：`bun run i18n:sync` 后工作区无变化（幂等），`_sync-report.json` 中 7 个语言 `missingCount=0`、`extrasCount=0`；`bun run typecheck` 通过。

## rerere 种子回填

`sync-upstream.yml` 的 rerere 种子以 `.github/rerere-cache/` 为唯一事实源，CI 无法自动收录人工解决方案：

1. 合并前 `git config rerere.enabled true`、`rerere.autoupdate true`。
2. 解决冲突后，`.git/rr-cache` 新增 7 条记录（locale 文件；修改/删除冲突不产生 rr 记录）。
3. 提交合并后 `cp -r .git/rr-cache/* .github/rerere-cache/`，逐条核对 `postimage` 与合并后文件逐字节一致。
4. `git commit --amend --no-edit` 把种子折入合并提交；不删除已有条目。

## 验证策略

| 项目 | 命令 | 期望 |
| --- | --- | --- |
| 根模块 vet（workflow 关卡） | `GOWORK=off go vet ./...` | 退出码 0 |
| relaykit vet（workflow 关卡） | `cd relaykit && GOWORK=off go vet ./...` | 退出码 0 |
| tokenkit 独立构建 | `cd tokenkit && GOWORK=off go build ./...` | 退出码 0 |
| 冲突文件回归 | `GOWORK=off go test ./controller -count=1` | ok |
| 前端类型 | `cd web && bun run typecheck` | 退出码 0 |
| i18n 归一幂等 | `cd web && bun run i18n:sync && git diff --stat` | 无输出 |
| 价格同步前端测试 | `cd web && bun run test -- src/features/system-settings/__tests__/pricing-sync.test.tsx src/features/system-settings/models/__tests__/upstream-ratio-sync-helpers.test.ts` | 全部通过 |

> 前置：`main.go` 有 `//go:embed web/dist`，`go vet` 前需 `mkdir -p web/dist && touch web/dist/index.html`（workflow 同样处理）。`web/dist` 已被 `web/.gitignore` 忽略。

## 交付与验收

1. 合并提交（`chore(sync): 合并 upstream/main 并解决 ratio_sync 测试与 i18n 冲突`）推送到 `origin main`。
2. `gh workflow run sync-upstream.yml` 重跑；成功判据：`skipped=true`、merge 步骤 skipped，`git merge-base --is-ancestor upstream/main HEAD` 为真。
3. workflow 不会自动关闭 issue，需手工关闭 #16。

## 失败路径

| 失败点 | 表现 | 处置 |
| --- | --- | --- |
| 合并结果编译失败 | vet 非零 | 回到冲突解决，按两侧语义修正，不使用 `-X ours` |
| 上游测试在本地仍失败 | `go test ./controller` 失败 | 先确认是过滤逻辑导致还是断言与本地行为不符；只允许补 fixture，不允许删断言 |
| i18n 非幂等 | `bun run i18n:sync` 产生 diff | 以工具输出为准重新归一，直到无 diff |
| 推送后 workflow 仍失败 | 新 run 未短路 | 按 issue #16 的步骤重新预演 `merge-tree`，检查是否有新上游提交 |

## 范围外

- 不实现上游未合并的新功能，不调整本地价格同步业务规则。
- 不修改 `sync-upstream.yml` 的流程设计。
- 不改变 `web/scripts/sync-i18n.mjs` 的归一规则。

# 修复 Sync Upstream 每日同步冲突（issue #21）

## 问题

`Sync Upstream` workflow（run [42960292980](https://github.com/thntime/new-api/actions/runs/42960292980)）在「Merge upstream into main (three-way + rerere)」步骤失败，并自动创建 issue #21。本地 `git merge-tree --write-tree HEAD upstream/main` 预演结果与 CI 一致，冲突文件为：

- `web/src/i18n/locales/{en,fr,ja,ru,vi,zh-TW,zh}.json`：内容冲突，每个文件 5 处（两侧在同一批锚点新增翻译键）
- `web/src/features/channels/components/drawers/channel-mutate-drawer.tsx`：内容冲突 3 处（本地渠道接入方式回显逻辑 vs 上游新增的模型重定向同步重置）

基线：本地 `main` 为 `ed9175de8`，`upstream/main` 为 `be5e3153d`，merge-base 为 `2de51a19e`；本地领先 16 个提交、落后 345 个提交。

## 根因

1. **locale JSON**：两侧都在同一位置插入翻译键。三方比对（`translation` 层）：本地新增 284 个键、上游新增 18 个键，**同名键取值冲突为 0**，冲突纯粹由插入锚点重叠造成，并集为 7694 个键。`web/scripts/sync-i18n.mjs` 以叶子键最多的 `en.json` 为基准顺序重排其余语言，因此键的物理顺序不是语义契约，键集完整性才是。
2. **`channel-mutate-drawer.tsx`**：本地在 `form.reset(defaults)` 后保留了 VolcEngine(45)、Zhipu V4(26)、Moonshot(25) 的接入方式回显（含存量魔法键归一与自定义 URL 分支）；上游在同一段新增 `resetModelMappingSync(...)` 调用，并把该函数加入 effect 依赖。两侧改动落在同一行区间，形成冲突。
3. **自动化停摆**：`sync-upstream.yml` 在冲突时中止合并、不推送、不兜底取本地侧，必须人工解决后 push，日常同步才能恢复。

## 修复目标

1. 在本地 `main` 完成 `upstream/main` 三方合并，冲突按两侧语义融合，不做整侧覆盖、不使用 `-X ours`。
2. 7 个 locale JSON 取键集并集：本地 284 个键与上游 18 个键全部保留，无取值冲突；序列化与 `bun run i18n:sync` 产物一致（该命令跑完后工作区不再变化）。
3. `channel-mutate-drawer.tsx` 同时保留上游的 `resetModelMappingSync` 调用（含依赖数组）与本地三类渠道的接入方式回显逻辑，冲突标记清零。
4. 通过 workflow 同款关卡：根模块、`relaykit`、`tokenkit` 分别执行 `GOWORK=off go vet ./...` 退出码为 0；前端执行类型检查与相关测试。
5. 回填 rerere 种子到 `.github/rerere-cache/`（唯一事实源）并折入合并提交；推送 `origin/main`，关闭 issue #21。

## 范围外

- 不新增功能，不调整本地渠道接入方式与 i18n 的业务规则。
- 不修改 `sync-upstream.yml` 的流程设计。
- 不处理与本次合并无关的工作区改动。

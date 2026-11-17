# 修复 Sync Upstream 合并冲突（issue #22）

## 问题描述

`.github/workflows/sync-upstream.yml` 每日把上游 `QuantumNous/new-api` 的 `main` 三方合并进本 fork（`thntime/new-api`）的 `main`。本次调度 run [44023137063](https://github.com/thntime/new-api/actions/runs/44023137063)（2026-11-17T05:18:23Z）在「Merge upstream into main」步骤失败并创建 issue #22，冲突类型为 `conflict`，vet 日志为空（尚未进入 vet）。

当前基线：

- 本地 `HEAD` = `fba0c90c1`（`main`），`upstream/main` = `669d77497`，merge-base = `0bda086fb`
- 本地领先 350、落后 17 个提交
- issue 列出的冲突文件共 8 个：`AGENTS.md` + `web/src/i18n/locales/{en,fr,ja,ru,vi,zh-TW,zh}.json`

只读复现 `git merge-tree --write-tree --name-only HEAD upstream/main` 得到与 issue 完全一致的 8 个冲突文件；`controller/model.go`、`model/channel.go`、`model/pricing.go`、`web/package.json`、`web/bun.lock`、渠道抽屉与用量日志组件等其余文件均可自动合并。

## 根因

1. **AGENTS.md**：本地在文件末尾追加了 Comet ambient resume 块（约 20 行）；上游提交 `288dea549`、`f36c460f0` 把 Issues / Pull requests 段落改写为引用 `.agents/github/CHECKS.md` 的新文字。两处改动落在同一文件，git 无法自动合并。
2. **7 个 locale JSON**：本地相对 merge-base 新增 350 个键（CodeBuddy/CodeArts、性能健康页、兜底渠道等本地功能文案），上游新增 58 个键（缓存计费类别与时长标签、插件市场等），两侧都插入到相同的键锚点附近，产生内容冲突。另有三类共有键取值冲突：
   - 缓存计费文案：上游 `bb30732b8` 统一缓存计费类别与时长标签，改写为「cache write / 缓存写入」术语与规范时长格式；
   - 合规提示：上游 `87c8848f3` 把 `root administrator` 改为 `super administrator`；
   - 其余为俄语 `кэш`/`кеш` 等译文归一。
   两侧同名键取值冲突共 7（fr）~8（ja/ru/vi/zh-TW/zh）个，`en` 无取值冲突。

## 修复目标

1. 在本地 `main` 完成 `upstream/main` 三方合并，8 个冲突文件全部按两侧语义融合，不使用 `-X ours` 或整侧覆盖。
2. `AGENTS.md`：采用上游新版 Issues / Pull requests 文字（引用 `CHECKS.md`），保留本地末尾 Comet ambient resume 块。
3. locale：7 个语言均取两侧键集并集（8136 键 = 本地 8078 ∪ 上游 7782），共有键取值冲突采用上游译文；`bun run i18n:sync` 幂等，`_reports/_sync-report.json` 中 7 个语言 `missingCount=0`、`extrasCount=0`。
4. 验证：根模块、`relaykit`、`tokenkit` 的 `GOWORK=off go vet ./...` 退出码均为 0；`bunx tsc -b web/tsconfig.json` 通过；受影响的渠道相关前端测试通过。
5. 回填 rerere 种子并折入合并提交；推送 `origin/main`、关闭 issue #22（归档阶段交付）。

## 范围外

- 不修改 `sync-upstream.yml` 流程本身。
- 不实现上游新功能之外的本地改造，不调整插件市场、缓存计费或 provider plugins 的设计。
- 不处理与本次合并无关的工作区改动。

## Capabilities

本次仅解决合并冲突并恢复既有同步流程，不新增或改变 spec 级行为，因此不创建 delta spec。

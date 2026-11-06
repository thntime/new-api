---
comet_change: fix-upstream-sync-and-add-codebuddy-codearts
role: technical-design
canonical_spec: openspec
archived-with: 2026-11-06-fix-upstream-sync-and-add-codebuddy-codearts
status: final
---

# Design

## Context

见 `proposal.md` 的 Why。当前状态与约束：

- 本地 `main` = `5d0fd1056`，`upstream/main` = `2de51a19e`；merge-base = `45746eee4`，本地领先 340 个提交、落后 10 个提交。
- `git merge-tree --write-tree HEAD upstream/main` 复现 10 个冲突文件：`model/channel.go`、`web/src/features/system-settings/models/model-ratio-form.tsx`、`model-ratio-visual-editor.tsx`、7 个 `web/src/i18n/locales/*.json`。
- 冲突分三类：纯 import 重叠、i18n 相邻行插入、以及本地可视化编辑器/批量清空与上游价格变更复核流程的语义重叠。
- 仓库同步 workflow 明确禁止 `-X ours` 整侧兜底；合并结果必须在根模块与 `relaykit` 分别通过 `GOWORK=off go vet ./...`。
- CodeBuddy 接入方式当前只存在于 `settings.codebuddy_region`，后端校验在 `relaykit/dto/channel_settings.go`，前端枚举在 `web/src/features/channels/lib/channel-form.ts`，选择器在 `channel-mutate-drawer.tsx`；该字段是纯标记，不参与地址推导。
- 上游 `72f3dc7da` 引入价格变更复核、删除保留草稿并可撤销、`buildModelRow`、`price-change-diff.tsx`；上游 `e3e74205f` 引入定价问题引导 `model-pricing-issue-alerts.tsx`。这两个文件与 `model-pricing-snapshots.ts` 的改动不与本地冲突，直接合入。

## Goals / Non-Goals

**Goals:**

- 以三方合并方式把 `upstream/main` 合入 `main`，两侧语义都不丢。
- 让上游「保存前复核价格变更、删除保留草稿并可撤销」与本地「可视化编辑器多选、批量清空」在同一套草稿/保存流程下共存。
- 以最小改动把 `CodeArts` 加入 CodeBuddy 接入方式，存储值 `codearts`，行为与 `cn`/`intl` 完全一致。
- 回填 rerere 种子，push `origin/main`，关闭 issue #20。

**Non-Goals:**

- 不为 `CodeArts` 推导或预填 API 地址，不改请求路径与请求内容。
- 不修改上游新增功能的语义（价格复核、撤销、问题引导等），也不改动本地与上游无关的定制。
- 不改数据库 schema、计费算法、其他渠道类型或其他接入方式。
- 不重构价格编辑器为新的架构；只做冲突融合与必要接线。

## Decisions

### D1：以 `git merge --no-edit upstream/main` 执行合并，逐文件人工融合

采用仓库同步 workflow 相同的三方合并方式，不使用 `-X ours` 或整侧覆盖。合并前开启 rerere，以便记录本轮解决方案并回填种子。

替代方案：按文件挑拣上游提交。被否，因为 340/10 的提交差意味着挑拣会引入大量重复冲突，且与 workflow 行为不一致。

### D2：`model/channel.go` 采用并集，并接受上游 `BatchInsertChannels` 改动

import 冲突同时保留本地 `regexp` 与上游 `slices`；`BatchInsertChannels` 采用上游 `slices.Chunk` 与 `panic(r)` 的写法。该改动与本地在该文件的其他定制无重叠。

### D3：i18n 按 key 并集解决，保持字母序

7 个 locale 的冲突全部是相邻行插入。逐文件取两侧 key 并集，按现有文件顺序插入，不重排全文件、不运行会批量补译的同步命令，避免引入范围外改动。

### D4：价格表单以「上游新流程为骨架 + 回接本地能力」融合

`model-ratio-form.tsx` 以上游版本为基础，保留 `changeRows`、`PriceChangesDialog`、`editorDirty`、`isMobile` 次级操作菜单与统一保存入口；回接本地 `selectedCount`、`onSelectionChange`、可视化模式下的「清空」按钮。

- 清空动作直接写草稿（删除选中模型的定价字段），保存时由统一入口进入价格变更复核。
- 移除本地单独的「清空」二次确认 `ConfirmDialog`：草稿删除可撤销、可丢弃，保存前还有一次复核，重复确认没有额外保护价值。
- 宽屏下「清空」与「Reset prices」同排；窄屏下进入上游新增的次级操作菜单。
- 本地原有的「仅 JSON 模式显示保存按钮」被上游统一保存入口取代。

替代方案：保留本地工具栏与二次确认弹窗，再叠加上游复核对话框。被否，会重复两套保存入口、制造双重确认，并放大冲突面。

### D5：可视化编辑器采用上游行构建与撤销，保留本地筛选语义

`model-ratio-visual-editor.tsx` 采用上游 `buildModelRow`、`handleRestore`（撤销）、`onEditorDirtyChange`、`closeEditor`；保留本地 `onSelectionChange`、`clearSelectedRows` 与按基础定价过滤主列表的语义。

- 筛选条件：`unset` 模式为「未删除且已保存行没有基础定价」；`all` 模式为「有基础定价，或存在定价问题」。删除行因保留 `saved` 定价仍会显示，并提供撤销。
- 清空实现统一为本地 `clearPricingFields(names: string[])`；上游 `handleDelete(name)` 收敛为 `clearPricingFields([name])` 的单元素封装，避免两套删除逻辑。
- 本地行内联构建改为上游 `buildModelRow`，但保留本地筛选表达式。

替代方案：保留本地行构建与 `isDraftDeleted` 过滤。被否，会丢失上游的删除行撤销能力。

### D6：CodeArts 作为第三个标记值，值域 `cn` / `intl` / `codearts`

- 后端：`ValidateCodeBuddyRegion` 接受 `codearts`，空白仍视为未设置；非法值继续报错。
- 前端：zod 枚举、解析与表单类型加入 `codearts`；选择器新增 `CodeArts` 选项；`settings` 写入逻辑不变。
- i18n：7 个语言文件新增 `CodeArts` 键，各语言使用同一专有名词。
- 存储值使用小写 `codearts`，与 `cn`/`intl` 的取值风格一致；显示名保持 `CodeArts`。

替代方案：使用 `codeArts` 原样作为存储值。被否，会打破现有小写枚举约定，且该值对上游无协议含义。

### D7：交付顺序

先完成合并并验证，再实现 CodeArts，最后一次性提交交付：合并提交 → CodeArts + 用户确认并入的历史归档文档提交 → rerere 种子回填提交 → push `origin/main` → 关闭 issue #20。

## 测试策略

- 后端：根模块与 `relaykit` 分别执行 `GOWORK=off go vet ./...`；`relaykit/dto` 单测覆盖 `codearts` 合法值与非法值。
- 前端：`bun run test -- codebuddy-channel model-ratio-table-selection price-changes`；`bun run typecheck`；`bun run lint`；`bun run build`。
- 合并验收：10 个冲突文件无冲突标记；7 个 locale JSON 解析通过；前端构建成功。
- 交付验收：`git rev-parse origin/main` 等于本地 `HEAD`；`gh issue view 20` 状态为 `CLOSED`。

## Risks / Trade-offs

- [移除清空二次确认后误操作] → 草稿删除可撤销、可丢弃，保存前仍有价格变更复核；若实际使用反馈需要，可恢复确认弹窗。
- [融合价格表单时丢失本地批量清空] → 保留现有 `model-ratio-table-selection.test.tsx` 覆盖，并补充清空进入复核的断言。
- [本地「主列表只显示已定价模型」与上游全量列表冲突] → 明确保留本地筛选语义，删除行因保留 `saved` 定价仍可见并可撤销。
- [上游新增文件未随冲突提示出现但被本地同名文件覆盖] → 合并后运行 `GOWORK=off go vet ./...` 与前端 typecheck/build，按编译错误补齐。
- [i18n 并集破坏 JSON 结构] → 合并后解析 7 个文件校验 JSON 合法。
- [rerere 种子回填遗漏] → 合并后对比 `.git/rr-cache` 与 `.github/rerere-cache/`，复制新增目录并提交。
- [push 后 CI 重建镜像或再次触发同步] → 属于既有 workflow 预期行为；推送前确认合并提交通过双模块 vet。

## Migration Plan

无数据迁移。回滚方式：`git revert -m 1 <merge-commit>` 可撤销合并；CodeArts 改动是独立提交，可单独 revert。

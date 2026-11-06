# Tasks

## 1. 上游合并与冲突解决

- [x] 1.1 记录合并基线（`HEAD`、`upstream/main`、merge-base）并执行 `git merge --no-edit upstream/main`，确认冲突文件清单与 issue #20 一致（`git status --short` 显示 10 个冲突文件） <!-- comet-task:f0f41525-32d2-493c-a0e6-d5b8817371a4 -->
- [x] 1.2 解决 `model/channel.go` 冲突：import 同时保留 `regexp` 与 `slices`，接受上游 `BatchInsertChannels` 改动；确认文件无冲突标记且 `GOWORK=off go build ./model/` 通过 <!-- comet-task:d95656a8-ad16-4bb6-9f19-6d5ae4e7971f -->
- [x] 1.3 解决 7 个 `web/src/i18n/locales/*.json` 冲突：按两侧 key 并集插入；确认 7 个文件均无冲突标记且 `python3 -m json.tool` 逐个解析通过 <!-- comet-task:667f05a5-69e3-4f2c-8423-28c6f1863881 -->
- [x] 1.4 融合 `model-ratio-form.tsx`：以上游保存前复核流程为骨架，回接本地批量清空按钮与选中计数（清空写草稿、保存前复核，不保留二次确认弹窗）；确认文件无冲突标记 <!-- comet-task:9b987eb2-82fa-4e3d-a38f-79636d38f316 -->
- [x] 1.5 融合 `model-ratio-visual-editor.tsx`：采用上游 `buildModelRow` 与撤销，保留本地列表筛选语义与 `clearSelectedRows`/`onSelectionChange`；确认文件无冲突标记 <!-- comet-task:a4670fc6-aeca-4801-a0ac-f004ead95206 -->
- [x] 1.6 合并结果双模块验证：根模块与 `relaykit` 分别执行 `GOWORK=off go vet ./...`，两者退出码均为 0 <!-- comet-task:27a46e54-3d0e-411c-96b7-2ed523986993 -->
- [x] 1.7 价格管理前端回归：运行 `bun run test -- model-ratio-table-selection price-changes`（在 `web/`），确认现有选择/清空与上游复核用例全部通过 <!-- comet-task:92f07b04-40fa-45f2-94ad-f5c1ca6f7123 -->
- [x] 1.8 前端类型检查与 lint：在 `web/` 运行 `bun run typecheck` 与 `bun run lint`，受影响文件无 error <!-- comet-task:ec85c47f-e6a5-480e-8937-d1ff3609575b -->

## 2. CodeBuddy 新增 CodeArts 接入方式

- [x] 2.1 后端校验：`relaykit/dto/channel_settings.go` 的 `ValidateCodeBuddyRegion` 接受 `codearts`，非法值仍报错；在 `relaykit/` 运行 `GOWORK=off go test ./dto/...` 通过 <!-- comet-task:33526724-ed12-461d-a0bf-463d31b47f91 -->
- [x] 2.2 前端枚举与选择器：`channel-form.ts` 的 zod 枚举/解析/表单类型加入 `codearts`，`channel-mutate-drawer.tsx` 新增 `CodeArts` 选项且只写 `settings`；运行 `bun run test -- codebuddy-channel` 通过 <!-- comet-task:e6ace28b-d392-4a17-ad1c-ff69703bcb9c -->
- [x] 2.3 i18n：7 个语言文件新增 `CodeArts` 键；确认 JSON 合法、7 个文件均含该键 <!-- comet-task:889da2b7-bfd4-43cc-9162-a0cd90ba225b -->
- [x] 2.4 前端整体校验：在 `web/` 运行 `bun run typecheck`、`bun run lint` 与 `bun run build`，全部成功 <!-- comet-task:35d54739-35df-4035-a7ae-1479e1a4c9b6 -->

## 3. 交付

- [x] 3.1 提交合并结果：生成合并提交，确认提交统计包含冲突文件与上游改动（`git log --oneline -1` 为 merge commit） <!-- comet-task:0e86a5f7-4bb7-4f22-bba6-495afa6b47c3 -->
- [x] 3.2 提交 CodeArts 改动与工作区既有归档文档改动：确认提交只包含本 change 相关文件与用户确认并入的历史归档改动 <!-- comet-task:f821ce02-277e-46d6-8993-68d3d5cba847 -->
- [x] 3.3 回填 rerere 种子：把 `.git/rr-cache` 新增记录复制到 `.github/rerere-cache/` 并提交，确认两目录无差异 <!-- comet-task:a864c191-fa0e-493d-9244-50d5077bfda9 -->
- [x] 3.4 push `origin/main`：确认 `git rev-parse origin/main` 等于本地 `HEAD` <!-- comet-task:fe11a72c-d6d0-4a45-a1cc-b8ada2c5f660 -->
- [x] 3.5 关闭 issue #20：`gh issue view 20 --repo thntime/new-api --json state` 返回 `CLOSED` <!-- comet-task:0f28b70c-da23-40c5-9c5e-d297bce5d6dc -->

## Workflow follow-up

- 通过 Verify 阶段的一次最终集成审查后归档本 change。
- 归档时按 delta 语义合并 `codebuddy-provider` 与 `model-pricing-management` 主 spec。

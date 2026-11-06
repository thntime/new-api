# Tasks: 修复 sync-upstream 冲突策略

## Task 1：复现并记录问题失败证据

- [x] 在隔离 worktree 中执行 `git merge upstream/main`（默认三方 + 复刻 workflow 的 -X ours 兜底），记录：
  - 默认 merge 的 10 个冲突文件列表
  - -X ours 兜底 merge 成功但 `go vet ./...` 失败（`undefined: constant.ChannelTypeVLLM` 等）
- [x] 将失败证据记录到本 proposal/验证报告

## Task 2：手动合并当前上游（issue #7）

- [x] 在 main 上执行 `git merge upstream/main`，获取冲突文件列表
- [x] 逐个解决 10 个冲突文件（保留本地扩展 + 补入上游 VLLM/SGLang/Responses 符号）
- [x] 运行 `GOWORK=off go vet ./...` 与 `cd relaykit && GOWORK=off go vet ./...`，确认零错误
- [x] 前端构建检查（如受影响）：`cd web && bun run build` 或 `bun run tsc`（视可用性）
- [x] 提交合并结果（保留 MERGE_MSG），同步 rerere 种子到 `.github/rerere-cache/`

## Task 3：改造 sync-upstream workflow 防复发

- [x] 修改 `.github/workflows/sync-upstream.yml`：
  - 移除全局 `-X ours` 兜底逻辑
  - 默认三方 merge 冲突 → 中止 + 创建/更新 sync-upstream issue + job 结束（不 push）
  - 保留 vet 验证关卡与 rerere 种子恢复/保存
- [x] 审查 workflow 语法（actionlint 或 YAML 校验，若可用）

## Task 4：推送与验证

- [x] 由用户确认后 `git push origin main`
- [x] 触发一次 `sync-upstream` workflow（workflow_dispatch）验证成功，或核对结果
- [x] 关闭 issue #7
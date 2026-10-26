# Tasks

## 1. 本地完成上游合并

- [x] 1.1 确认本地 `HEAD` 与 `origin/main` 一致，重新 `git fetch upstream`，并用 `git merge-tree --write-tree --name-only HEAD upstream/main` 复核合并无冲突；验证：输出不含 `CONFLICT`，且 `git rev-list --count HEAD..upstream/main` 等于预期提交数。 <!-- comet-task:cf7d913a-15d8-4507-971e-3f0781e250ba -->
- [x] 1.2 执行 `git merge --no-edit upstream/main` 生成合并提交；验证：`git log -1 --format=%P` 显示两个父提交，`git status` 无 `Unmerged` 路径。 <!-- comet-task:54d1898f-330c-4547-b08a-362c8506e00f -->
- [x] 1.3 运行 `GOWORK=off go vet ./...` 与 `(cd relaykit && GOWORK=off go vet ./...)`；验证：两条命令退出码均为 0。 <!-- comet-task:a6b489a9-3c9b-4bfa-97b4-d0cb309848b6 -->
- [x] 1.4 核对 `.git/rr-cache` 与 `.github/rerere-cache` 的差异，确认本次无新增冲突种子；验证：无差异或差异已回填并纳入提交。 <!-- comet-task:3a26aa76-0ea0-4260-a6f5-a4f2b99b2abd -->

## 2. 修复 workflow 推送凭证与失败上报

- [x] 2.1 修改 `sync-upstream.yml`：`actions/checkout` 使用 `token: ${{ secrets.SYNC_UPSTREAM_TOKEN }}`，push 复用同一凭证；验证：`python3 -c "import yaml;yaml.safe_load(open('.github/workflows/sync-upstream.yml'))"` 解析成功，且文件中不再依赖默认 `GITHUB_TOKEN` 执行 push。 <!-- comet-task:6053b383-6ff8-4176-877f-d4e27f638cdd -->
- [x] 2.2 在 merge 前增加 secret 预检步骤，secret 缺失或为空时输出 `::error::` 并退出 1；验证：以空值执行预检脚本片段，退出码为 1 且输出包含 `SYNC_UPSTREAM_TOKEN`。 <!-- comet-task:2f8f04c8-f495-4fd3-a4b4-d975e4bc2bfd -->
- [x] 2.3 增加失败类型分类（`conflict` / `vet` / `push` / `auth`），issue 正文按实际类型给出对应说明；验证：用样例输入执行分类逻辑，`push` 分支正文包含凭证权限提示，`conflict` 分支正文包含冲突文件清单占位。 <!-- comet-task:ac58926c-a9e1-42f2-8eed-09ae3400f872 -->
- [x] 2.4 更新 workflow 顶部注释与步骤说明，使其与新的凭证要求和失败分类一致；验证：逐条对照 `design.md` 的 Decisions 1–3，注释无过时描述。 <!-- comet-task:a5535159-3b09-40f1-8703-2e7cae135ea4 -->

## 3. 本地验证

- [x] 3.1 复核 workflow 的完整步骤顺序（checkout → 预检 → fetch → 短路 → merge → vet → rerere → push → failure），确认没有任何步骤仍会在缺少 `workflows` 权限时推送 workflow 变更；验证：逐步骤阅读 diff 并在验证报告中记录结论。 <!-- comet-task:cf66fc16-ada8-4ab6-b232-f199cccf5a44 -->
- [x] 3.2 重跑 1.3 的 vet 命令确认工作区最终状态仍通过；验证：两条命令退出码为 0。 <!-- comet-task:dbade7d3-49e1-47b2-8eef-2b3d689d262d -->

## Workflow follow-up

- 用户在 GitHub 仓库创建 fine-grained PAT（Contents: write、Workflows: write），保存为 Actions secret `SYNC_UPSTREAM_TOKEN`。
- 推送 `main`（上游合并提交 + workflow 修复提交）。
- 手动触发 `Sync Upstream`，确认在「上游已包含」时短路成功。
- 关闭 issue #17。
- 归档本 change。

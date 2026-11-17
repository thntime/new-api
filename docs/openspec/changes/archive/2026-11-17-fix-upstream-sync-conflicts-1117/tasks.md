# Tasks

## 1. 合并与冲突解决

- [x] 1.1 复现冲突：确认 `HEAD=fba0c90c1`、`upstream/main=669d77497`、merge-base `0bda086fb`，执行 `git merge --no-edit upstream/main`，确认 `git diff --name-only --diff-filter=U` 恰好列出 issue #22 的 8 个文件 <!-- comet-task:05f97bd7-e3c6-4cff-9cea-352af4a378c2 -->
- [x] 1.2 解决 `AGENTS.md`：以上游新版为骨架（Issues / Pull requests 引用 `.agents/github/CHECKS.md`），在末尾保留本地 `<comet-ambient-resume>` 块；验收：无冲突标记，且 `git diff upstream/main -- AGENTS.md` 只显示新增的 Comet 块 <!-- comet-task:471856c5-b086-429f-93de-30c7136a21de -->
- [x] 1.3 解决 7 个 `web/src/i18n/locales/*.json`：逐语言取两侧键集并集（8136 键），共有键取值冲突采用上游译文；验收：8 个冲突文件均无 `<<<<<<<` / `>>>>>>>` 标记，`git diff --name-only --diff-filter=U` 为空 <!-- comet-task:a974b6ce-c065-4f5d-9a91-313eec41d8d1 -->
- [x] 1.4 归一 i18n：在 `web/` 运行 `bun run i18n:sync`；验收：`_reports/_sync-report.json` 中 7 个语言 `missingCount=0`、`extrasCount=0`，再次运行该命令工作区无变化 <!-- comet-task:6b1128d1-9ba0-4642-907d-5c756a42c6d3 -->

## 2. 合并结果验证

- [x] 2.1 后端模块验证：根模块、`relaykit`、`tokenkit` 分别执行 `GOWORK=off go vet ./...`，三者退出码均为 0 <!-- comet-task:3fa55882-6c14-4e92-b667-48606eaa9a88 -->
- [x] 2.2 前端验证：在 `web/` 运行 `bunx tsc -b web/tsconfig.json` 通过，并运行与渠道抽屉/渠道配置相关的测试文件通过 <!-- comet-task:f42f34f5-8aab-48cc-a83a-c384527f2292 -->

## 3. 交付准备

- [x] 3.1 完成合并提交并回填 rerere 种子：生成 merge commit，把 `.git/rr-cache` 新增记录复制到 `.github/rerere-cache/` 并折入该提交；验收：`git log --oneline -1` 为 merge commit，两目录 `diff -r` 无差异 <!-- comet-task:ce0a93cc-e6d0-4792-b2ba-4daa4c79df7b -->

## Workflow follow-up

交付不在 build 任务范围，由归档阶段执行并记录：

- 推送 `origin/main`，确认 `git rev-parse origin/main` 等于本地 `HEAD`
- 手动触发 `sync-upstream`，确认「Check if upstream already merged」短路成功且不产生新 issue
- 关闭 issue #22，确认 `gh issue view 22 --repo thntime/new-api --json state` 返回 `CLOSED`

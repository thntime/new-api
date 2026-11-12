# Tasks

## 1. 合并与冲突解决

- [x] 1.1 记录基线：确认 `HEAD=ed9175de8`、`upstream/main=be5e3153d`、merge-base `2de51a19e`，并用 `git merge-tree --write-tree HEAD upstream/main` 复现与 issue #21 一致的 8 个冲突文件 <!-- comet-task:d6de44a2-7276-42f8-8ad0-2aa0e3c0756f -->
- [x] 1.2 执行 `git merge --no-edit upstream/main`（`rerere.enabled=true`），确认 `git status --short` 中 8 个文件处于冲突状态 <!-- comet-task:fbb2f5b8-c099-4bfa-a4e6-b601679c4308 -->
- [x] 1.3 解决 7 个 `web/src/i18n/locales/*.json`：按两侧键集并集写入（本地 284 + 上游 18，无取值冲突），在 `web/` 运行 `bun run i18n:sync` 归一；验收：7 个文件均无冲突标记，`_reports/_sync-report.json` 中 7 个语言 `missingCount=0`、`extrasCount=0`，再次运行 `i18n:sync` 工作区无变化 <!-- comet-task:ef68f9a1-068f-4adc-9169-5b882d9672bd -->
- [x] 1.4 解决 `channel-mutate-drawer.tsx` 3 处冲突：保留上游 `resetModelMappingSync(...)` 调用与依赖数组，同时保留本地 VolcEngine/Zhipu/Moonshot 接入方式回显与 `setDoubaoAccessMode('standard')`；验收：无冲突标记，`bunx tsc -b web/tsconfig.json` 通过 <!-- comet-task:21796a31-d148-4fc3-8276-2a19a8648d3d -->
- [x] 1.5 合并结果模块验证：根模块、`relaykit`、`tokenkit` 分别执行 `GOWORK=off go vet ./...`，三者退出码均为 0 <!-- comet-task:260d9b80-0bd6-4735-9d68-40baea6fb985 -->
- [x] 1.6 前端回归：在 `web/` 运行与渠道抽屉相关的测试文件（如 `bun --cwd web run test channel-mutate`），确认通过 <!-- comet-task:bd9cebe2-eb25-4046-8cf9-825222e28598 -->

## 2. 交付

- [x] 2.1 回填 rerere 种子：把 `.git/rr-cache` 新增记录复制到 `.github/rerere-cache/` 并折入合并提交；验收：`git log --oneline -1` 为 merge commit，两目录 `diff -r` 无差异 <!-- comet-task:629c9ee0-36d4-4317-9734-b896f1d48ef5 -->

## Workflow follow-up

交付不在 build 任务范围，由归档阶段执行并记录：

- 推送 `origin/main`，确认 `git rev-parse origin/main` 等于本地 `HEAD`
- 手动触发 `sync-upstream`，确认「Check if upstream already merged」短路成功且不产生新 issue
- 关闭 issue #21，确认 `gh issue view 21 --repo thntime/new-api --json state` 返回 `CLOSED`

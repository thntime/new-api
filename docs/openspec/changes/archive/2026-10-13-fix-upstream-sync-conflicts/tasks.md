# Tasks: fix-upstream-sync-conflicts

## 1. 复现（RED 证据）

- [x] 1.1 记录当前合并失败证据：`git fetch upstream` 后确认上游 tip（`633b4c5b1`），`git merge-tree --write-tree HEAD upstream/main` 输出 6 个冲突文件。

## 2. 执行合并并解决冲突

- [x] 2.1 执行 `git merge --no-edit upstream/main`，进入冲突状态
- [x] 2.2 解决 `.github/workflows/ci.yml`（融合：本地 push 触发 + repository 回退 + 上游 `ref: github.sha`）
- [x] 2.3 解决 `controller/security_enrollment_test.go`（本地优先：保留 `newSecurityTestDatabase` helper 与 sqlite import）
- [x] 2.4 解决 `web/package.json`（融合：保留本地 `@testing-library/dom` + 上游依赖升级；并删除合并产生的重复 `test` 脚本键）
- [x] 2.5 解决 `web/bun.lock`（取上游后由 `bun install` 重新生成）
- [x] 2.6 解决 `web/src/features/channels/components/drawers/channel-mutate-drawer.tsx`（保留本地 `modelProxyRulesFieldsUi` + 吸收上游 `tlsInsecureSkipVerifyFields`，两处渲染均保留）
- [x] 2.7 解决 `web/src/features/pricing/__tests__/model-cards.test.tsx`（保留本地「单页无分页」测例并吸收上游请求参数断言）

## 3. 验证

- [x] 3.1 `git diff --name-only --diff-filter=U` 为空、`git diff --check` 无残留冲突标记
- [x] 3.2 `GOWORK=off go vet ./...` 通过（root 模块）
- [x] 3.3 `relaykit`、`tokenkit` 各自 `GOWORK=off go vet ./...` 通过
- [x] 3.4 `make test`（root/relaykit/tokenkit 全部 Go 测试）通过
- [x] 3.5 前端 `bun run typecheck` 通过；`bun run test` 全量 223 文件 / 2472 用例通过

## 4. 提交

- [x] 4.1 `git add` 并提交合并结果（合并提交 `f210a78b0`）
- [x] 4.2 回填 rerere 种子：`cp -r .git/rr-cache/* .github/rerere-cache/` 并提交（`babcd8e0b`）

## 5. 交付（归档阶段执行）

push 到 `origin/main`，在 `/comet-archive` 的归档前最终确认中选择交付方式后执行。

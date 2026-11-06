# Design: fix-upstream-sync-conflicts

## 高层决策

1. **合并策略 = 融合优先**：逐个冲突文件精细融合，保留本地 fork 定制，同时吸收上游新改动（沿用历史 change `merge-upstream-rc36` 的策略）。
2. **矛盾点以「合并后的生产代码行为」为准**：测试文件（`model-cards.test.tsx`）的断言必须对齐合并后 `model-card-grid.tsx` 的实际行为，而不是机械取某一侧。
3. **本地行为优先**：fork 的分页定制（`MODEL_CARD_GRID_PAGE_SIZE = 1000`，无分页控件）等本地行为保持不变；上游对同一处的新行为仅在无冲突时吸收。

## 冲突解决矩阵

| 文件 | 策略 | 要点 |
|------|------|------|
| `.github/workflows/ci.yml` | 融合 | 保留本地 `push: branches: [main]` 与 `repository: ... || github.repository`；采纳上游对 `ref` 的改进（先确认上游语义与 fork push 触发兼容） |
| `controller/security_enrollment_test.go` | 本地优先 | 保留 `newSecurityTestDatabase` helper（对 db/logDB 均用 `_txlock=immediate`）；保留 `github.com/glebarez/sqlite` import |
| `web/package.json` | 融合 | 保留本地 `test` 脚本、`@testing-library/dom`、`user-event` 升级；采纳上游全部依赖版本升级与 `overrides` 新增 |
| `web/bun.lock` | 重新生成 | 冲突文件无法手工对齐，解决 `package.json` 后运行 `bun install` 重新生成 |
| `channel-mutate-drawer.tsx` | 本地优先 + 吸收 | 以本地 +740 版本为基底，移植上游 +28 行新增改动 |
| `model-cards.test.tsx` | 逐例对齐 | 保留本地「无分页」测例；吸收上游新增测例，并按合并后生产代码校正 query key / `recent_model` / 分页相关断言 |

## 风险与对策

- **bun.lock 手工合并风险**：不手工合并 lockfile，交由 `bun install` 重算，避免依赖树不一致。
- **测试与生产代码漂移**：`model-cards.test.tsx` 的断言若与合并后 `model-card-grid.tsx` 行为不符会导致前端测试失败；解决后用 `bun run test` / `vitest` 验证并按真实行为校正。
- **ci.yml ref 语义**：上游把 `ref` 改为 `github.sha`；fork 又有 `push` 触发。需确认合并后两个 job 在 push 与 pull_request 事件下均能正确 checkout，避免破坏 fork 自身 CI。
- **rerere 种子**：人工解决方案需回填 `.github/rerere-cache/`，否则下次同步会重复冲突。

## 验证方案

1. `GOWORK=off go vet ./...`（根模块）通过。
2. `cd relaykit && GOWORK=off go build ./...`（模块独立构建）通过。
3. `git ls-files -u` 为空（无未解决冲突标记）。
4. 前端：`cd web && bun install`（重算 lockfile）后按依赖可用情况运行 typecheck / 相关测试。
5. `git status` 干净、合并提交生成。

## 交付

按 issue #11 步骤：解决冲突 → 提交合并结果 → 回填 `.github/rerere-cache/` → push 到 `origin/main`。

# Proposal

## Why

每日 `Sync Upstream` workflow（run [40156915796](https://github.com/thntime/new-api/actions/runs/40156915796)，2026-10-25）在「Push merge」步骤失败。三方合并与 `GOWORK=off go vet ./...` 双模块关卡都已通过，真正原因是 `GITHUB_TOKEN` 属于 GitHub Actions App 安装令牌，不具备 `workflows` 权限，无法推送包含 `.github/workflows/ci.yml` 变更的合并提交（本次上游 16 个提交修改了 `ci.yml`、`electron-build.yml`、`release.yml`）。workflow 的兜底逻辑把该失败误报为「无法自动解决冲突」，issue #17 因此缺少真实原因与可执行线索。只要上游再次修改 workflow 文件，自动同步就会以同样方式失败。

## What Changes

- 在本地 `main` 完成 `upstream/main`（16 个提交、54 个文件）的三方合并，保持既有策略（不使用 `-X ours`），并通过根模块与 `relaykit` 的 `GOWORK=off go vet ./...`。
- 将 `sync-upstream.yml` 的 checkout/push 凭证改为专用 fine-grained PAT secret（`SYNC_UPSTREAM_TOKEN`，需 Contents: write 与 Workflows: write），使合并提交能够更新 workflow 文件。
- 增加凭证预检：secret 缺失或为空时立即失败并给出可操作提示，而不是在 push 阶段抛出难以理解的权限错误。
- 区分失败类型：push 权限失败与 merge 冲突分别写入 issue 摘要，避免把权限问题误导为冲突无法解决。
- 验证同步链路：本地 vet 通过、push 成功、issue #17 关闭，并确认后续 workflow 在「上游已包含」时短路成功。

## Capabilities

### New Capabilities

- `upstream-sync`: 每日把上游 `QuantumNous/new-api` main 三方合并进本仓库 main 的自动化行为，覆盖合并策略、构建关卡、rerere 种子回填、推送凭证要求、失败上报与短路条件。

### Modified Capabilities

（无）

## Impact

- `.github/workflows/sync-upstream.yml`
- 仓库 secret `SYNC_UPSTREAM_TOKEN`（需要用户在 GitHub 仓库设置中创建，本次实现不写入任何凭据）
- `main` 分支合并提交：16 个上游提交、54 个文件，含 3 个 workflow 文件
- 本次不自行编写业务逻辑；上游业务改动按原样合入，不改变数据库 schema，也不新增本地公共 API
- 关闭 issue #17

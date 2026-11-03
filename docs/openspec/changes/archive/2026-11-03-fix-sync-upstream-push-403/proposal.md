# Proposal

## Why

每日 `Sync Upstream` workflow 的最近一次运行（run [41387498945](https://github.com/thntime/new-api/actions/runs/41387498945)，2026-11-02T04:16:06Z）在「Push merge」步骤失败，自动创建 issue [#19](https://github.com/thntime/new-api/issues/19)。

- 合并与双模块 vet 均已通过：`git merge --no-edit upstream/main` 成功，`GOWORK=off go vet ./...` 与 `relaykit` 的 vet 均通过。
- 失败只发生在 push：`remote: Permission to thntime/new-api.git denied to thntime.` / `The requested URL returned error: 403`。
- 原因是仓库 secret `SYNC_UPSTREAM_TOKEN` 对应的 fine-grained PAT 已失效或未获得本仓库写权限（Contents: write / Workflows: write），属于 GitHub 仓库设置中的凭据问题，代码侧无法直接修复。
- CI 在 runner 内生成的合并提交（`5d0489398`）未推送成功，因此 `origin/main` 仍停在 `e191ca5e5`，上游 `main` 的 5 个新提交尚未进入本仓库。

当前上游新增提交：

```
45746eee4 feat(relay): add opt-in HTTP/2 health checks (#8059)
c046559bb feat(plugins): bill gemini omni by the tokens vertex reports (#8153)
651d5a213 fix(xai): preserve token details in stream usage conversion (#6420)
70e6af642 perf: read and edit request JSON and base64 with moego (#8148)
d0abbf6e0 feat(plugins): generate video with Gemini Omni on Vertex AI channels (#8142)
```

## What Changes

- 在本地 `main` 完成 `upstream/main`（5 个提交）的三方合并，保持既有策略（不使用 `-X ours`）。
- 通过根模块与 `relaykit` 的 `GOWORK=off go vet ./...` 关卡。
- 核对 `.git/rr-cache` 与 `.github/rerere-cache/` 无新增差异。
- 推送 `main`，使 `origin/main` 包含本次合并。
- 关闭 issue #19，并记录真实失败原因与后续需要用户完成的 PAT 修复步骤。
- 本次不修改 `sync-upstream.yml`，不改变合并策略、推送凭证方案或失败上报逻辑。

## Impact

- `main` 分支合并提交：5 个上游提交，涉及 `relay/`、`service/`、`plugins/tasks/vertex-ai/`、`relaykit/` 与 `.env.example`。
- 上游计费改动（Gemini Omni 按 token 计费）按上游实现原样合入，不新增本地公共 API，不修改数据库 schema。
- 仓库外动作：用户需在 GitHub 仓库 Settings → Secrets and variables → Actions 中重新生成或补授权 `SYNC_UPSTREAM_TOKEN`，否则下一次定时同步仍会以同样方式失败。
- 关闭 issue #19。

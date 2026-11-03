# Design

## Context

`Sync Upstream` 每日把上游 `QuantumNous/new-api` 的 `main` 三方合并进本仓库 `main`。2026-11-02 的运行在 merge 与双模块 vet 通过后，于 push 步骤被远端以 403 拒绝：

```
remote: Permission to thntime/new-api.git denied to thntime.
fatal: unable to access 'https://github.com/thntime/new-api/': The requested URL returned error: 403
```

checkout 与 push 都使用 fine-grained PAT secret `SYNC_UPSTREAM_TOKEN`。该 PAT 没有本仓库写权限、已过期，或未被授权访问 `thntime/new-api`，因此 runner 内的合并提交没有推送到远端。本地 `origin/main` 仍为 `e191ca5e5`，上游 `main` 比它多 5 个提交。

## Goals / Non-Goals

**Goals:**

- 立即恢复同步：在本地 `main` 完成 5 个上游提交的三方合并，通过双模块 vet，推送 `main`。
- 关闭 issue #19，并在关闭说明中记录真实失败原因与后续凭据修复步骤。

**Non-Goals:**

- 不修改 `.github/workflows/sync-upstream.yml`，不改变合并策略、rerere 机制、推送凭证方案或失败上报格式。
- 不引入 `-X ours`、强制推送或丢弃本地改动。
- 不自行改写上游业务逻辑；上游计费与协议改动按原样合入。
- 不处理工作区中与本次无关的 `docs/openspec` 归档产物。

## Decisions

### 1. 本地完成三方合并并直接推送 `main`

`git merge-tree --write-tree --name-only HEAD upstream/main` 预演无冲突；CI 同一合并已通过 vet。因此本地执行 `git merge --no-edit upstream/main`，生成纯上游合并提交，不夹带本地代码改动。合并提交保持可单独回滚。

### 2. 不修改 workflow

本次失败是仓库 secret 的权限问题，修改 workflow 不能恢复该 PAT 的写权限。按用户确认的最小范围，只完成同步与 issue 关闭；`sync-upstream.yml` 保持原样。

### 3. 关闭 issue 并写明外部后续动作

issue #19 由 CI 自动创建。同步推送成功后，用评论记录：失败步骤、真实 403 错误、本地补推的合并范围，以及需要用户在 GitHub 设置中重新生成/补授权 `SYNC_UPSTREAM_TOKEN`。随后关闭 issue。

## Risks / Trade-offs

- **PAT 未修复则次日再次失败** → 关闭说明中明确写出该外部动作；本次不掩盖问题。
- **本地推送使用当前登录账号的凭据** → 只推送已通过 vet 的合并提交，不改变远端历史。
- **上游带入计费改动** → 上游已评审并通过其 CI；本地以双模块 vet 验证，不额外改写。Gemini Omni 插件对时长做了 3–10 秒边界，视频 token 估算上限为 10 × 17376，不会溢出。
- **rerere 种子遗漏** → 推送前对比 `.git/rr-cache` 与 `.github/rerere-cache/`。

## Migration Plan

1. `git fetch upstream`，确认 `HEAD` 与 `origin/main` 一致、上游多 5 个提交、merge-tree 无冲突。
2. `git merge --no-edit upstream/main`。
3. 运行 `GOWORK=off go vet ./...` 与 `(cd relaykit && GOWORK=off go vet ./...)`。
4. 对比 rerere 缓存，确认无新增差异。
5. `git push origin main`。
6. 评论并关闭 issue #19。
7. 用户在 GitHub 设置中重新生成/补授权 `SYNC_UPSTREAM_TOKEN`，等待下一次定时运行确认。

## Open Questions

无。

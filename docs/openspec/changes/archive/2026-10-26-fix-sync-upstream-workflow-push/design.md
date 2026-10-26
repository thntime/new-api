---
comet_change: fix-sync-upstream-workflow-push
role: technical-design
canonical_spec: openspec
archived-with: 2026-10-26-fix-sync-upstream-workflow-push
status: final
---

# Design

## Context

`sync-upstream.yml` 目前用 `actions/checkout` 的默认 `GITHUB_TOKEN` 完成 checkout 与 `git push`。`GITHUB_TOKEN` 是 GitHub Actions App 的安装令牌，workflow 的 `permissions:` 块没有 `workflows` 权限（写入该字段会被判为非法配置），因此只要合并结果包含 `.github/workflows/` 下的改动，push 就会被远端以 `refusing to allow a GitHub App to create or update workflow ... without workflows permission` 拒绝。

本次上游 16 个提交恰好修改了 `ci.yml`、`electron-build.yml`、`release.yml`，run 40156915796 的合并与 vet 都成功，只在 push 阶段失败。失败兜底逻辑不区分失败类型，issue #17 因此写成「无法自动解决冲突」。当前仓库没有任何可用的 PAT 或 GitHub App secret，`gh` 已确认只有 `DOCKERHUB_*`、`TRANSLATE_*` 五类 secret。

## Goals / Non-Goals

**Goals:**

- 立即恢复同步：在本地完成本次三方合并、通过双模块 vet、推送 `main`，关闭 issue #17。
- 让同步任务在上游修改 workflow 文件时仍能自动推送。
- 凭证缺失时快速、明确地失败，不再把权限问题伪装成冲突问题。

**Non-Goals:**

- 不改变三方合并策略，不引入 `-X ours`、强制推送或自动丢弃本地改动。
- 不引入 GitHub App（需要额外 app id / private key secret 与安装步骤）。
- 不把上游 workflow 变更排除在合并之外，避免 CI 配置与 `package.json` 漂移。
- 不自行编写业务逻辑；上游业务改动按原样合入。
- 不处理工作区中与本次无关的 `docs/openspec` 归档产物。

## Decisions

### 1. 使用 fine-grained PAT secret 作为推送凭证

新增仓库 secret `SYNC_UPSTREAM_TOKEN`，通过 `actions/checkout` 的 `token` 输入注入，并让后续 `git push` 复用它。PAT 需要 `Contents: write` 与 `Workflows: write`，仅授权 `thntime/new-api`。

备选方案：

- **继续使用 `GITHUB_TOKEN`**：不可行。`permissions:` 不支持 `workflows`，无法授予该权限。
- **`actions/create-github-app-token`**：可行但需要新建 GitHub App、保存 app id 与 private key，并完成安装；对单一仓库的每日同步过重。
- **合并时保留本地 workflow 文件**：无需 secret，但上游 CI 配置（如 Bun 版本、`bun check -b`）会与合并后的 `package.json` 漂移，且需要长期手工同步。

### 2. 在 checkout 之前预检 secret

预检必须是第一步，放在 `actions/checkout` 之前。checkout 本身要使用该 PAT，若 secret 为空，checkout 会先失败并给出含糊的认证错误。预检通过 `env` 注入 secret 并检查非空；为空时输出 `::error::`、写入失败标记 `auth` 并退出 1，提示需要配置的 secret 名称与权限。检查过程不回显 secret 内容。

`permissions` 收敛为 `contents: read`（推送改由 PAT 完成）与 `issues: write`（失败兜底继续用 `GITHUB_TOKEN` 创建或更新 issue）。

### 3. 失败类型显式上报

预检、merge、verify、push 分别把 `auth` / `conflict` / `vet` / `push` 写入 `/tmp/failure-kind`。失败兜底脚本读取该标记，按实际类型生成 issue 标题与处理步骤，并同步更新已存在 issue 的标题。push 被权限拒绝时明确说明是凭证问题；merge 冲突时才展示冲突文件清单。

失败兜底步骤的条件改为 `if: failure() && steps.check.outputs.skipped != 'true'`：预检失败、fetch 失败也能上报；上游已包含时不会误建 issue。

### 4. 交付顺序：先合并提交，再 workflow 修复

本地先生成纯上游合并提交并验证 vet，然后在其上追加 workflow 修复提交，两者一起推送。这样合并提交保持纯同步、可单独回滚，workflow 修复也可独立审查。不使用 `--amend` 把 workflow 修改折进合并提交。

### 5. 本次不产生新的 rerere 种子

本地 `git merge-tree` 预演与上游合并均无冲突，因此 `.git/rr-cache` 不会新增记录；仍按既有步骤核对 `.github/rerere-cache/` 与 `.git/rr-cache` 的差异，确认无遗漏后再推送。未来人工解决冲突时继续按既有流程回填。

### 6. 用 PAT 推送会触发下游 workflow

`GITHUB_TOKEN` 的推送不会触发新的 workflow run；PAT 推送会触发。`build-image.yml` 等下游任务因此会在同步推送后自动运行，与 workflow 注释中「push 触发 build-image 重建镜像」的预期一致。代价是推送者显示为 PAT 所有者，而不是 `github-actions[bot]`。

## Risks / Trade-offs

- **PAT 泄露** → secret 只注入所需步骤、不回显；使用 fine-grained、单仓库、最小权限并设置有效期。
- **PAT 过期或权限不足** → checkout 或 push 会失败；预检至少覆盖 secret 缺失，过期由 GitHub 的认证错误直接暴露。
- **用户尚未创建 secret** → 预检立即失败并给出配置步骤，不会静默跳过同步。
- **PAT 推送触发下游 workflow** → 属于预期行为，但会增加一次镜像构建；如需抑制可改用 `repository_dispatch`，本次不做。
- **上游未来仍可能出现真实冲突** → 继续依赖三方合并 + rerere + 人工处理，不改变现有策略。
- **本次合并带入上游计费与协议改动** → 上游已评审并通过其 CI；本地以双模块 vet 关卡验证，不额外改写上游逻辑。

## Migration Plan

1. 用户在 GitHub 仓库 Settings → Secrets and variables → Actions 创建 fine-grained PAT，权限 `Contents: write`、`Workflows: write`，仅授权 `thntime/new-api`，保存为 `SYNC_UPSTREAM_TOKEN`。
2. 本地完成 `upstream/main` 合并，运行根模块与 `relaykit` 的 `GOWORK=off go vet ./...`。
3. 提交 workflow 修复，推送 `main`。
4. 关闭 issue #17；手动触发一次 `Sync Upstream`（或等待次日定时运行），确认在「上游已包含」时短路成功。
5. 回滚方式：`git revert` workflow 修复提交；同步任务退回 `GITHUB_TOKEN` 行为，仅在上游修改 workflow 文件时再次失败。

## Open Questions

无。

# Proposal

## Why

`sync-upstream` 目前每天把上游 `main` 的全部新提交合并进本 fork，即使上游只是普通开发提交。任何提交都可能与本地定制冲突并要求人工处理（issue #24 即一次每日合并冲突）。上游版本由 `v1.0.0-rc.N` tag 标识，只有版本变化才值得同步；普通提交可以等到下一个版本。

## What Changes

- `sync-upstream.yml` 的自动合并前置条件从「`upstream/main` 有新提交」改为「上游最新 `v*` 版本 tag 尚未包含在 `HEAD` 中」。
- 定时触发时，若上游最新版本 tag 已是 `HEAD` 祖先，则跳过合并与推送并以成功状态结束；即使 `upstream/main` 仍有新提交。
- 手动 `workflow_dispatch` 视为明确要求同步，跳过版本门禁，保留立即复核与追赶能力。
- 版本 tag 解析规则：fetch 上游 tags，在 `upstream/main` 可达的 `v*` tag 中按版本序取最新，并排除带 12 位时间戳后缀的 fork 发布 tag。
- 无法解析上游版本 tag 时失败并上报新的 `version` 失败类型，不无条件合并。
- 保持三方合并、rerere、双模块 vet、推送凭证、失败 issue 与 rerere 回填逻辑不变。
- 不改变 `release.yml`：本地 push 仍会生成 fork Release。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `upstream-sync`: 自动合并的触发条件改为上游版本变化；新增版本门禁短路、手动触发绕过与版本解析失败上报。

## Impact

- 代码：`.github/workflows/sync-upstream.yml`
- 规格：`docs/openspec/specs/upstream-sync/spec.md`（Purpose 文案）与本次 delta spec
- 不影响 Go、前端、API、数据库；不改变 `release.yml`、`build-image.yml` 的触发条件。
- issue #24 中待合并的上游提交不由本 change 解决；它们会在上游发布下一个版本 tag 时按既有冲突流程处理，或通过手动 `workflow_dispatch` 立即同步。

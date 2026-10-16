# Proposal: fix-upstream-sync-channel-settings-conflict

## 问题描述

`.github/workflows/sync-upstream.yml` 每日把上游 `QuantumNous/new-api` 的 `main` 三方合并进本 fork（`zhangjinglei137/new-api`）的 `main`。上一个 change 的修复（`relay/channel/api_request.go` 融合 + rerere 种子回填）已归档并 push，但随后触发的 run [38866286804](https://github.com/zhangjinglei137/new-api/actions/runs/38866286804)（2026-10-16T07:17:35Z，`workflow_dispatch`，headSha `cfb79af57`）仍在 step 7「Merge upstream into main (three-way + rerere)」失败（13s），issue #13 已被 workflow 更新为这轮的新冲突内容。

## 背景与现状

- HEAD = `origin/main` = `cfb79af57`（上一个 change 的归档提交）；`upstream/main` = `d03220473 feat(aws): add Mantle Messages API support`（在上次 push 之后由上游合入）。
- `HEAD..upstream/main` = 1 个提交；merge-base = `ca2cae676`（即上一个合并提交的上游父提交）。
- `git merge-tree --write-tree --name-only HEAD upstream/main` 预演：冲突文件 `relaykit/dto/channel_settings.go`、`web/src/features/channels/lib/channel-form.ts`；自动合并 `web/src/features/channels/components/__tests__/channel-configuration.test.tsx`、`web/src/features/channels/components/drawers/channel-mutate-drawer.tsx`、`web/src/features/channels/types.ts`、`web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json`。
- 上一轮修复依然有效：`relay/channel/api_request.go` 不再出现在冲突列表中，其 rerere 种子已入库。本轮失败是**新上游提交带来的新冲突**，不是旧问题复发。

## 根因分析

上游 `d03220473` 为 AWS 增加 Mantle Messages API 支持，改动落在本地 fork 同样自定义过的两个文件与相邻行：

| 文件 | 上游改动 | 本地改动 | 冲突成因 |
|------|----------|----------|----------|
| `relaykit/dto/channel_settings.go` | 新增 `AwsApiType` 类型与 `AwsApiTypeInvokeModel`/`AwsApiTypeMantle` 常量（插在 `ChannelOtherSettings` 之前），并在结构体中 `AwsKeyType` 之后新增 `AwsApiType` 字段 | 在 `ChannelOtherSettings` 之前新增 `ModelProxyRule` 类型与辅助函数，并在结构体中新增 `ModelProxyRules`、`CommandCodeCookie`、`SenseNovaUsername`、`SenseNovaPassword`、`EndpointProfile` 等字段 | 两侧插入点相同，行区间重叠 |
| `web/src/features/channels/lib/channel-form.ts` | 新增 `aws_api_type` 的 schema 字段、默认值、`transformChannelToFormDefaults` 读取，并把 `buildSettingsJSON` 的 AWS 分支由 `else if ('aws_key_type' in settingsObj)` 改为 `else` 且同时删除 `aws_key_type`/`aws_api_type` | 新增渠道类型 import、`ModelProxyRule` 表辅助函数与校验、schema 字段（`sort`、`commandcode_cookie`、`sensenova_*`、`volc_coding_plan_*` 等） | 两侧改动落在同一 schema/默认值/transform 区域，且触碰同一 AWS 分支 |

`.git/rr-cache` 中无这两个文件的 postimage 种子；workflow 有意不做 `-X ours` 整文件兜底（会静默丢弃上游新增符号），因此合并中止并交人工处理。

## 修复目标

- 融合两侧改动解决两个文件的冲突，不丢弃任何一侧语义。
- `GOWORK=off go vet ./...`（root、`relaykit`、`tokenkit`）以及前端 `bun run typecheck`、`bun run test` 全部通过。
- 提交合并结果并手动回填 rerere 种子；push 后 Sync Upstream 重跑走短路分支成功，据此关闭 issue #13。

## 非目标

- 不修改合并策略、不回退到 `-X ours`、不重构 `sync-upstream.yml`。
- 不重构本地 per-model proxy rules、CommandCode / SenseNova / EndpointProfile 等既有功能。
- 不新增功能、公共 API 或结构化数据格式变更（`aws_api_type` 是上游既有新增，本 change 只完成合并）。
- 不顺手升级依赖、格式化无关文件或处理与本次合并无关的仓库脏文件。

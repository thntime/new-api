# Proposal: fix-upstream-sync-api-request-conflict

## 问题描述

`.github/workflows/sync-upstream.yml` 每日把上游 `QuantumNous/new-api` 的 `main` 三方合并进本 fork（`zhangjinglei137/new-api`）的 `main`。本次调度运行 [38843406519](https://github.com/zhangjinglei137/new-api/actions/runs/38843406519)（2026-10-16T03:01:23Z，headSha `315d35301`）在 step 7「Merge upstream into main (three-way + rerere)」失败，后续 vet / rerere 回填 / push 步骤全部 skipped，failure 兜底创建 issue [#13](https://github.com/zhangjinglei137/new-api/issues/13) 等待人工处理。

## 背景与现状

- 本地 `main` == `origin/main` == `315d35301`，上游 `upstream/main` == `ca2cae676`（2026-10-16）。本地领先上游 256 个提交、落后 6 个提交，属真实三方合并，不能 fast-forward。
- 落后的 6 个提交：`ca2cae676`、`45fbb149f`、`6549a17ed`、`c9179d38b`、`908f94d65`、`19d435e7f`。
- dry-run 预演（`git merge-tree --write-tree HEAD upstream/main`，git 2.47.3）与 issue #13 一致：**唯一冲突文件 `relay/channel/api_request.go`**；`controller/relay.go`、`relaykit/types/error.go`、`service/error.go` 可自动合并。
- 工作区当前无代码改动，只有与本 change 无关的未跟踪 Comet 归档产物 `docs/openspec/changes/archive/*`（保留不处理）。

## 根因分析

两侧在同一位置附近插入代码，git 无法自动归并（三方合并 stage 2/3 均为内容冲突）：

| 侧 | 提交 | 改动 |
|----|------|------|
| 本地（fork） | `d4decae73 feat(日志): 新增上游原始报文采集与管理员查看面板` | 在 `doRequest` 的 `relayClient.Do(req)` 之前插入 `service.UpstreamPayloadCapture` 采集块，并在 `err != nil` / `resp == nil` 分支补 `capture.Finish()`、给 `resp.Body` 套 `WrapResponseBody` |
| 上游 | `19d435e7f fix(relay): mask only the custom upstream address in error messages (#7822)` | 在 `relayClient.Do(req)` 紧前一行插入 `common2.SetContextKey(c, appconstant.ContextKeyUpstreamRequestOrigin, req.URL.Scheme+"://"+req.URL.Host)`，并新增 `net/url`、`appconstant` import 与 websocket 分支的同类 `SetContextKey` |

两处改动语义正交（一个做报文采集埋点，一个记录上游来源地址用于报错脱敏），但插入点紧邻，形成文本冲突。

该冲突形态是本次上游提交新产生的，`.github/rerere-cache/` 中没有对应 `postimage` 种子，因此 CI 的 `git merge --no-edit` 只能失败退出——workflow 有意不在冲突时用 `-X ours` 兜底（见该文件头部注释），人工解决并回填种子是既定的人工介入路径。

## 修复目标

完成一次人工合并：融合解决 `relay/channel/api_request.go` 的唯一冲突（保留本地采集埋点 + 吸收上游来源地址记录），确保 `GOWORK=off go vet ./...`（root 与 `relaykit`）通过；提交合并结果并回填 `.github/rerere-cache/` 种子，推送 `origin/main` 后使同步 workflow 重跑成功，并关闭 issue #13。

## 非目标

- 不改变同步 workflow 的合并策略（不引入 `-X ours` 或其它全局兜底）。
- 不重构 fork 的上游报文采集功能，不放宽上游报错脱敏行为。
- 不新增功能、不改变对外 API / 数据库 schema。

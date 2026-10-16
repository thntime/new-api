# Design: fix-upstream-sync-api-request-conflict

## 高层决策

1. **融合优先**：两侧改动语义正交，合并后同时保留——本地 `UpstreamPayloadCapture` 采集埋点与上游 `ContextKeyUpstreamRequestOrigin` 来源地址记录。
2. **不丢弃任何一侧**：既不退回本地版本（会丢掉上游报错脱敏，导致 #7822 修复丢失），也不取上游版本（会丢掉日志面板依赖的上游报文采集）。
3. **rerere 种子按需回填**：`.github/rerere-cache/` 是该种子的唯一事实源（workflow 从仓库恢复，不用 `actions/cache`）；人工解决方案必须回填，否则同型冲突会反复失败。

## 冲突解决矩阵

| 文件 | 策略 | 要点 |
|------|------|------|
| `relay/channel/api_request.go` | 融合 | 在 `doRequest` 中：保留采集块（`capture` 创建、`req.Body` 包装）与 `err != nil` / `resp == nil` 分支的 `capture.Finish()`、`resp.Body` 的 `WrapResponseBody`；在 `resp, err := relayClient.Do(req)` 紧前保留上游 `common2.SetContextKey(c, appconstant.ContextKeyUpstreamRequestOrigin, req.URL.Scheme+"://"+req.URL.Host)`；保留上游新增的 `net/url`、`appconstant` import 与 websocket 分支的同类 `SetContextKey` |

## 风险与对策

- **符号可用性**：`appconstant.ContextKeyUpstreamRequestOrigin`、`net/url` 来自上游提交，需确认合并后本地树中存在；由 `GOWORK=off go vet ./...` 把关。
- **采集与脱敏互相影响**：采集块只包装 `req.Body` / `resp.Body`，`SetContextKey` 只写 gin context，二者不共享变量；按「按行取并集」解决即可，不引入额外逻辑。
- **种子回填遗漏或过期**：回填后校验 `.git/rr-cache/<conflict-hash>/postimage` 已复制到 `.github/rerere-cache/<hash>/`；如页面上同名 hash 残留旧 `postimage` 导致下次合并取到过期内容，删除该目录中的过期 `postimage` 后重试。
- **推送后立即重跑**：`origin/main` 已包含 `upstream/main` 时，workflow 的 `git merge-base --is-ancestor upstream/main HEAD` 短路分支会直接成功；这是预期路径，不代表合并冲突被自动修复。

## 验证方案

1. `git diff --name-only --diff-filter=U` 为空，`git diff --check` 无残留冲突标记。
2. `GOWORK=off go vet ./...`（root 模块）通过。
3. `(cd relaykit && GOWORK=off go vet ./...)` 通过（与 workflow 的 vet 关卡同款命令）。
4. 行为核对：`grep -n "UpstreamPayloadCapture\|ContextKeyUpstreamRequestOrigin" relay/channel/api_request.go` 显示两侧代码均在 `doRequest` 中保留。
5. 提交后 `git merge-base --is-ancestor upstream/main HEAD` 为真（覆盖 workflow 短路路径），并核对 `.github/rerere-cache/<hash>/postimage` 已入库。
6. workflow 重跑成功（`gh run view` 结论为 success）。

## 交付

解决冲突 → 提交合并结果 → 回填 `.github/rerere-cache/` → push `origin/main` → 确认 Sync Upstream 重跑成功 → 关闭 issue #13。推送与 issue 关闭在 `/comet-archive` 的归档前最终确认中选择交付方式后执行。

# Tasks: fix-upstream-sync-api-request-conflict

## 1. 复现（RED 证据）

- [x] 1.1 记录失败证据：run `38843406519` 在 step 7「Merge upstream into main (three-way + rerere)」失败，issue #13 列出冲突文件 `relay/channel/api_request.go`；本地 `git merge-tree --write-tree HEAD upstream/main` 同样只报该文件内容冲突。无自动化单测覆盖：交付物是合并结果本身，回归证据改用 `merge-tree` 预演 + workflow 同款 vet 关卡复现。

## 2. 执行合并并解决冲突

- [x] 2.1 执行 `git merge --no-edit upstream/main`，进入冲突状态并确认冲突文件仅 `relay/channel/api_request.go`（`git diff --name-only --diff-filter=U` 仅此一个；`controller/relay.go`、`relaykit/types/error.go`、`service/error.go` 自动合并）
- [x] 2.2 融合解决 `relay/channel/api_request.go`（保留采集埋点 + 吸收上游 `ContextKeyUpstreamRequestOrigin`）：采集块 → `SetContextKey` → `relayClient.Do(req)`，error/nil 分支保留 `capture.Finish()`、返回前保留 `WrapResponseBody`
- [x] 2.3 确认无残留冲突标记（`git diff --check`、`git diff --name-only --diff-filter=U` 为空）并 `git add`

## 3. 验证

- [x] 3.1 `GOWORK=off go vet ./...`（root）通过（exit 0）
- [x] 3.2 `(cd relaykit && GOWORK=off go vet ./...)` 通过（exit 0）；`tokenkit` 同样 exit 0
- [x] 3.3 行为核对：`doRequest` 中同时存在采集块与 `ContextKeyUpstreamRequestOrigin` 写入；`go test ./relay/channel/...`、`go test ./service/... -run 'Capture|Upstream'` 通过

## 4. 提交与回填种子

- [x] 4.1 提交合并结果：合并提交 `a4119b453ae67cfec5d3370d35842a7e98b3e937`（parents `315d35301` + `ca2cae676`）
- [x] 4.2 回填 `.github/rerere-cache/`（`cp -r .git/rr-cache/* .github/rerere-cache/`，仅新增 1 个目录、未改动既有 218 个文件）并用 `git commit --amend` 折入合并提交，校验冲突 hash 目录 `181ee148b0c843fa1eb8326c33a8346a4986bff0` 含 `preimage`/`postimage`，且 `postimage` 与提交内 `relay/channel/api_request.go` 完全一致

## 5. 交付（归档阶段执行）

push 到 `origin/main` 并在 `/comet-archive` 的归档前最终确认中选择交付方式后执行；push 后触发 Sync Upstream（`workflow_dispatch` 或用当日 `schedule`）确认走短路分支成功，再手工关闭 issue #13（workflow 无自动关闭步骤）。

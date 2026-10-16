# Tasks: fix-upstream-sync-channel-settings-conflict

## 1. 复现（RED 证据）

- [x] 1.1 记录失败证据：run `38866286804`（headSha `cfb79af57`）在 step 7「Merge upstream into main (three-way + rerere)」失败（13s），issue #13 被更新为上游提交 `d03220473 feat(aws): add Mantle Messages API support` 与冲突文件 `relaykit/dto/channel_settings.go`、`web/src/features/channels/lib/channel-form.ts`；本地 `git fetch upstream main` 后 `git merge-tree --write-tree --name-only HEAD upstream/main` 复现同样两个冲突文件。无自动化单测覆盖：交付物是合并结果本身，回归证据改用 `merge-tree` 预演 + workflow 同款 vet 关卡复现。

## 2. 执行合并并解决冲突

- [x] 2.1 执行 `git merge --no-edit upstream/main`，确认冲突文件仅 `relaykit/dto/channel_settings.go`、`web/src/features/channels/lib/channel-form.ts`（其余 web 文件自动合并）。实测：`git diff --name-only --diff-filter=U` 仅这两个文件；自动合并 `web/src/features/channels/components/__tests__/channel-configuration.test.tsx`、`drawers/channel-mutate-drawer.tsx`、`types.ts`、`web/src/i18n/locales/{en,fr,ja,ru,vi,zh-TW,zh}.json`；两文件均记录 preimage。
- [x] 2.2 融合解决 `relaykit/dto/channel_settings.go`：保留本地 `ModelProxyRule` 类型与辅助函数块，采纳上游 `AwsApiType` 类型与常量；结构体内同时保留上游 `AwsApiType` 字段与本地新增字段。实测：删除三处冲突标记后两段并存，`grep -c` 得 `ModelProxyRule` 11 次、`AwsApiTypeMantle` 1 次。
- [x] 2.3 融合解决 `web/src/features/channels/lib/channel-form.ts`：保留本地 import、`ModelProxyRule` 辅助与 schema 字段，采纳上游 `aws_api_type` 的 schema/默认值/transform 与 `buildSettingsJSON` 的 opt-in 语义。实测：返回对象同时保留 `endpoint_profile` 与 `aws_api_type`；`buildSettingsJSON` 中 AWS 分支已为上游语义（`type === 33` 写入 `aws_key_type` + `aws_api_type`，否则两者都 delete），本地 `endpoint_profile` 分支未被牵连。
- [x] 2.4 确认无残留冲突标记（`git diff --check`、`git diff --name-only --diff-filter=U` 为空）并 `git add`。实测：解决后两个命令均无输出，`git add` 后待解决列表为空。

## 3. 验证

- [x] 3.1 `GOWORK=off go vet ./...`（root）通过；`(cd relaykit && GOWORK=off go vet ./...)`、`(cd tokenkit && GOWORK=off go vet ./...)` 通过。实测：三次 `comet check run` 退出码均为 0（logRef `f41a3aa2-e4b9-4ab9-ad85-99fd8ef7fad2` / `ad5d8771-ebbd-4510-a363-284ac73b524e` / `abe9a995-1f20-44a1-aa73-2a871859ae3b`）。
- [x] 3.2 `make test`（root + relaykit + tokenkit 的 Go 测试）通过。实测：退出码 0，logRef `0fbb9748-93ba-43e6-aeeb-8bfd5a9fb4ca`。
- [x] 3.3 前端 `cd web && bun run typecheck` 与 `bun run test` 通过。实测：typecheck 退出码 0（logRef `41fb890b-653d-4446-92e4-f221192caaf0`）；vitest 退出码 0（logRef `c0d39e0d-baad-42e4-b839-17f55cbc8765`）。`bun run lint` 退出码 1，但日志中 172 个 error / 66 个 warning 全部位于本次未改动的既有文件（如 `src/lib/utils.ts`、`src/components/**`），本次改动的三个前端文件单独运行 oxlint 无任何输出（无 error/warning），属仓库既有 lint 债务，非本次引入。
- [x] 3.4 行为核对：`relaykit/dto/channel_settings.go` 中同时存在 `ModelProxyRule` 与 `AwsApiType`；`channel-form.ts` 中同时存在 `parseModelProxyRules` 与 `aws_api_type`。实测：`grep -c` 分别为 11/1 与 2/6；且 `git merge-base --is-ancestor upstream/main HEAD` 为真、workflow 文件相对 `cfb79af57` 无改动（根因消除检查）。

## 4. 提交与回填种子

- [x] 4.1 提交合并结果（`git commit --no-edit` 生成合并提交，记录实际 sha 与 parents）。实测：`git commit --no-edit` 输出「已记录 ... 的解决方案」，生成合并提交（amend 前 `8e5ea5704`）。
- [x] 4.2 回填 `.github/rerere-cache/`（`cp -r .git/rr-cache/* .github/rerere-cache/`）并用 `git commit --amend --no-edit` 折入合并提交，校验新增 hash 目录含 `preimage`/`postimage` 且 `postimage` 与提交内目标文件一致。实测：新增 `174e2617dd3ea7b78bdd69ebd77a4ceb096e4dce`、`c44ae1164aa95baa2558e32a525d75816477c40d` 两个目录（`git status --short .github/rerere-cache/` 仅此两项），两者 `postimage` 分别与 `web/src/features/channels/lib/channel-form.ts`、`relaykit/dto/channel_settings.go` 逐字节一致（`diff -q` 无差异）；最终合并提交 `5d85c71cf84b9163d1e0ac3396ecf058cb671095`，parents `cfb79af57` + `d03220473`，共 20 files changed，4877 insertions(+), 3 deletions(-)。

## 5. 交付（归档阶段执行）

push 到 `origin/main` 并在 `/comet-archive` 的归档前最终确认中选择交付方式后执行；push 后触发 Sync Upstream（`workflow_dispatch` 或用当日 `schedule`）确认走短路分支成功，再手工关闭 issue #13（workflow 无自动关闭步骤）。

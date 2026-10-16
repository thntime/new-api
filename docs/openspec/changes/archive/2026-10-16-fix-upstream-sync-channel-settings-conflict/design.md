# Design: fix-upstream-sync-channel-settings-conflict

## 高层决策

1. **融合优先**：两侧改动语义正交，合并后同时保留——上游的 AWS Mantle 接口选择（`AwsApiType` / `aws_api_type`）与本地的 per-model proxy rules、CommandCode / SenseNova / EndpointProfile 等字段。
2. **不丢弃任何一侧**：既不退回本地版本（会丢掉上游新增的 `AwsApiType` 符号，导致上游引用方编译失败），也不取上游版本（会丢掉本地功能与依赖它们的引用方）。
3. **rerere 种子按需回填**：`.github/rerere-cache/` 是种子的唯一事实源（workflow 从仓库恢复，不用 `actions/cache`）；人工解决方案必须回填，否则同型冲突会反复失败。

## 冲突解决矩阵

| 文件 | 策略 | 要点 |
|------|------|------|
| `relaykit/dto/channel_settings.go` | 融合 | `ChannelOtherSettings` 之前同时保留本地 `ModelProxyRule` 类型与 `compileModelProxyRegex` / `modelProxyRuleExactMatch` / `modelProxyRuleRegexMatch` 辅助函数块，与上游 `AwsApiType` 类型及 `AwsApiTypeInvokeModel`/`AwsApiTypeMantle` 常量；结构体内同时保留上游 `AwsApiType` 字段（紧邻 `AwsKeyType`）与本地新增字段（`ModelProxyRules`、`CommandCodeCookie`、`SenseNovaUsername`、`SenseNovaPassword`、`EndpointProfile` 等），字段顺序按各自原有语义排列，不重排既有字段 |
| `web/src/features/channels/lib/channel-form.ts` | 融合 | 保留本地 import（`CHANNEL_TYPE_CLINE`/`COMMANDCODE`/`OPENCODE_GO`/`SENSENOVA`）、`ModelProxyRule` 表辅助函数与校验、本地 schema 字段（`sort`、`commandcode_cookie`、`sensenova_*`、`volc_coding_plan_*`）；采纳上游 `aws_api_type` 的 schema 字段、`CHANNEL_FORM_DEFAULT_VALUES` 默认值 `invoke_model`、`transformChannelToFormDefaults` 中的读取与回填；`buildSettingsJSON` 采纳上游语义（`type === 33` 时写入 `aws_key_type` 与 `aws_api_type`，否则两者都删除） |

## 风险与对策

- **上游符号可用性**：`AwsApiType`、`AwsApiTypeMantle` 及前端 `aws_api_type` 的引用方分布在其它文件（已自动合并），合并后必须仍能编译；由 root/`relaykit` 的 `GOWORK=off go vet ./...` 与前端 `tsgo -b` 把关。
- **同一分支逻辑被改写**：上游把 `buildSettingsJSON` 的 `else if ('aws_key_type' in settingsObj)` 改为 `else` 并新增删除 `aws_api_type`。按上游语义采纳，并确认本地新增字段仍由各自的写入分支处理，不因该改动被牵连删除。
- **本地专属改动被误删**：解决冲突后逐项核对本地符号仍在（`ModelProxyRule`、`parseModelProxyRules`、`CHANNEL_TYPE_SENSENOVA` 等），避免以「取一侧」图省事。
- **种子回填遗漏或过期**：回填后校验 `.git/rr-cache/<hash>/postimage` 已复制到 `.github/rerere-cache/<hash>/`，且与提交内的目标文件内容一致；若同名 hash 残留旧 `postimage` 导致下次合并取到过期内容，删除该目录中的过期 postimage 后重试。
- **重跑即短路**：`origin/main` 已包含 `upstream/main` 时，workflow 的 `git merge-base --is-ancestor upstream/main HEAD` 短路分支会直接成功，这是预期路径，不代表合并冲突被自动修复。
- **上游持续前进**：上游可能再次合入新提交而引入新一轮冲突；本 change 只负责当前 `d03220473` 这一轮，出现新冲突时按同样流程另开 change。

## 验证方案

1. `git diff --name-only --diff-filter=U` 为空，`git diff --check` 除 rerere `preimage` 外无残留冲突标记。
2. `GOWORK=off go vet ./...`（root 模块）通过。
3. `(cd relaykit && GOWORK=off go vet ./...)`、`(cd tokenkit && GOWORK=off go vet ./...)` 通过（与 workflow 的 vet 关卡同款命令）。
4. `make test`（root + relaykit + tokenkit 的 Go 测试）通过。
5. 前端 `sh -c 'cd web && bun run typecheck'` 与 `sh -c 'cd web && bun run test'` 通过（渠道相关用例覆盖 `aws_api_type` 与本地表单字段）。
6. 行为核对：`relaykit/dto/channel_settings.go` 同时存在 `ModelProxyRule` 与 `AwsApiType`；`channel-form.ts` 同时存在 `parseModelProxyRules` 与 `aws_api_type`。
7. 提交后 `git merge-base --is-ancestor upstream/main HEAD` 为真，且 `.github/rerere-cache/<hash>/postimage` 已入库。
8. workflow 重跑成功（`gh run view` 结论为 success）。

## 交付

解决冲突 → 提交合并结果 → 回填 `.github/rerere-cache/` → push `origin/main` → 确认 Sync Upstream 重跑成功 → 关闭 issue #13。推送与 issue 关闭在 `/comet-archive` 的归档前最终确认中选择交付方式后执行。

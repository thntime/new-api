# Proposal

## Why

`.github/workflows/sync-upstream.yml` 每日把上游 `QuantumNous/new-api` 的 `main` 三方合并进本 fork（`thntime/new-api`）的 `main`。run [42165615583](https://github.com/thntime/new-api/actions/runs/42165615583) 在 step 7「Merge upstream into main (three-way + rerere)」失败，failure 兜底创建 issue [#20](https://github.com/thntime/new-api/issues/20)。本地 `upstream/main` 现为 `2de51a19e`，`git merge-tree --write-tree HEAD upstream/main` 复现出 10 个冲突文件；冲突既包含纯文本重叠（import、i18n 相邻行），也包含语义重叠（上游新增的价格变更复核流程与本地可视化编辑器、批量清空能力），不能靠整侧覆盖解决。

同一请求还要求给已注册的 CodeBuddy 渠道类型（93）增加第三种「接入方式」`CodeArts`。现有接入方式只有 `国内(cn)` / `国际(intl)`，设计上仅作为区域标记写入 `settings.codebuddy_region`，从不推断、预填或改写 API 地址。

## What Changes

- 完成 `upstream/main`（`2de51a19e`）到 `main` 的三方合并，解决 10 个冲突文件并生成合并提交；上游 10 个提交（缓存、鉴权、定价复核、Gemini/Vertex 等）按原样进入本仓库。
- `model/channel.go`：合并 import 冲突，同时保留本地 `regexp` 与上游 `slices`；接受上游 `BatchInsertChannels` 改为 `slices.Chunk` 并回填生成 ID 的改动。
- `web/src/features/system-settings/models/model-ratio-form.tsx`、`model-ratio-visual-editor.tsx`：融合上游「保存前复核价格变更、删除保留为草稿并支持撤销、编辑器脏状态」与本地「可视化编辑器多选、批量清空、`ConfirmDialog`」两套语义，两侧能力都不丢。
- 7 个 `web/src/i18n/locales/*.json`：按并集解决相邻行插入冲突，同时保留上游新增 key 与本地新增 key。
- CodeBuddy 渠道「接入方式」新增 `CodeArts`：后端 `relaykit/dto/channel_settings.go` 的取值校验接受新值，前端表单枚举、选择器选项与 7 个语言文件同步新增；该值仍仅作为标记写入 `settings.codebuddy_region`，**MUST NOT** 推断、预填或改写 `base_url`，也 **MUST NOT** 改变发往上游的请求内容。
- 交付：把 `.git/rr-cache` 中本轮新增记录回填到 `.github/rerere-cache/`，push `origin/main`，关闭 issue #20。
- 不引入新的计费算法，不改动数据库 schema，不改动其他渠道类型或其他接入方式。

## Capabilities

### New Capabilities

- `codebuddy-provider`: CodeBuddy 渠道类型的接入行为，包括渠道类型注册、`国内` / `国际` / `CodeArts` 三种接入方式标记、三种客户端协议转发、上游模型列表与图标展示。主 spec 此前未随归档同步生成，本次归档补齐完整能力定义，并把接入方式扩展到三个取值。

### Modified Capabilities

- `model-pricing-management`: 合并上游价格变更复核后，补充「保存前复核每个模型的旧价与新价」「删除保留为草稿并支持撤销」两条要求，并调整「批量清空」要求，使其清空结果同样进入保存前复核，且窄屏下次级操作可收进菜单而不违反入口位置约定。

## Impact

- 后端：`model/channel.go`（合并 import 与 `BatchInsertChannels`）、`relaykit/dto/channel_settings.go`（`ValidateCodeBuddyRegion` 取值）及其测试。
- 前端：`web/src/features/system-settings/models/model-ratio-form.tsx`、`model-ratio-visual-editor.tsx`、`web/src/features/channels/lib/channel-form.ts`、`web/src/features/channels/components/drawers/channel-mutate-drawer.tsx`、`web/src/features/channels/lib/__tests__/codebuddy-channel.test.ts`、`web/src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json`。
- 仓库运维：`.github/rerere-cache/`（rerere 种子回填）、`main` 分支推送、issue #20 关闭。
- 数据库：不改动 schema；`settings.codebuddy_region` 继续复用既有 JSON 字段。
- 不涉及：`CodeArts` 专属地址推导、计费表达式、其他渠道类型、其他接入方式。

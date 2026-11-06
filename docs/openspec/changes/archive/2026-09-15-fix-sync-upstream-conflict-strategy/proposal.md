# Proposal: 修复 sync-upstream 冲突策略导致自动同步连续失败

## Why

本 fork（thntime/new-api）的每日上游同步（`.github/workflows/sync-upstream.yml`）已连续 6 次失败并触发 issue #7。同步流程采用「本地优先」策略：默认三方 merge 冲突时回退到 `-X ours` 全局兜底。但该兜底按**整文件**取本地版本，无法区分「本地自定义改动」与「上游新增必填符号」：当上游在某冲突文件新增常量/字段（如 `constant/channel.go` 新增 `ChannelTypeVLLM`/`ChannelTypeSGLang`），而引用这些符号的其它文件（`common/api_type.go`、`common/advanced_custom_presets.go`）无冲突正常合并时，合并结果必然编译失败（`undefined: constant.ChannelTypeVLLM`）。`-X ours` 兜底因此是结构性缺陷，必须消除。

## What Changes

- 手动合并当前 `upstream/main` 与本地 `main`，逐个解决 10 个冲突文件，保留本地自定义扩展（渠道排序、亲和重试、序号等）的同时补入上游新增符号（VLLM/SGLang 渠道类型、Responses WebSocket 支持等），保证 `go vet` 通过。
- 改造 `sync-upstream.yml`：移除全局 `-X ours` 兜底，改为「默认三方 merge 冲突 → 中止并创建 issue 等待人工处理」，不再自动产出可能编译失败的合并结果。
- 保留并修正 rerere 缓存机制：已人工解决的冲突由 rerere 自动记录，未来相同冲突自动重放，避免重复人工介入。
- 清理 sync 失败 issue 的重复创建逻辑（可选，随 workflow 修改一并处理）。

## Capabilities

### New Capabilities

- 无（纯工具/CI 流程修复，不改变产品行为）

### Modified Capabilities

- 无（`skip_specs: true`，不涉及任何产品 spec 行为变更）

## Impact

- `.github/workflows/sync-upstream.yml`：合并策略与失败处理逻辑
- `.github/rerere-cache/`：人工解决后重新记录的冲突方案
- 本地 `main` 分支：一次性手动合并提交（上游 7 个提交）
- 冲突文件（10 个）：`constant/channel.go`、`relaykit/dto/channel_settings.go`、`common/endpoint_type.go`、`relay/common/relay_info.go`、前端 channels 相关 6 文件、`.gitignore`

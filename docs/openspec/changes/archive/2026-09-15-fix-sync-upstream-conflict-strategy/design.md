# Design: 手动合并 + 移除 -X ours 全局兜底

## 方案概述

一次手动合并解决当前冲突（issue #7），同时改造 workflow 消除 `-X ours` 结构性缺陷，使其未来不再自动产出损坏的合并结果。

## 修复方案

### 1. 手动合并当前上游（一次性操作）

1. 在本地 `main` 执行 `git merge upstream/main`，得到 10 个冲突文件。
2. 逐个解决冲突：
   - `constant/channel.go`：保留本地全部自定义渠道类型，**补入上游新增的 `ChannelTypeVLLM`/`ChannelTypeSGLang` 及关联定义**。
   - `common/endpoint_type.go`、`relay/common/relay_info.go`：保留本地改动，补入上游新增分支/字段。
   - `relaykit/dto/channel_settings.go`：保留本地扩展字段，补入上游新增设置项（VLLM/SGLang 相关）。
   - 前端 channels 6 文件：保留本地排序/序号/亲和相关 UI 与逻辑，合并上游新增渠道配置项（VLLM/SGLang 选项、Responses WebSocket 开关等）。
   - `.gitignore`：合并双方忽略规则。
3. 合并完成后运行 `GOWORK=off go vet ./...` 与 `cd relaykit && GOWORK=off go vet ./...`，确认零错误。
4. 用 `git add` + `git commit` 生成合并提交（保留 MERGE_MSG 说明上游提交范围）。
5. rerere 自动记录本次解决的冲突方案，手动检查 `.git/rr-cache` 后将其种子同步到 `.github/rerere-cache/`（供 workflow 恢复）。
6. 由用户确认后 `git push origin main`。

### 2. 改造 sync-upstream workflow（防复发）

核心原则：**永不自动产生验证未通过的合并结果**。

- 删除「全局 `-X ours` 兜底」逻辑（第二层 merge）。
- 默认三方 merge 若冲突：立即中止 merge，列出冲突文件，创建 `sync-upstream` issue 等待人工处理，job 结束（不 push、不提交任何东西）。
- 保留现有 `go vet` 验证关卡：merge 成功后仍必须通过 vet 才 push。
- 保留 rerere 种子恢复/保存机制：人工解决并 push 后，rerere 方案随合并提交进入 `.github/rerere-cache/`，未来相同冲突自动重放。
- 失败 issue 去重：存在未关闭的同名 issue 时更新而非重复创建。

### 备选方案（未采用）

- 分支化同步（临时分支 merge + PR）：更安全但改变交付习惯，用户已选择「半自动：冲突/失败即停止等人工」。
- 为每个冲突文件定制 merge 驱动（union/ours）：维护成本高且无法解决「引用分散」类编译问题。

## 关键决策

- **不引入新的 public API / schema / capability**，纯 CI 与一次性合并，符合 hotfix 范围。
- 手动合并结果以 `go vet` 双模块零错误为验收门槛，与 workflow 验证关卡一致。

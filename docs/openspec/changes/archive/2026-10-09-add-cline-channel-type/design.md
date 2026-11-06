## Context

参见 proposal.md。Cline 是同时提供按量与 Coding Plan 的 AI 编码服务。现有仓库已具备可复用的范式：

- 渠道子模式（Coding Plan 等）通过 `OtherSettings.EndpointProfile` 承载，并由 `ResolveSpecialPlan` 解析为不同上游端点；前端已有对应的 "Access Mode" 选择器（Zhipu/Volc/Moonshot 先例）。
- 上游取模型逻辑集中在 `controller/channel_upstream_update.go` 的手写分支，无统一注册表；计划用量则有 Volc/Moonshot/Zhipu/OpenCodeGo 等「service + controller + route + 前端弹窗」先例。
- 渠道级 `header_override` / `param_override` 在转发前应用，`param_override` 简写格式对顶层 key 做整体替换。

## Goals / Non-Goals

**Goals:**

- 以最小改动面新增 Cline 渠道类型，并复用既有「子模式」与「计划用量」范式。
- 保持与现有渠道类型一致的注册、转发与前端展示契约。

**Non-Goals:**

- 不引入新的计费算法；ClinePass 仍按 token 扣减用户额度。
- 不改动用户级订阅/套餐系统。
- 不改动数据库 schema。

## Decisions

### D1：计费方式复用 `EndpointProfile`

采用 `OtherSettings.EndpointProfile` 承载 `Cline`（按量）与 `ClinePass`（计划），与现有 `coding` / `coding-intl` 子模式并列，并复用前端 Access Mode 选择器。实现时在 `ChannelSpecialPlanProfiles[ChannelTypeCline]` 登记 `{"clinepass": "cline-pass"}`，并在 `ChannelSpecialBases["cline-pass"]` 登记两个 base（均为 Cline 默认地址），从而直接复用既有 `ValidateSettings` 与 `ResolveSpecialPlan`，无需为 Cline 特判校验。

- 理由：与仓库既有渠道子模式一致，改动面最小，且校验路径完全复用。因两种方式上游 base 相同，Cline adaptor 不按 profile 切换基础端点（三路由 URL 固定）。
- 备选：新增独立渠道字段。否决原因：会引入与既有 Access Mode 平行的第二套模式机制，增加不一致与维护成本。

### D2：新增独立 adaptor 并复用协议转换器

新增 `relay/channel/cline` adaptor，实现渠道适配器接口；chat completions 与 responses 复用 OpenAI 协议转换，messages 复用 Claude 协议转换，adaptor 负责按 `EndpointProfile` 拼接上游 URL 与应用默认请求体覆盖。

- 理由：三种路由的上游协议已由仓库现有 OpenAI/Claude 转换器覆盖，避免重复实现。
- 备选：复用 Advanced Custom 命名渠道。否决原因：Cline 需要按模式分流取模型/用量与非通用鉴权，命名渠道预设不足以表达。

### D3：取模型按计费方式分流

在 `controller/channel_upstream_update.go` 的取模型分支中新增 Cline 处理：按量走 `{base}/v1/models`（标准解析），计划走 `{base}/v1/ai/cline/recommended-models` 并仅解析 `clinePass[].id`。

- 理由：与现有按渠道类型分支的既有结构一致。
- 备选：为 Cline 走 Advanced Custom 配置化取模型。否决原因：计划方式的 `clinePass` 专属解析不是通用模型列表形状。

### D4：计划用量独立实现

按 Volc/Moonshot 先例，新增 Cline 计划用量 service + controller + 路由 + 前端用量弹窗，仅在 `ClinePass` 时可用。

### D5：默认请求体覆盖作为前端预填

默认覆盖（`providerOptions` / `provider`）作为新建 Cline 渠道时的前端预填值（参照 `channel-type-defaults.ts` 的 OpenCode Go 默认 `header_override` 模式），管理员可编辑或清空。

- 理由：需求已确认该覆盖「可编辑」，前端预填非硬编码更符合既有模式。
- 备选：后端 adaptor 强制注入。否决原因：会导致管理员无法编辑/清空。

### D6：自定义图标

新增 `web/src/assets/custom/icon-cline.tsx` 并注册到 `web/src/lib/lobe-icon.tsx` 的 `CUSTOM_ICONS`，在渠道类型→图标映射中登记类型 94。

## Risks / Trade-offs

- [EndpointProfile 取值与既有枚举冲突] → 采用稳定、不冲突的取值 token，并在设计阶段与既有 `coding` 系列对齐后再落地。
- [计划方式取模型的 `clinePass` 数组形状随上游变化] → 解析时对缺失/空数组做容错，返回空列表而非报错，并保留原始错误信息。
- [默认覆盖中的 provider 限定值为字面量，可能与实际所选模型不匹配] → 作为可编辑预填，管理员可自行修改；在设计阶段确认是否需要改为模型相关值。
- [三种路由的上游映射假设] → 需在设计阶段确认 Cline 上游对 `chat`/`responses`/`messages` 的原生支持情况，必要时统一协议。
- [param_override 简写格式不做深合并] → 默认覆盖以整块顶层 key 注入；若上游请求体已有同名顶层 key，需在其存在性场景下验证覆盖效果。

## Migration Plan

1. 后端注册渠道类型、API 类型映射、adaptor、取模型与计划用量分支。
2. 前端注册类型、图标、表单默认值与用量入口、i18n。
3. 无需数据库迁移；历史数据不受影响。回滚方式为回退代码版本。

## Open Questions

- `EndpointProfile` 面向 Cline 的具体取值 token：已定为 `clinepass`（按量为空），并在 Task 1 落地、Task 5 前端对齐。
- Cline 上游鉴权头的确切形式与默认 `header_override` 是否需要额外头：实现采用 `Authorization: Bearer {api_key}`；真实渠道核实留待端到端验证。
- 默认覆盖中的 `only` 取值是否保持固定字面量或改为模型相关：采用给定字面量 `["deepseek"]` 作为可编辑预填。

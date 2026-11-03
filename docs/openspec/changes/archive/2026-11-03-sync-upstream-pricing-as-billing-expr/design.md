---
comet_change: sync-upstream-pricing-as-billing-expr
role: technical-design
canonical_spec: openspec
archived-with: 2026-11-03-sync-upstream-pricing-as-billing-expr
status: final
---

# Design

## Context

当前价格同步链路：后端 `FetchUpstreamRatios` 抓取上游并计算差异 → 前端选择来源与调价 → `applyPriceSyncSelections` 把选中值合并进定价草稿 → `buildPricingChanges` → `PATCH /api/option/model_pricing` 版本化保存。

项目已有 legacy→expression 转换能力：`POST /api/option/model_pricing/convert`（`PreviewModelPricingConversion`）接收完整的旧定价草稿，返回 `expression`、生效价格快照 `effective` 或具体的 `unsupported_reason`，不写数据库；定价编辑器已使用该接口。转换规则与限制见 `pkg/billingexpr/expr.md`「旧定价转换」。提案动机见 `proposal.md`。

模型价格主列表由 `buildModelSnapshots` 汇总各定价 option map 的 key 生成；「未设置价格」标签使用启用模型列表（`/api/channel/models_enabled`）并过滤出没有基础定价的模型。

opencode-go 的 `https://models.opencode.ai/api.json` 是静态目录：每个模型只有一个 `cost` 对象，包含 `input`、`output`、`cache_read`、`cache_write` 与可选的 `tiers`（上下文阶梯）；DeepSeek 条目的 `cost` 是 off-peak 单价，不包含 peak 档，也不随请求时间变化。OpenCode Go 文档定义 peak = 2× off-peak，peak 为 UTC 周一至周五 `01:00–04:00` 与 `06:00–10:00`，其余时间含周末为 off-peak。

## Goals / Non-Goals

**Goals:**

- 价格同步保存时默认把可转换的选中模型落库为 `tiered_expr`，价格与所选上游值等价。
- 同步只处理当前启用渠道提供的模型，`differences` 与 `prices` 使用同一范围。
- 主列表只展示已设置基础定价的模型；「未设置价格」标签语义不变。
- 提供批量清空选中模型定价的入口，复用现有草稿与保存流程。

**Non-Goals:**

- 不迁移已按 ratio 落库的历史定价。
- 不改变抓取、来源优先级、选择与调价交互。
- 不改变 ratio 模式的运行时支持。
- 不为同步流程新增“保持旧模式”开关。
- 不改变「未设置价格」标签的定义，也不自动停用或删除模型。

## Decisions

### 1. 在调价之后、保存之前逐模型转换

转换使用用户最终确认的选中值（含汇率调价结果），在 `handleApplySync` 生成保存草稿时调用现有 convert API。

- 理由：调价会改变最终数值，若在抓取/预览阶段转换，结果会与保存值不一致。
- 备选：新增后端批量转换接口——为纯前端流程扩大公共 API 面，否决。
- 备选：在 `PATCH /api/option/model_pricing` 保存时自动转换——会改变手动保存的语义，否决。

### 2. 转换输入与将要保存的行保持一致

用最终选中值构造完整 legacy 草稿（与 `pricingRow` / `pricingFromDraft` 的结果一致），不混入未选中的本地旧字段。

- 理由：同步是字面导入，草稿应只反映本次选择；混入本地值会让转换结果偏离所选上游价格。
- 未选择的模型不进入转换，也不产生变更。

### 3. 转换成功时同时保存表达式与生效 legacy 字段

转换成功时写入 `billing_mode = "tiered_expr"` 与 `billing_expr`，并把转换返回的 `effective` legacy 字段一并保存；上游本身提供 `billing_expr` 时原样使用，不二次转换。

- 理由：与编辑器转换后的保存行为一致，保留切回旧模式所需的回退数据。
- 备选：只保存表达式——会丢失回退数据，否决。

### 4. 无法转换时保留 ratio 并提示

`unsupported_reason` 或转换请求失败时，该模型按现有 ratio 字段保存，并记录“需要手工转换”的提示；其他模型继续保存。

- 理由：用户已确认的回退策略；单个模型失败不应阻塞整批同步。
- 提示需要包含模型名与原因，便于管理员后续在定价编辑器中手工转换。

### 5. 受控并发与保存状态

选中模型可能较多，转换按固定并发上限（建议 4）执行，期间保存按钮进入 loading；每个模型的失败独立处理。

- 理由：避免数百个模型串行请求造成明显等待，也避免无限并发压垮后端。
- 转换结果按模型缓存于本次保存流程内，不写入全局状态。

### 6. 同步范围改为当前启用的模型

后端在 `FetchUpstreamRatios` 中用 `model.GetEnabledModels()`（启用渠道的 distinct model）构建允许集合，替代当前基于模型库的 `GetAllModelNames()`；`differences`、`prices` 以及 `include_unchanged` 分支统一使用该集合。

- 理由：「当前模型」指实际由启用渠道提供的模型；与「未设置价格」标签的数据源一致，也避免模型库中未启用的历史模型进入同步结果。
- `include_unchanged` 目前会把上游全部模型加入 `prices`，必须与 `differences` 使用同一过滤，恢复“价格集合是差异集合子集”的约束。
- 备选：继续使用模型库并只修复 `include_unchanged`——仍会包含未启用的历史模型，不满足“只同步现在的模型”。

### 7. 主列表按基础定价过滤

主列表（`filterMode !== 'unset'`）只保留 `isBasePricingUnset` 为 false 的行；仅有 `completion_ratio`、`cache_ratio` 或 `billing_mode = "ratio"` 等孤立字段的模型不再显示。非空的任务插件表达式也算基础定价；检测到定价问题的模型即使没有基础定价也保留，避免隐藏需要处理的插件配置问题。

- 理由：行的存在不应由任意配置字段决定；没有基础定价且没有问题的模型属于「未设置价格」标签，但已配置插件表达式或存在待处理问题的任务模型必须保留在主列表。
- 保留 `buildModelSnapshots` 的原始输出，只在展示层过滤，避免影响草稿删除、冲突检测和保存签名。
- 「未设置价格」标签保持现有 `candidateModelNames` + `isBasePricingUnset` 逻辑，不改为依赖定价配置。

### 8. 批量清空复用单行删除的字段清理

在 `Reset prices` 旁新增「清空」按钮，作用于当前选中行；清理逻辑与现有单行 `handleDelete` 共用，删除全部定价字段，随后按现有草稿/保存流程提交。未选中时按钮禁用，执行前使用现有确认对话框并显示模型数量。

- 理由：单行删除已覆盖全部字段；复用可保证批量与单行语义一致。
- 备选：调用后端 `DeleteModelPricingConfig`——绕过版本化保存与冲突检查，且与当前草稿模型不一致，否决。
- 清空入口只在可视列表模式可用；JSON 模式继续使用文本编辑。

### 9. opencode-go 源直接生成表达式

opencode-go 的转换在服务端直接产出 `billing_mode = "tiered_expr"` 与 `billing_expr`，不再先转成 ratio 再由前端转换回来。表达式系数直接使用目录中的 USD/百万 token 单价：

- `input` → `p`，`output` → `c`，`cache_read` → `cr`，`cache_write` → `cc`。
- 存在 `tiers` 时按 `len <= size ? tier("standard", ...) : tier("long_context", ...)` 生成阶梯；多个阶梯按尺寸升序串联。
- 目录缺省的字段不生成对应项，避免把未知价格当成 0。

理由：目录本身就是真实美元单价，直接生成表达式可以避免 ratio 往返的精度损失，并让同步表格直接显示表达式。备选：继续转 ratio 再走前端转换——会多一次舍入，且同步预览仍显示 token 模式，否决。

### 10. DeepSeek 闲时/忙时使用内置规则

opencode-go 的 DeepSeek 条目（`deepseek-v4.1-flash`、`deepseek-v4-pro`）不直接采用目录中的单一 `cost`，而是按内置规则生成两档表达式：

- 目录中的 `input`、`output`、`cache_read` 作为 off-peak 系数。
- peak 系数固定为 off-peak 的 2 倍。
- peak 条件使用 `weekday("Asia/Shanghai")` 与 `hour("Asia/Shanghai")`：周一至周五 `09:00–12:00`、`14:00–18:00`；其余含周末为 off-peak。

同时在 `setting/billing_setting/builtin_billing.go` 补充 `deepseek-v4.1-flash` 的内置表达式，作为未经过同步时的默认峰谷价格。理由：目录只发布 off-peak 价，直接采用会在 peak 时段少收一半；以目录 off-peak 为基准推导 peak，可以在上游调价时保持同步，内置表达式只作兜底。备选：完全硬编码两档价格——上游调价后需要手工更新，否决。

### 11. opencode-go 零价与缺省字段

目录中所有价格均为 0 的免费模型生成 `tier("base", fixed(0))`；缺少 `output` 时按旧 completion ratio 解析（`ratio_setting.ResolveCompletionRatio(name, nil)`，含模型族硬编码倍率）由输入价推导输出价，缺少 `cache_read`/`cache_write` 时不生成对应计费项。病态价格（NaN/±Inf/负值/超过现有上界）沿用现有策略：跳过该模型并告警。

- 理由：免费模型用固定 0 价表达最清晰；缺省 output 不能写成免费，必须与旧引擎的输出价一致；缺省缓存字段的 token 会留在 `p` 中按输入价计费，与旧缓存倍率 1 的默认行为一致。
- `cache_write` 统一映射为通用 `cc`；目录没有 TTL 区分，不推断 `cc1h`/`cc12h`。

## Risks / Trade-offs

- [表达式预扣与舍入和旧 ratio 不完全一致] → 验收以相同 token 用量下费用等价为准，spec 已明确不要求逐笔舍入一致。
- [逐模型转换增加保存耗时] → 固定并发上限并显示进度；失败只回退单模型。
- [转换依赖渠道与模型元数据，结果可能随渠道状态变化] → 返回 `unsupported_reason` 时回退 ratio 并提示，不阻断保存。
- [legacy 字段与表达式不一致会导致切回旧模式价格漂移] → 使用转换返回的 `effective` 一并保存，保证回退价格与表达式等价。
- [启用模型范围与模型库预期不同] → 在 proposal/spec 明确范围；未启用模型不会进入同步结果，也不会被自动修改。
- [主列表隐藏未设置行可能让用户误以为数据被删除] → 「未设置价格」标签仍可查看启用模型；清空是显式且需确认的操作。
- [批量清空误操作] → 使用确认对话框并显示选中数量；改动在保存前仍是草稿。
- [opencode-go 目录新增未知阶梯形态] → 转换只消费已知字段；未知字段忽略并保留基础档，测试覆盖 `tiers` 与 `cache_write`。
- [DeepSeek 峰谷规则或价格变化] → 规则与内置表达式集中维护；测试覆盖 peak/off-peak 边界，价格变化时更新内置表达式。
- [含 `weekday`/`hour` 的表达式无法生成可视化预览] → 同步表格回退展示原始表达式，不阻断选择与保存。

## Migration Plan

- 后端仅改变同步查询范围，无数据库迁移与 schema 变更。
- 前端行为变更随构建发布；历史 ratio 定价保持不变。
- 回滚：回退后端与前端构建即可；已保存的表达式定价继续正常运行，必要时可切回已保留的 legacy 字段。

## Open Questions

（无）

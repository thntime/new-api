# Proposal

## Why

管理界面已将「按 Token（ratio）」和「按次」定价标记为已弃用，新建定价默认使用计费表达式；但「上游价格同步」在保存时仍显式写入 `billing_setting.billing_mode = "ratio"`（`web/src/features/model-pricing/pricing.ts` 的 `applyPriceSyncSelections`）。同时，同步表格会把上游目录中的全部模型带入结果，而模型价格主列表在删除定价后仍会展示只有孤立配置字段、没有基础价格的模型。这三点共同导致同步结果与当前定价策略、当前启用模型范围不一致。

## What Changes

- 上游价格同步在合并选中的上游价格时，默认通过现有 legacy→expression 转换生成计费表达式，并保存为 `billing_mode = "tiered_expr"`。
- 上游本身提供非空 `billing_expr` 时保持原表达式，不做二次转换。
- 固定按次价格转换为 `tier("request", fixed(price))`；图片按次价格转换为 `tier("image", fixed(price)) * image_count`。
- 无法安全转换的模型（任务插件、视频、Realtime、依赖上游 cost 反推缓存写入的 OpenRouter Claude、音频单价冲突等）保留 ratio 落库，并在同步结果中提示需要手工转换。
- opencode-go 来源的模型直接以 `tiered_expr` + 表达式进入同步结果，不再转换为 ratio；表达式覆盖 `input`、`output`、`cache_read`、`cache_write` 以及上下文阶梯 `tiers`。
- DeepSeek 使用内置的闲时/忙时规则：opencode-go 目录只提供 off-peak 单价，peak 按 2× 生成；peak 时段为 UTC 周一至周五 `01:00–04:00`、`06:00–10:00`（Asia/Shanghai `09:00–12:00`、`14:00–18:00`），其余含周末为 off-peak。
- 价格同步的模型范围改为「当前启用的模型」（启用渠道提供的模型），`differences` 与 `prices` 使用同一范围；`include_unchanged` 分支不得绕过该过滤。
- 模型价格主列表只展示已设置基础定价（`model_ratio`、`model_price` 或 `tiered_expr` + 表达式）的模型；只有孤立配置字段的模型不再出现在主列表。
- 「未设置价格」标签保持原语义：展示当前启用、但没有设置基础定价的模型。
- 在「Reset prices」旁新增批量「清空」：清空选中模型的全部定价字段（ratio、price、completion、cache、image、audio、billing_mode、billing_expr 与插件表达式）。
- 已按 ratio 落库的历史模型不在本次迁移；同步抓取、来源优先级和价格选择交互保持不变。

## Capabilities

### New Capabilities

- `model-pricing-management`: 管理端模型价格列表的展示范围与批量清理行为。

### Modified Capabilities

- `model-price-sync`: 同步范围限定为当前启用的模型；同步保存后的定价默认落库为计费表达式，无法安全转换时回退 ratio 并给出提示。

## Impact

- 后端：`controller/ratio_sync.go` 的模型范围过滤（`GetAllModelNames` → 启用模型，并覆盖 `include_unchanged` 分支）；`service/opencodego_models.go` / opencode-go 转换改为直接产出表达式；`setting/billing_setting/builtin_billing.go` 补充 DeepSeek V4.1 Flash 的峰谷表达式。
- 前端：`web/src/features/model-pricing/pricing.ts`（`applyPriceSyncSelections`）、`web/src/features/system-settings/models/upstream-ratio-sync.tsx` 的保存与提示流程；`model-ratio-visual-editor.tsx`、`model-ratio-form.tsx`、`model-pricing-snapshots.ts` 的列表过滤与批量清空。
- 计费语义：转换后的预扣与舍入按表达式规则执行，不保证与旧 ratio 模式逐笔舍入完全一致；验收以价格等价为准（见 `pkg/billingexpr/expr.md`「旧定价转换」）。
- 数据：只影响此后经同步保存的模型；无数据库迁移，无 schema 变更。

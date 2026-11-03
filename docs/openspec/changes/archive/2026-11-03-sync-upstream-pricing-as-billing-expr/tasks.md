# Tasks

## 1. 同步范围过滤（后端）

- [x] 1.1 在 `controller/ratio_sync.go` 用启用模型集合（`model.GetEnabledModels()`）替代模型库 `GetAllModelNames()`，并让 `differences` 与 `prices` 使用同一过滤。验证：新增/扩展 `controller/ratio_sync` 测试，覆盖上游独有模型、仅停用渠道模型、启用模型三种情况。 <!-- comet-task:8c1f22fd-7567-4bae-a77d-57e8e10be070 -->
- [x] 1.2 让 `include_unchanged = true` 分支也应用同一启用模型过滤，保证 `prices` 的模型键是过滤后 `differences` 的子集。验证：测试断言 `include_unchanged` 时 `prices` 不包含上游独有或未启用模型。 <!-- comet-task:0b252e32-bc62-4240-b9b7-9a56453020c7 -->

## 2. opencode-go 表达式与 DeepSeek 峰谷（后端）

- [x] 2.1 将 opencode-go 转换改为直接产出 `billing_mode = "tiered_expr"` + 表达式：`input`→`p`、`output`→`c`、`cache_read`→`cr`、`cache_write`→`cc`，存在 `tiers` 时生成 `len` 条件分支。验证：单元测试覆盖普通模型、带上下文阶梯、`cache_write`、缺省字段四种情况，断言表达式系数等于目录 USD/百万 token 单价。 <!-- comet-task:1e44bc38-c143-4213-a6b2-d9596643e375 -->
- [x] 2.2 在 `setting/billing_setting/builtin_billing.go` 补充 `deepseek-v4.1-flash` 的峰谷表达式，并让 opencode-go 的 DeepSeek 条目优先使用内置表达式（off-peak 取自目录，peak 为 2×）。验证：测试断言工作日 `09:00–12:00`、`14:00–18:00` 命中 peak，周末与其余时段命中 off_peak，且 peak 系数是 off-peak 的 2 倍。 <!-- comet-task:f668fd5c-73a4-4df9-85ae-c08e146a9e11 -->

## 3. 表达式转换与合并（前端）

- [x] 3.1 在 `web/src/features/model-pricing/pricing.ts` 增加异步解析入口：对每个选中模型，若选中值不含非空 `billing_expr`，用最终选中值构造完整 legacy 草稿并调用 `previewModelPricingConversion`；成功后按 `tiered_expr` + 表达式 + `effective` legacy 字段合并，`unsupported_reason` 或请求失败时保留 ratio 并返回 warning。验证：单元测试覆盖转换成功、`unsupported_reason`、请求失败三条路径，断言各自的 `billing_mode` 与 warning。 <!-- comet-task:0cef9d17-8793-4980-919d-0643c20d02e0 -->
- [x] 3.2 保持 `applyPriceSyncSelections` 为纯合并函数，并让转换结果复用同一合并路径；上游已提供 `billing_expr` 时不调用转换。验证：测试断言表达式原样写入、未调用转换，且现有 `pricing-sync` 行为不变。 <!-- comet-task:353e3da5-794f-485e-9598-163faf1b6f48 -->
- [x] 3.3 为转换增加固定并发上限（4）与单模型独立失败处理，结果只在本次保存流程内使用。验证：测试断言同时进行中的转换数不超过 4，且单个失败不影响其他模型的合并结果。 <!-- comet-task:dd537bff-aa2c-4b52-a5ce-1a8d13b10fc7 -->

## 4. 同步保存流程与提示

- [x] 4.1 将 `web/src/features/system-settings/models/upstream-ratio-sync.tsx` 的 `handleApplySync` 改为异步：转换期间禁用保存并显示进度，转换完成后进入现有确认对话框。验证：组件测试 mock convert API，断言确认对话框在转换完成后出现且期间保存不可用。 <!-- comet-task:b2adec55-b912-4308-af0e-b7bff36eff97 -->
- [x] 4.2 在确认对话框或保存结果中展示回退提示，包含模型名与 `unsupported_reason`，并确保整体保存仍成功。验证：组件测试断言不可转换模型出现在提示中、可转换模型保存为表达式。 <!-- comet-task:6954f892-3b4d-45e6-8794-3eabdabe4e01 -->
- [x] 4.3 同步表格展示 opencode-go 表达式；含 `weekday`/`hour` 的表达式无法生成可视化预览时回退为原始表达式，不报错。验证：组件测试断言 opencode-go 行显示 expression 类型，DeepSeek 行渲染不抛错。 <!-- comet-task:62b132de-8451-4b21-b308-73f02f53d652 -->
- [x] 4.4 为新增提示文案补充 i18n，写入全部 7 个 locale 文件。验证：`cd web && bun run i18n:sync` 无新增缺失项，且 en/zh 均能取到文案。 <!-- comet-task:64ba0600-0174-44f9-93df-9a25a7fc49d3 -->

## 5. 主列表可见性与批量清空

- [x] 5.1 在 `web/src/features/system-settings/models/model-ratio-visual-editor.tsx` 的主列表过滤掉 `isBasePricingUnset` 的行；「未设置价格」标签保持现有 `candidateModelNames` + `isBasePricingUnset` 逻辑。验证：测试覆盖「仅剩 billing_mode/其他倍率字段 → 主列表不显示」「已设置 ratio/price/表达式 → 显示」「启用但未设置 → 只出现在未设置标签」。 <!-- comet-task:5e09285b-ee4a-4141-905c-1fcd68ac6e91 -->
- [x] 5.2 在 `web/src/features/system-settings/models/model-ratio-form.tsx` 的 `Reset prices` 旁新增「清空」按钮，作用于当前选中行；复用单行删除的字段清理逻辑，删除全部定价字段；未选中时禁用，执行前使用现有确认对话框并显示数量。验证：组件测试断言按钮位置、禁用状态、确认流程，以及清空后选中模型的全部定价字段被移除。 <!-- comet-task:24d1eb7e-8b6d-47ed-a859-61b897804f92 -->
- [x] 5.3 清空改动进入现有草稿流程，保存后生效并从主列表消失；未保存不写服务端。验证：测试断言清空后未保存时服务端未被调用，保存后请求体包含全部字段删除。 <!-- comet-task:34adfa89-e83f-4a19-aa74-dd9ca3fe9151 -->
- [x] 5.4 为「清空」按钮与确认文案补充 i18n，写入全部 7 个 locale 文件。验证：`cd web && bun run i18n:sync` 无新增缺失项，且 en/zh 均能取到文案。 <!-- comet-task:1cfa3fe5-e082-4b4a-aa4c-70c8c235d073 -->

## 6. 集成验证

- [x] 6.1 运行后端受影响测试：`go test ./controller/ ./service/ ./setting/billing_setting/ -run 'RatioSync|OpenCodeGo|DeepSeek|BillingExpr'`，确认通过并记录输出。 <!-- comet-task:e6a3e875-fea0-4872-86fe-924e494ce17c -->
- [x] 6.2 运行前端类型检查 `cd web && bun check -b`，确认无类型错误。 <!-- comet-task:fb3b1fe9-05cb-497f-bd8c-3c7453643c7e -->
- [x] 6.3 运行受影响测试 `cd web && bunx vitest run src/features/system-settings/__tests__/pricing-sync.test.tsx src/features/system-settings/models/__tests__ src/features/model-pricing/__tests__`，确认全部通过。 <!-- comet-task:2d9c9d92-4111-414a-99b2-5b3b351c8327 -->
- [x] 6.4 对受影响文件执行 lint 并修复 error，确认无遗留 lint error。 <!-- comet-task:48dc28b1-fac8-4ec5-8b1c-f236b14425c4 -->
- [x] 6.5 用固定测试数据完成端到端验收：opencode-go 普通模型 → 同步结果直接显示表达式；DeepSeek → peak/off-peak 两档且 peak 为 2×；上游仅倍率 → 保存后 `tiered_expr` 且费用等价；不可转换模型 → 保留 ratio 并提示；上游独有/停用模型 → 不出现在同步结果；删除 JSON 定价 → 主列表不再显示该行；批量清空 → 选中模型全部定价被删除。记录实际结果。 <!-- comet-task:e3f52b03-dae4-466b-a9a1-dec75de1ffbe -->

## Workflow follow-up

- 通过 Verify 后归档 change，并核对归档结果与交付状态。

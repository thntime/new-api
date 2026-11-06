# Tasks

## 1. 目录解析（后端核心）

- [x] 1.1 重构 `service/opencodego_models.go` 的 `collectOpenCodeGoModels`：先写入 `opencode-go` 段条目，再按段名升序遍历其余 provider 段，仅在该 id 不存在时补入条目，存在时按字段粒度补齐空值；验证：`go test ./service/ -run OpenCodeGo` 通过，覆盖「精确匹配补入」「大小写/前缀不同不匹配」「已有字段不被覆盖」。
- [x] 1.2 保证同一 id 只产出一个条目且输出顺序稳定，并把 `parseOpenCodeGoModels`、`parseOpenCodeGoModelEntries`、`convertOpenCodeGoRatioData` 统一到该结果；验证：新增「同一输入连续解析两次结果一致」用例与「多 provider 段同 id 只出现一次」用例通过。
- [x] 1.3 保持免费 `-free` 模型与价格校验语义：免费零成本条目仍覆盖同 id，且保留原条目非空字段；非法 input 跳过模型、缺失 input 跳过而非按 0 计费、非法 output/cache_read 只跳过对应比值；验证：`go test ./service/ -run 'OpenCodeGo|Ratio'` 全部通过。

## 2. 同步入口接线

- [x] 2.1 核对 `controller/model_sync.go` 的 `fetchOpenCodeGoCatalog`：回退条目的 `provider.npm` 参与 vendor 判定，缺失时回退默认 vendor，富字段组装不因回退条目缺字段而报错；验证：`go build ./...` 通过，并新增/扩展用例覆盖「回退条目的 vendor 与富字段」。
- [x] 2.2 核对 `controller/ratio_sync.go` 的 opencode-go 预设：价格候选模型集合是同步模型入口集合的子集（额外剔除缺失/非法价格）；验证：新增用例对比两个入口对同一 api.json 得到的模型 id 集合，断言子集关系且缺失价格模型不出现在价格结果中。

## 3. 前端筛选栏收尾

- [x] 3.1 复核 `Expand` / `Collapse` 键：两者已存在于 7 个 locale 的 `translation` 命名空间且译文齐全，无需新增；验证：`jq -r '[.translation.Expand, .translation.Collapse] | @tsv'` 对 7 个文件均返回非空。
- [x] 3.2 确认 `More filters` / `Fewer filters` 已无引用后保持删除状态；验证：`grep -rn "More filters\|Fewer filters" web/src` 无结果。
- [x] 3.3 新增 `web/src/features/performance-health/__tests__/filter-bar.test.tsx`，覆盖收起态（Badge 显示计数、文案 Expand、`aria-expanded=false`）、展开态（文案 Collapse、无 Badge、`aria-expanded=true`）、计数为 0 时不渲染 Badge；验证：`cd web && bunx vitest run src/features/performance-health/__tests__/filter-bar.test.tsx` 通过。
- [x] 3.4 复核 Badge 与按钮尺寸改动同 `logs-filter-toolbar.tsx` 保持一致（含计数 Badge 对读屏语义弱化的取舍），并运行前端类型检查；验证：`cd web && bunx tsc --noEmit` 通过。

## 4. 集成验证

- [x] 4.1 后端：`go build ./...` 与 `go test ./service/ ./controller/ -run 'OpenCodeGo|Metadata|Ratio'` 全部通过。
- [x] 4.2 前端：`cd web && bun run build` 成功（含 i18n 键完整性检查）。

## Workflow follow-up

- 验证通过后按 Comet 流程归档，并在用户确认后统一提交本次改动（含工作区已有的 performance-health/i18n 改动）。

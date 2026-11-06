# Design

## Context

见 `proposal.md` - Why。当前实现集中在 `service/opencodego_models.go`：

- `collectOpenCodeGoModels(upstream)` 只读取 `opencode-go` 段，再用 `opencode` 段的免费 `-free` 模型覆盖同 id 条目。
- `parseOpenCodeGoModels` / `parseOpenCodeGoModelEntries` / `convertOpenCodeGoRatioData` 三个消费方都调用该函数，因此模型集合与字段取值天然一致。
- `controller/model_sync.go` 的 `fetchOpenCodeGoCatalog` 在解析结果之上组装 vendor 与富元数据；`controller/ratio_sync.go` 走 `ConvertOpenCodeGoToRatioData`。

约束：api.json 约 6 MB、230 余个 provider 段、按 id 去重约 4147 个模型；`opencode-go` 段 33 个。解析必须确定性（同一输入两次解析结果一致），价格换算的安全校验不得放松。

## Goals / Non-Goals

**Goals:**

- 在单一解析函数内完成「opencode-go 优先 + 其他段精确 id 回退 + 字段级补齐」，三个消费方无需各自实现。
- 保证同一份 api.json 的解析结果稳定（条目唯一、顺序确定）。
- 保留现有免费模型合并规则与价格校验规则。

**Non-Goals:**

- 不改变 api.json 的拉取方式、超时、代理设置。
- 不改变 `/api/channel/fetch_models`、模型同步、价格同步的接口形状。
- 不为 4000+ 候选模型做分页/懒加载（若后续确认有性能问题再单独立项）。
- 不引入 provider 白名单或数量上限。

## Decisions

### 1. 回退逻辑放进 `collectOpenCodeGoModels`，保持函数契约

该函数返回 `map[string]openCodeGoModel`，是三个消费方的唯一入口。把条目级回退与字段级补齐放在这里，可避免元数据同步与价格同步出现两套规则。

替代方案：在 `parseOpenCodeGoModelEntries` 与 `convertOpenCodeGoRatioData` 各写一份回退。放弃，因为两处规则必然漂移。

### 2. 遍历顺序：`opencode-go` 段先入，其余段按段名升序

先写入 `opencode-go` 条目；随后对剩余段名排序后遍历，只在该 id 尚不存在时补入条目，存在时按字段补齐。这样「同 id 取哪个回退来源」由段名顺序唯一确定，不依赖 Go map 遍历顺序。

替代方案：按 provider 段在 JSON 中的出现顺序。放弃，因为 JSON 对象顺序在解析到 map 后不可靠。

### 3. 字段级补齐按字段粒度，不做整条替换

补齐只填充空值：字符串为 `""`、指针为 `nil`、切片/映射长度 0。`cost` 是含指针字段的结构体，按 `input` / `output` / `cache_read` 分别判断与补齐，避免「input 有值就整条跳过」。

`opencode-go` 已提供的非空值一律不覆盖。

### 4. 免费模型规则保持在最后一步

顺序为：`opencode-go` 入表 → 其他段条目级回退 → `opencode` 段 `-free` 零成本条目覆盖同 id（保留原条目非空字段）。与现有实现相比只多出中间一步，免费语义不变。

### 5. 价格与元数据不改校验/映射

`convertOpenCodeGoRatioData` 继续复用 `isValidOpenCodeGoCost` 与 `maxRatioBound`；`cost.input` 缺失（nil）的条目跳过并告警，避免未知价格按 0 计入 model_ratio；`fetchOpenCodeGoCatalog` 继续用 `openCodeGoVendorForModelAndProvider(entry.ID, entry.Provider)`，回退条目的 `provider.npm` 参与 vendor 判定，缺失时回退现有默认 vendor。

### 6. 前端只补 i18n 键，不改交互

`filter-bar.tsx` 的展开/收起改造保留；`Expand` / `Collapse` 键已存在于 7 个 locale 的 `translation` 命名空间，只需复核译文完整，并用全仓库检索确认 `More filters` / `Fewer filters` 无其他引用后删除。不新增组件、不改布局。

## Risks / Trade-offs

- [候选模型从 33 增至约 4147，同步向导与价格预览列表显著变大] → 后端保持稳定排序与一次计算；前端沿用现有筛选/分页能力。若实测卡顿，再单独立项做分页。
- [同 id 在不同 provider 的价格差异大，回退结果可能偏离预期] → 明确「opencode-go 优先 + 段名升序」的确定性规则，并在 spec 场景中固定；用户仍可在同步向导中不选该条目。
- [字段级补齐可能把不同语义的字段拼在一起] → 只补空值，且仅限同名 id；不改变已有值。
- [免费模型覆盖与字段补齐顺序出错导致回归] → 保留现有 `-free` 覆盖测试，并新增「免费条目缺字段时回退原条目」的用例。
- [前端删键导致其他页面缺翻译] → 删除前全仓库检索两个键的引用。

## Migration Plan

无数据库或配置迁移。回滚即还原 `collectOpenCodeGoModels` 与 locale 文件改动。

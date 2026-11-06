# Design

## Context

见 `proposal.md` - Why。

- `FetchUpstreamRatios`（`controller/ratio_sync.go:217`）并发拉取各来源，转换为 ratio 结构后调用 `buildDifferences`（`:653`）；`buildDifferences` 以「本地价格数据 ∪ 各来源数据」的并集作为模型全集（`:669-683`），因此上游独有的模型也会生成差异行。`prices` 由 `differences` 派生（`:634-641`）。
- `collectOpenCodeGoModels`（`service/opencodego_models.go`）先写入 `opencode-go`，再把其余段名排序后遍历；`opencode` 段因此落在字母序位置，而不是固定第二。

## Goals / Non-Goals

**Goals:**

- 价格同步的模型集合严格等于本地模型库中的模型名集合。
- 目录解析的来源优先级明确为 opencode-go → opencode → 其余段（段名升序）。

**Non-Goals:**

- 不改变模型元数据同步的候选范围（仍为全量目录）。
- 不改变接口响应结构与字段语义。
- 不引入模型名大小写折叠或别名匹配。

## Decisions

### 1. 在 `buildDifferences` 之后按模型库过滤差异集合

过滤点选在差异计算完成、构建 `prices` 之前：一处改动即可覆盖所有来源，且 `prices` 天然与过滤后的 `differences` 一致。

替代方案：在每个来源转换后分别过滤。放弃，因为需要改动 4 个来源分支，且容易漏掉后续新增来源。

### 2. 新增 `model.GetAllModelNames()`，查询失败即返回错误

用 `DB.Model(&Model{}).Pluck("model_name", &names)` 读取模型名，与 `model/missing_models.go` 的既有用法一致。查询失败时返回 500，而不是退化为「不过滤」——否则会在无提示的情况下把数千个上游模型重新带回同步结果。

替代方案：缓存模型名集合。放弃，当前请求频率低且查询本身很轻，缓存会引入失效问题。

### 3. 来源优先级用显式顺序列表

遍历顺序改为：`opencode-go`（步骤 1 已写入）→ `opencode` → 其余段名升序。实现上先构造排除这两段的排序列表，再把 `opencode` 插到最前。免费 `-free` 覆盖仍放在最后一步，语义不变。

替代方案：给段名加权重前缀。放弃，可读性差且难以解释。

## Risks / Trade-offs

- [过滤后用户无法通过价格同步为「模型库中没有」的模型补价格] → 这是本次需求的目标行为；模型需先进入模型库（同步模型）后才可同步价格。
- [模型库与上游存在大小写/别名差异时会被过滤] → 精确匹配是刻意选择；如需支持需单独定义映射规则。
- [模型库规模大时 `Pluck` 占用内存] → 万级字符串可接受；查询为单次请求内一次性读取。
- [优先级调整改变回退来源，个别模型价格可能与之前不同] → 已由 spec 场景与测试固定，且 opencode 官方段比聚合段更可信。

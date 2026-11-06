# Proposal

## Why

「同步模型」（来源 OpenCode Go）与「同步价格」（opencode-go 预设）都从 `https://models.opencode.ai/api.json` 取数，但目前只解析 `opencode-go` 段（33 个模型）以及 `opencode` 段的免费 `-free` 模型。凡是只出现在其他 provider 段的模型，其元数据与价格完全缺失，需要人工补录。

同时，性能健康页筛选栏的展开/收起改造已经写入工作区但未提交：按钮文案改用 `Expand` / `Collapse`，而这两个键在 7 个 locale 文件中都不存在，旧键 `More filters` / `Fewer filters` 又已被删除，非英文环境会回退显示英文源串。

## What Changes

- **目录解析改为 opencode-go 优先 + 精确回退**：按模型 id 解析 api.json。id 在 `opencode-go` 段时以该条目为准；不在时，从其他 provider 段中按 **id 完全相同**（不做大小写折叠、前缀/后缀或别名匹配）的条目补入。同一 id 只产出一个条目。
- **字段级补齐**：模型已存在于 `opencode-go` 段但某字段缺失（description、name、cost、provider、family、limit、modalities、能力标记等）时，用其他 provider 段同 id 条目的该字段补齐；`opencode-go` 已有的值永不被覆盖。
- **两处同步共用同一解析结果**：同步模型（元数据/富字段）与同步价格（ratio 换算）都消费上述解析结果，保证两个入口的模型集合与取值一致。
- **价格安全校验不变**：NaN/±Inf/负值/超过 `maxRatioBound` 的处理规则维持现状；另外，缺少 `cost.input` 的条目改为跳过（不再按 0 计费），避免未知价格被当作免费模型写入计费。
- **前端筛选栏收尾**：保留展开/收起按钮与数量 Badge 的改造，复核 7 个 locale 已有的 `Expand` / `Collapse` 键（位于 `translation` 命名空间），并移除不再引用的 `More filters` / `Fewer filters` 键。
- **规模提示**：api.json 全量 provider 段按 id 去重、剔除 deprecated 后约 3643 个模型，`opencode-go` 段仅 29 个。改动后 OpenCode Go 来源的候选模型会从 29 个增加到约 3643 个，同步向导与价格预览的列表规模显著变大（用户已确认接受该范围）。

## Capabilities

### New Capabilities

- `opencode-catalog`: models.opencode.ai 目录的模型集合解析与逐模型数据解析规则，包括 opencode-go 优先、其他 provider 段精确 id 回退、字段级补齐与确定性去重，供同步模型与同步价格共用。

### Modified Capabilities

（无。现有 `models-page-navigation` 的「同步来源顺序与默认值」不变；`performance-health` 的筛选条件集合不变，本次前端改动只涉及展示与文案。）

## Impact

- 后端：`service/opencodego_models.go`（`collectOpenCodeGoModels` 及解析/拉取函数）、`service/opencodego_pricing.go`（`convertOpenCodeGoRatioData` 复用解析结果）、`controller/model_sync.go`（`fetchOpenCodeGoCatalog` 的 vendor 与富字段组装）。
- 测试：`service/opencodego_test.go` 扩展解析与回退用例。
- 前端：`web/src/features/performance-health/components/filter-bar.tsx`、`web/src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json`。
- 数据规模：OpenCode Go 来源候选模型约 29 → 3643；价格同步的候选 ratio 条目同量级增长（其中约 250 条缺价格会被跳过）。
- 不改变公共 API 形状（`/api/channel/fetch_models`、`/api/models/sync/*`、价格同步接口的请求/响应结构不变）。

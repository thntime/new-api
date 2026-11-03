# Spec Delta

## ADDED Requirements

### Requirement: 只同步当前启用的模型

价格同步结果 MUST 只包含当前启用渠道提供的模型；未启用的模型和上游独有的模型 MUST NOT 出现在 `differences` 或 `prices` 中。`include_unchanged` 为 true 时 MUST 使用同一范围。

#### Scenario: 上游独有的模型被排除
- **WHEN** 上游返回 `kimi-k3` 的价格，而当前没有启用任何提供 `kimi-k3` 的渠道
- **THEN** 该模型不出现在 `differences` 与 `prices` 中

#### Scenario: 已停用模型被排除
- **WHEN** 某模型只存在于已停用的渠道
- **THEN** 该模型不出现在同步结果中

#### Scenario: 启用模型正常比较
- **WHEN** 当前启用的渠道提供 `gpt-5.6-luna`，上游也返回该模型
- **THEN** 该模型按现有规则参与差异比较

#### Scenario: include_unchanged 不扩大范围
- **WHEN** 请求包含 `include_unchanged = true`
- **THEN** `prices` 仍只包含当前启用范围内的模型

### Requirement: opencode-go 同步直接生成表达式

opencode-go 来源的模型 MUST 以 `tiered_expr` + 表达式形式进入同步结果，MUST NOT 转换为 ratio。表达式 MUST 反映目录中的 `input`、`output`、`cache_read`、`cache_write`，并在存在上下文阶梯 `tiers` 时生成对应的 `len` 条件分支。

#### Scenario: 普通模型
- **WHEN** opencode-go 目录中的模型只有基础单价
- **THEN** 同步结果给出单档表达式，系数等于目录中的 USD/百万 token 单价

#### Scenario: 带上下文阶梯
- **WHEN** 模型带有 `tiers`（例如 272K 或 256K 上下文）
- **THEN** 表达式按 `len` 条件生成标准档与长上下文档

#### Scenario: 缓存写入
- **WHEN** 目录提供 `cache_write` 单价
- **THEN** 表达式包含对应的 `cc` 项

#### Scenario: 缺少 output 价格
- **WHEN** 目录条目缺少 `output` 单价
- **THEN** 表达式按旧 completion ratio 解析由输入价推导输出价，MUST NOT 把输出写成免费

### Requirement: DeepSeek 使用内置闲时/忙时规则

opencode-go 的 DeepSeek 模型 MUST 使用内置的闲时/忙时表达式。opencode-go 目录只提供 off-peak 单价，peak MUST 按 off-peak 的 2 倍生成；peak 时段 MUST 为 UTC 周一至周五 `01:00–04:00` 与 `06:00–10:00`（Asia/Shanghai `09:00–12:00`、`14:00–18:00`），其余时间含周末为 off-peak。

#### Scenario: DeepSeek V4.1 Flash
- **WHEN** 从 opencode-go 同步 `deepseek-v4.1-flash`
- **THEN** 保存为包含 peak 与 off_peak 两档的表达式，peak 系数是 off-peak 的 2 倍

#### Scenario: 非高峰时段
- **WHEN** 请求发生在周末或每日 peak 时段之外
- **THEN** 表达式命中 off_peak 档

#### Scenario: 高峰时段
- **WHEN** 请求发生在 UTC 周一至周五 `01:00–04:00` 或 `06:00–10:00`
- **THEN** 表达式命中 peak 档

### Requirement: 同步保存默认生成计费表达式

价格同步保存选中价格时，系统 MUST 为可转换的模型生成计费表达式并保存为 `tiered_expr` 模式，MUST NOT 再写入已弃用的按 Token（ratio）模式。上游已提供非空 `billing_expr` 时 MUST 原样保留该表达式，不得二次转换。

#### Scenario: 上游只提供倍率
- **WHEN** 上游只提供 `model_ratio`、`completion_ratio` 等倍率，用户选中并保存
- **THEN** 该模型以 `billing_mode = tiered_expr` 保存，表达式中的每百万 token 单价与所选倍率等价

#### Scenario: 上游提供固定按次价格
- **WHEN** 上游提供固定 `model_price`，用户选中并保存
- **THEN** 该模型保存为 `tier("request", fixed(price))` 形式的表达式

#### Scenario: 上游已提供表达式
- **WHEN** 上游返回非空 `billing_expr`
- **THEN** 保存后表达式与上游完全一致，不被重新生成或改写

### Requirement: 无法转换的模型保留 ratio 并提示

无法安全转换为表达式的模型（任务插件、视频、Realtime、依赖上游 cost 反推缓存写入的 OpenRouter Claude、音频单价冲突等）MUST 继续按 ratio 落库，并在同步结果中提示需要手工转换；单个模型无法转换 MUST NOT 阻塞其他模型的保存。

#### Scenario: 不可转换模型
- **WHEN** 用户选中的模型无法安全转换为表达式
- **THEN** 该模型仍按 ratio 保存，结果中给出需手工转换的提示，整体保存成功

#### Scenario: 混合选择
- **WHEN** 一次保存同时包含可转换与不可转换的模型
- **THEN** 可转换模型保存为表达式，不可转换模型保持 ratio，全部选中模型均完成保存

### Requirement: 价格等价且不改动未选中模型

转换后的定价 MUST 与所选上游价格等价；预扣与舍入按表达式规则执行，不要求与旧 ratio 模式逐笔一致。同步 MUST NOT 修改本次未选中的模型，也 MUST NOT 迁移此前已按 ratio 落库的历史定价。

#### Scenario: 未选中的模型不受影响
- **WHEN** 用户只选中部分模型保存
- **THEN** 未选中的模型保持原有定价与计费模式

#### Scenario: 转换价格等价
- **WHEN** 倍率定价被转换为表达式
- **THEN** 在相同 token 用量下，表达式计算的费用与所选倍率对应的费用等价

## MODIFIED Requirements

### Requirement: 差异与价格集合一致

`prices` 的模型键集合 MUST 与过滤后的 `differences` 一致，不得出现只在其中一侧存在的模型。

#### Scenario: 排除模型不残留在价格集合
- **WHEN** 某上游模型因不在同步范围内而被过滤
- **THEN** `prices` 中也不存在该模型的条目

#### Scenario: include_unchanged 不绕过过滤
- **WHEN** 请求包含 `include_unchanged = true`
- **THEN** `prices` 仍只包含当前启用范围内的模型，不出现上游独有的模型

## REMOVED Requirements

### Requirement: 只同步模型库中已有的模型

**Reason**: 同步范围改为当前启用的模型，模型库中未启用的模型不再参与同步。

**Migration**: 无数据迁移；同步结果按启用模型重新计算，已有定价保持不变。

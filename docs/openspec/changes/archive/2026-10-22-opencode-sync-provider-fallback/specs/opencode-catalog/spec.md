# Spec Delta

## Purpose

定义 models.opencode.ai 目录（api.json）如何解析出可同步的模型集合与每个模型的字段取值，使「同步模型」与「同步价格」在 opencode-go 优先的前提下，能够对缺失模型精确回退到其他 provider 段。

## ADDED Requirements

### Requirement: opencode-go 段优先

系统解析 models.opencode.ai 目录时，MUST 以 `opencode-go` 段的条目作为同名模型的首选来源；该段已提供的字段值优先级高于其他任何 provider 段。

#### Scenario: 模型同时存在于两段
- **WHEN** 某模型 id 同时出现在 `opencode-go` 段与另一个 provider 段
- **THEN** 解析结果采用 `opencode-go` 条目的取值

#### Scenario: 模型仅存在于 opencode-go 段
- **WHEN** 某模型 id 只出现在 `opencode-go` 段
- **THEN** 该模型进入解析结果，字段取值来自该条目

### Requirement: 精确 id 回退

当模型 id 不在 `opencode-go` 段时，系统 MUST 从其他 provider 段中查找 id 完全相同的条目并补入解析结果；MUST NOT 使用大小写折叠、前后缀、别名或任何模糊匹配。

#### Scenario: 精确匹配补入
- **WHEN** 模型 id `kimi-k3` 不在 `opencode-go` 段，而某个 provider 段存在 id 为 `kimi-k3` 的条目
- **THEN** 该条目被补入解析结果

#### Scenario: 非精确匹配不补入
- **WHEN** 候选 provider 段只有 `Kimi-K3`、`kimi-k3-mini` 或 `kimi-k3@latest` 这类 id
- **THEN** 系统不把其中任何一条当作 `kimi-k3` 的匹配

### Requirement: 字段级补齐

对已存在于 `opencode-go` 段的模型，若某字段为空值（空字符串、nil、空数组或空对象），系统 MUST 使用其他 provider 段中同 id 条目的对应字段补齐；MUST NOT 覆盖 `opencode-go` 已提供的非空值。

#### Scenario: 缺失字段被补齐
- **WHEN** `opencode-go` 条目缺少 description，而同 id 的其他 provider 条目提供了 description
- **THEN** 解析结果的 description 取该回退值

#### Scenario: 已有字段不被覆盖
- **WHEN** `opencode-go` 条目已提供 cost，而同 id 的其他 provider 条目 cost 不同
- **THEN** 解析结果保留 `opencode-go` 的 cost

### Requirement: 单一条目与确定性顺序

解析结果 MUST 对每个模型 id 至多保留一个条目；当多个 provider 段都能提供同 id 条目时，系统 MUST 按稳定的 provider 段顺序选择补齐来源，使同一份 api.json 的多次解析得到相同结果。

#### Scenario: 多来源同 id
- **WHEN** 三个 provider 段都包含 id 为 `glm-5.3` 的条目，且 `opencode-go` 段没有该 id
- **THEN** 解析结果只出现一个 `glm-5.3`，且重复解析选择同一来源

#### Scenario: 输出顺序稳定
- **WHEN** 对同一份 api.json 连续解析两次
- **THEN** 两次得到的模型 id 顺序完全一致

### Requirement: 同步入口共用解析结果

模型元数据同步与价格同步 MUST 使用同一套模型集合与字段解析结果。价格同步 MAY 额外剔除价格缺失或非法的模型，但 MUST NOT 引入解析结果之外的模型。

#### Scenario: 价格集合是模型集合的子集
- **WHEN** 对同一份 api.json 分别执行同步模型预览与价格同步预览
- **THEN** 价格预览列出的模型 id 集合是同步模型预览集合的子集

### Requirement: 免费模型合并规则保持

系统 MUST 继续把 `opencode` 段中 cost 全为零且状态非 deprecated 的 `-free` 模型纳入解析结果；同 id 时以免费条目为准，并用原条目的非空字段补齐免费条目的缺失字段。

#### Scenario: 免费条目覆盖同 id 付费条目
- **WHEN** `opencode` 段存在 cost 全零的 `glm-5.3-flash-free`，且 `opencode-go` 段存在同 id 条目
- **THEN** 解析结果采用免费条目的 cost，并保留原条目的非空元数据

### Requirement: 价格换算安全校验

价格换算 MUST 维持既有校验：input 为 NaN、±Inf、负值或超过上界时跳过该模型；input 缺失时 MUST 跳过而不是按 0 计费；output 或 cache_read 非法时只跳过对应比值。经回退得到的价格 MUST 同样经过这些校验。

#### Scenario: 回退价格非法
- **WHEN** 某模型仅通过回退来源获得价格，且其 input 为负值
- **THEN** 该模型不进入价格换算结果，其余模型不受影响

#### Scenario: 价格缺失不按免费处理
- **WHEN** 某模型条目没有任何 input 价格
- **THEN** 该模型不进入价格换算结果，且不会被写成 model_ratio = 0

#### Scenario: 仅比值非法
- **WHEN** 某模型 input 合法但 output 为 NaN
- **THEN** 该模型仍生成 model_ratio，但不生成 completion_ratio

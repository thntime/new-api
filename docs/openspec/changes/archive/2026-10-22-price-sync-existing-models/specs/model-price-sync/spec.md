# Spec Delta

## Purpose

定义价格同步的候选范围：只有本地模型库中已存在的模型才进入同步结果，避免上游目录中的海量模型淹没本地真实使用的模型。

## ADDED Requirements

### Requirement: 只同步模型库中已有的模型

价格同步结果 MUST 只包含本地模型库（模型元数据表的模型名）中存在的模型；上游存在但模型库中不存在的模型 MUST NOT 出现在差异集合或价格集合中。

#### Scenario: 上游独有的模型被排除
- **WHEN** 上游返回 `kimi-k3` 的价格，而本地模型库中没有 `kimi-k3`
- **THEN** 该模型不出现在 `differences` 与 `prices` 中

#### Scenario: 库中已有模型正常比较
- **WHEN** 本地模型库与上游都包含 `gpt-5.6-luna`
- **THEN** 该模型按现有规则参与差异比较

### Requirement: 过滤对所有价格同步来源生效

模型范围过滤 MUST 应用于全部价格同步来源，包括 opencode-go 预设、models.dev、官方倍率预设与自定义渠道；MUST NOT 只对其中某个来源生效。

#### Scenario: 多来源同时过滤
- **WHEN** 一次请求同时拉取 opencode-go 预设与一个自定义渠道
- **THEN** 两个来源的结果都只保留模型库中已有的模型

### Requirement: 差异与价格集合一致

`prices` 的模型键集合 MUST 与过滤后的 `differences` 一致，不得出现只在其中一侧存在的模型。

#### Scenario: 排除模型不残留在价格集合
- **WHEN** 某上游模型因不在模型库中而被过滤
- **THEN** `prices` 中也不存在该模型的条目

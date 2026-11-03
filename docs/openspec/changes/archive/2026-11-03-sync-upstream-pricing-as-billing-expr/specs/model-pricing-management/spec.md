# Spec Delta

## Purpose

定义管理端模型价格列表的展示范围与批量清理行为，使列表只呈现有实际定价的模型，并允许批量移除选中模型的定价配置。

## ADDED Requirements

### Requirement: 主列表只展示已设置基础定价的模型

模型价格主列表 MUST 只展示已设置基础定价的模型，基础定价包括 `model_ratio`、`model_price`、`tiered_expr` 模式下的非空表达式，或非空的任务插件表达式。只有其他定价字段（如 `completion_ratio`、`cache_ratio`，或仅有 `billing_mode = "ratio"`）而没有基础定价的模型 MUST NOT 出现在主列表；检测到定价问题的模型 MUST 仍然展示，以便管理员处理。

#### Scenario: 仅剩孤立配置字段
- **WHEN** 某模型在 JSON 中删除了 `ModelRatio` 与 `ModelPrice`，但仍残留 `billing_mode` 或其他倍率字段
- **THEN** 该模型不出现在模型价格主列表

#### Scenario: 已设置基础定价
- **WHEN** 某模型设置了 `model_ratio`、`model_price`、保存了非空的 `tiered_expr` 表达式，或配置了非空的任务插件表达式
- **THEN** 该模型出现在模型价格主列表

#### Scenario: 存在定价问题的未定价模型
- **WHEN** 某模型没有基础定价，但存在插件表达式未配置或失效等定价问题
- **THEN** 该模型仍出现在主列表，并显示需要处理的问题

### Requirement: 「未设置价格」标签保持原语义

「未设置价格」标签 MUST 继续展示当前启用、但没有设置基础定价的模型；MUST NOT 展示未启用或上游独有的模型。

#### Scenario: 启用但未设置价格
- **WHEN** 某模型由当前启用的渠道提供，且没有设置基础定价
- **THEN** 该模型出现在「未设置价格」标签

#### Scenario: 已设置价格的模型
- **WHEN** 某模型已设置基础定价
- **THEN** 该模型不出现在「未设置价格」标签

#### Scenario: 未启用的模型
- **WHEN** 某模型只存在于已停用的渠道
- **THEN** 该模型不出现在「未设置价格」标签

### Requirement: 批量清空选中模型的定价

模型价格主列表 MUST 支持对选中模型批量清空全部定价字段；清空 MUST 删除 `ModelRatio`、`ModelPrice`、`CompletionRatio`、`CacheRatio`、`CreateCacheRatio`、`ImageRatio`、`AudioRatio`、`AudioCompletionRatio`、`billing_mode`、`billing_expr` 与插件表达式，等同对每个选中模型执行现有单行删除。清空入口 MUST 位于「Reset prices」旁，并遵循现有草稿与保存流程。

#### Scenario: 批量清空
- **WHEN** 用户选中多个模型并点击「清空」后保存
- **THEN** 这些模型的全部定价字段被删除，并从主列表消失

#### Scenario: 未选择模型
- **WHEN** 没有选中任何模型
- **THEN** 「清空」按钮不可用

#### Scenario: 清空后未保存
- **WHEN** 用户执行清空但没有保存
- **THEN** 改动停留在草稿状态，不写入服务端

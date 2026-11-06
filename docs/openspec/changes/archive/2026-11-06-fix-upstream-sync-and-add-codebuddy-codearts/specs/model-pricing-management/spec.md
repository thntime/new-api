# Spec Delta

## ADDED Requirements

### Requirement: 保存前复核价格变更

保存价格修改前，系统 MUST 展示受影响模型的当前价格与保存后价格，并在用户确认后才提交。复核列表中的模型数量 MUST 与本次待保存的改动数量一致；用户取消复核时 MUST NOT 提交任何改动。

#### Scenario: 保存时展示新旧价格

- **WHEN** 管理员修改了若干模型的价格并触发保存
- **THEN** 系统列出每个受影响模型的当前价格与保存后价格，等待管理员确认

#### Scenario: 取消复核

- **WHEN** 管理员在价格变更复核中取消
- **THEN** 不提交任何改动，改动仍停留在草稿状态

#### Scenario: 无改动时不提交

- **WHEN** 管理员没有修改任何价格却触发保存
- **THEN** 系统不产生价格变更提交

### Requirement: 删除保留为草稿并可撤销

删除某模型的定价 MUST 只作用于草稿：该行 MUST 保留在列表中并提供撤销操作，直到保存成功才真正移除。撤销 MUST 恢复该模型在草稿中的原定价。没有已保存价格的模型 MUST NOT 提供删除操作。

#### Scenario: 删除后尚未保存

- **WHEN** 管理员删除某模型的定价但没有保存
- **THEN** 该行仍显示在列表中，并提供撤销操作

#### Scenario: 撤销删除

- **WHEN** 管理员在保存前撤销删除
- **THEN** 该模型的草稿定价恢复为删除前的值

#### Scenario: 保存后移除

- **WHEN** 管理员删除定价并完成保存
- **THEN** 该模型的定价被删除，并从主列表消失

#### Scenario: 无已保存价格的行

- **WHEN** 某模型没有已保存的价格
- **THEN** 该行不提供删除操作

## MODIFIED Requirements

### Requirement: 批量清空选中模型的定价

模型价格主列表 MUST 支持对选中模型批量清空全部定价字段；清空 MUST 删除 `ModelRatio`、`ModelPrice`、`CompletionRatio`、`CacheRatio`、`CreateCacheRatio`、`ImageRatio`、`AudioRatio`、`AudioCompletionRatio`、`billing_mode`、`billing_expr` 与插件表达式，等同对每个选中模型执行现有单行删除。清空结果 MUST 作为待保存变更，在保存前进入价格变更复核；未保存前 MUST 保持草稿状态。宽屏下清空入口 MUST 位于「Reset prices」旁；窄屏下次级操作可以收进菜单，但仍 MUST 可到达。

#### Scenario: 批量清空

- **WHEN** 用户选中多个模型并点击「清空」后确认保存
- **THEN** 这些模型的全部定价字段被删除，并从主列表消失

#### Scenario: 未选择模型

- **WHEN** 没有选中任何模型
- **THEN** 「清空」按钮不可用

#### Scenario: 清空后未保存

- **WHEN** 用户执行清空但没有保存
- **THEN** 改动停留在草稿状态，不写入服务端

#### Scenario: 清空后在复核中取消

- **WHEN** 用户执行清空并触发保存，但在价格变更复核中取消
- **THEN** 清空结果仍停留在草稿状态，不写入服务端

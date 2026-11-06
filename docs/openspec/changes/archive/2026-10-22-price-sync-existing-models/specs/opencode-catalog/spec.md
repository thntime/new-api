# Spec Delta

## ADDED Requirements

### Requirement: 目录来源优先级

系统解析 models.opencode.ai 目录时，MUST 按 `opencode-go` → `opencode`（官方段）→ 其余 provider 段（段名升序）的顺序选择模型条目与补齐字段。

#### Scenario: opencode 官方段优先于其他 provider
- **WHEN** 某模型 id 同时出现在 `opencode` 段与 `aaa-provider` 段，且不在 `opencode-go` 段
- **THEN** 解析结果采用 `opencode` 段的取值

#### Scenario: 其余 provider 段按段名升序
- **WHEN** 某模型 id 只出现在 `zzz-provider` 与 `mmm-provider` 两段
- **THEN** 解析结果采用 `mmm-provider` 的取值

#### Scenario: opencode-go 仍最优先
- **WHEN** 某模型 id 同时出现在 `opencode-go`、`opencode` 与其他段
- **THEN** 解析结果采用 `opencode-go` 的取值

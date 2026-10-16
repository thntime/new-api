# Spec Delta

## Purpose

定义 CodeBuddy 渠道类型的接入行为：管理员可以将 CodeBuddy 作为上游渠道接入 new-api，按国内/国际记录接入方式，并以 chat completions、responses、messages 三种客户端协议转发请求、从上游获取模型列表。

## ADDED Requirements

### Requirement: 注册 CodeBuddy 渠道类型

系统 SHALL 提供一个 CodeBuddy 渠道类型，其类型标识为 93，显示名称为 `CodeBuddy`，默认 API 地址 SHALL 为空字符串。

#### Scenario: 渠道类型出现在提供方选择器

- **WHEN** 管理员打开渠道提供方选择器
- **THEN** 其中包含 CodeBuddy 选项

#### Scenario: 默认 API 地址为空

- **WHEN** 管理员选择 CodeBuddy 渠道类型且未手动填写 API 地址
- **THEN** API 地址字段为空，系统 MUST NOT 预填任何地址

### Requirement: 接入方式区域标记

CodeBuddy 渠道 SHALL 提供「接入方式」选择，取值为「国内」与「国际」。所选值 MUST 随渠道持久化并在编辑时回显；未选择时 SHALL 为空。该选择 MUST 仅作为区域标记，MUST NOT 推断、预填或改写渠道的 API 地址，也 MUST NOT 改变发往上游的请求内容。

#### Scenario: 未选择接入方式时不改变行为

- **WHEN** 管理员新建 CodeBuddy 渠道且未选择接入方式
- **THEN** 渠道保存成功，接入方式为空，转发行为与已选择区域时一致

#### Scenario: 选择接入方式后回显

- **WHEN** 管理员将接入方式设为「国际」并保存，随后重新打开该渠道
- **THEN** 接入方式仍显示为「国际」

#### Scenario: 切换接入方式不改变 API 地址

- **WHEN** 管理员在已填写 API 地址的 CodeBuddy 渠道上把接入方式从「国内」切换为「国际」并保存
- **THEN** 该渠道的 API 地址保持原值不变，转发请求的目标地址也不受影响

### Requirement: 客户端路由转发

CodeBuddy 渠道 SHALL 支持将客户端发往 `/v1/chat/completions`、`/v1/responses` 与 `/v1/messages` 的请求转发到上游，并返回与所选客户端协议一致的响应。messages 请求 MUST 以 Anthropic 协议原样转发，MUST NOT 被降级为 chat completions 请求。

#### Scenario: 转发 chat completions 请求

- **WHEN** 客户端向 `/v1/chat/completions` 发起请求且命中 CodeBuddy 渠道
- **THEN** 请求被转发到该渠道 API 地址下的 chat completions 端点，并返回兼容的 chat completions 响应

#### Scenario: 转发 responses 请求

- **WHEN** 客户端向 `/v1/responses` 发起请求且命中 CodeBuddy 渠道
- **THEN** 请求被转发到该渠道 API 地址下的 responses 端点，并返回兼容的 responses 响应

#### Scenario: 转发 messages 请求

- **WHEN** 客户端向 `/v1/messages` 发起请求且命中 CodeBuddy 渠道
- **THEN** 请求以 Anthropic messages 协议转发到该渠道 API 地址下的 messages 端点，并返回兼容的 messages 响应

#### Scenario: 未配置 API 地址时返回明确错误

- **WHEN** 命中一个未填写 API 地址的 CodeBuddy 渠道
- **THEN** 系统返回明确的渠道配置错误，MUST NOT 拼出相对或无效的上游地址发起请求

#### Scenario: 流式请求按流式返回

- **WHEN** 客户端以流式方式向 CodeBuddy 渠道发起上述任一协议的请求
- **THEN** 系统按该渠道已登记的流式支持转发并以流式响应返回

### Requirement: 从上游获取模型列表

系统 SHALL 支持从 CodeBuddy 上游获取模型列表，请求地址为 `{渠道 API 地址}/v1/models`，并按标准 OpenAI 模型列表结构解析其中的模型 ID。获取失败时系统 MUST 返回失败提示，MUST NOT 写入任何模型。

#### Scenario: 成功获取模型列表

- **WHEN** 管理员对已填写 API 地址的 CodeBuddy 渠道执行「获取模型」
- **THEN** 系统请求 `{渠道 API 地址}/v1/models`，并返回响应中的模型 ID 列表

#### Scenario: 未填写 API 地址时给出提示

- **WHEN** 管理员对 API 地址为空的 CodeBuddy 渠道执行「获取模型」
- **THEN** 系统返回失败提示，不写入任何模型

#### Scenario: 上游返回错误时给出提示

- **WHEN** CodeBuddy 上游模型接口返回错误或不可达
- **THEN** 系统向管理员返回失败提示，不写入任何模型

### Requirement: 渠道图标展示

CodeBuddy 渠道类型 SHALL 在渠道列表与渠道类型选择器中展示 CodeBuddy 品牌图标。

#### Scenario: 渠道列表展示 CodeBuddy 图标

- **WHEN** 渠道列表中包含 CodeBuddy 渠道
- **THEN** 该渠道的类型标识处展示 CodeBuddy 品牌图标

#### Scenario: 选择器展示 CodeBuddy 图标

- **WHEN** 管理员在渠道类型选择器中查看 CodeBuddy 选项
- **THEN** 该选项展示 CodeBuddy 品牌图标

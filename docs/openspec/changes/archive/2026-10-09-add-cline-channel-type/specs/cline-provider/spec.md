## Purpose

定义 Cline 渠道类型的接入行为：管理员可以将 Cline 作为上游渠道接入 new-api，并按「计费方式」在按量（Cline）与计划（ClinePass）之间切换，从而按方式获取模型、查询计划用量并转发请求。

## ADDED Requirements

### Requirement: 注册 Cline 渠道类型

系统 SHALL 提供一个 Cline 渠道类型，其类型标识为 94，默认 API 地址为 `https://api.cline.bot/api`。

#### Scenario: 选择 Cline 时预填默认地址

- **WHEN** 管理员在渠道类型中选择 Cline 且未手动修改 API 地址
- **THEN** API 地址字段预填为 `https://api.cline.bot/api`

#### Scenario: 渠道类型出现在提供方选择器

- **WHEN** 管理员打开渠道提供方选择器
- **THEN** 其中包含 Cline 选项

### Requirement: 渠道计费方式选择

Cline 渠道 SHALL 提供「计费方式」选择，取值为 `Cline`（按量）与 `ClinePass`（计划）。所选值 MUST 随渠道持久化并在编辑时回显；未选择时默认 SHALL 为 `Cline`（按量）。ClinePass MUST NOT 改变按 token 扣减用户额度的计费语义。

#### Scenario: 默认按量方式

- **WHEN** 管理员新建 Cline 渠道且未修改计费方式
- **THEN** 该渠道计费方式为 `Cline`（按量）

#### Scenario: 选择计划方式后回显

- **WHEN** 管理员将计费方式改为 `ClinePass` 并保存，随后重新打开该渠道
- **THEN** 计费方式仍显示为 `ClinePass`

#### Scenario: 计划方式仍按 token 扣减额度

- **WHEN** 通过 ClinePass 渠道转发一次请求并成功返回用量
- **THEN** 系统按返回的 token 用量扣减用户额度，与按量方式一致

### Requirement: 客户端路由转发

Cline 渠道 SHALL 支持将客户端发往 `/v1/chat/completions`、`/v1/responses` 与 `/v1/messages` 的请求转发到 Cline 上游，并返回与所选客户端协议一致的响应。

#### Scenario: 转发 chat completions 请求

- **WHEN** 客户端向 `/v1/chat/completions` 发起请求且命中 Cline 渠道
- **THEN** 请求被转发到 Cline 上游并返回兼容的 chat completions 响应

#### Scenario: 转发 responses 请求

- **WHEN** 客户端向 `/v1/responses` 发起请求且命中 Cline 渠道
- **THEN** 请求被转发到 Cline 上游并返回兼容的 responses 响应

#### Scenario: 转发 messages 请求

- **WHEN** 客户端向 `/v1/messages` 发起请求且命中 Cline 渠道
- **THEN** 请求被转发到 Cline 上游并返回兼容的 messages 响应

### Requirement: 按计费方式获取模型

系统 SHALL 支持从 Cline 上游获取模型列表，且获取来源随计费方式变化。按量（`Cline`）方式 SHALL 请求 `{base}/v1/models` 并解析标准模型列表。计划（`ClinePass`）方式 SHALL 请求 `{base}/v1/ai/cline/recommended-models`，并且仅使用响应中 `clinePass` 数组元素的 `id` 作为候选模型名；MUST NOT 纳入 `recommended`、`free`、`clineCloud` 等其它数组。

#### Scenario: 按量方式从 /v1/models 获取

- **WHEN** 管理员对计费方式为按量的 Cline 渠道执行「获取模型」
- **THEN** 系统请求 `{base}/v1/models` 并返回其中的模型列表

#### Scenario: 计划方式仅取 clinePass

- **WHEN** 管理员对计费方式为计划的 Cline 渠道执行「获取模型」
- **THEN** 系统请求 `{base}/v1/ai/cline/recommended-models`，并且返回的候选模型名仅为 `clinePass` 数组元素的 `id`，不包含其它数组中的模型

#### Scenario: 上游获取失败给出提示

- **WHEN** 上游模型接口返回错误或不可达
- **THEN** 系统向管理员返回失败提示，不写入任何模型

### Requirement: 计划用量查询

当渠道计费方式为 `ClinePass` 时，系统 SHALL 提供 Coding Plan 用量查询，请求 `{base}/v1/users/me/plan/usage-limits`，并展示返回的每条 limit 的类型、已用百分比与重置时间。计费方式为按量（`Cline`）时，计划用量查询 SHALL 不适用。

#### Scenario: 计划渠道展示用量

- **WHEN** 管理员对计费方式为计划的 Cline 渠道发起计划用量查询
- **THEN** 系统请求 `{base}/v1/users/me/plan/usage-limits` 并展示各 limit 的类型、已用百分比与重置时间

#### Scenario: 按量渠道不适用计划用量

- **WHEN** 管理员对计费方式为按量的 Cline 渠道发起计划用量查询
- **THEN** 系统不返回计划用量，并给出该渠道不支持计划用量的提示

#### Scenario: 上游用量接口失败给出提示

- **WHEN** 计划用量接口返回错误或不可达
- **THEN** 系统向管理员返回失败提示，不展示伪造数据

### Requirement: 默认请求体参数覆盖

新建 Cline 渠道时，系统 SHALL 预填默认的请求体参数覆盖，内容为 `{"providerOptions":{"gateway":{"only":["deepseek"]}},"provider":{"only":["deepseek"]}}`。该覆盖 SHALL 可被管理员编辑或清空，并且在转发时 SHALL 注入到发往 Cline 上游的请求体。

#### Scenario: 新建渠道时预填默认覆盖

- **WHEN** 管理员新建一个 Cline 渠道
- **THEN** 请求体参数覆盖字段已预填 `providerOptions` 与 `provider` 两组 provider 限定内容

#### Scenario: 管理员可编辑默认覆盖

- **WHEN** 管理员修改或清空预填的请求体参数覆盖并保存
- **THEN** 保存后的渠道使用修改后的覆盖值，不再使用默认值

#### Scenario: 转发时注入覆盖

- **WHEN** 通过 Cline 渠道转发请求且该渠道配置了请求体参数覆盖
- **THEN** 发往上游的请求体包含该覆盖内容

### Requirement: 渠道图标展示

Cline 渠道类型 SHALL 在渠道列表与渠道类型选择器中展示 Cline 品牌图标。

#### Scenario: 渠道列表展示 Cline 图标

- **WHEN** 渠道列表中包含 Cline 渠道
- **THEN** 该渠道的类型标识处展示 Cline 品牌图标

#### Scenario: 选择器展示 Cline 图标

- **WHEN** 管理员在渠道类型选择器中查看 Cline 选项
- **THEN** 该选项展示 Cline 品牌图标

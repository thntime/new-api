# Spec Delta

## Purpose

让运维可以用可配置规则把特定的上游错误响应重写成下游客户端能识别的结果（例如把上游限额错误改写成 `429` 且带 `x-should-retry: true`），从而使支持重试的客户端能自动退避重试，而不是把请求判为终态失败。

## ADDED Requirements

### Requirement: 状态映射规则配置

系统 MUST 提供一组全局的「状态映射规则」，仅超级管理员可修改。每条规则由匹配条件（上游状态码、报文正则）与输出（状态码、响应报文、附加响应头）组成，规则列表按顺序求值，先命中者生效。规则默认不启用。

#### Scenario: 未启用规则时保持原样

- **WHEN** 所有状态映射规则都处于关闭状态，且上游返回任意错误
- **THEN** 客户端收到的状态码、响应头与响应体与未引入该功能时完全一致

#### Scenario: 启用一条规则后对其生效

- **WHEN** 管理员启用一条匹配「上游状态码 500 且报文含 `exhausted`」并输出 `429` 的规则，上游随后返回状态码 500 且报文含 `exhausted`
- **THEN** 客户端收到的状态码为 `429`，响应体为规则配置的内容

#### Scenario: 规则顺序决定优先级

- **WHEN** 两条规则都能匹配同一次上游响应
- **THEN** 只有顺序在前的规则生效，后一条规则不再参与匹配

### Requirement: 规则匹配语义

规则匹配 MUST 同时比较上游状态码与错误报文的文本。状态码为空时表示匹配任意状态码；报文条件为正则表达式，匹配时 MUST 不区分大小写，且采用「包含匹配」语义（错误文本任意位置命中即可）。用于匹配的文本 MUST 是上游返回的原始错误文本，而非脱敏后的输出。

#### Scenario: 状态码与报文同时命中

- **WHEN** 规则匹配条件为状态码 `200`、报文正则 `tpm/rpm|exhausted|rps|rate limit`，上游以状态码 `200` 返回报文 `inference exceeds tpm/rpm limit`
- **THEN** 该规则命中

#### Scenario: 仅报文命中而状态码不符

- **WHEN** 规则匹配条件为状态码 `500`、报文正则 `exhausted`，上游以状态码 `429` 返回报文 `rpm exhausted`
- **THEN** 该规则不命中，响应保持原样

#### Scenario: 大小写不敏感

- **WHEN** 规则报文正则为 `tpm exhausted`，上游报文为 `TPM Exhausted`
- **THEN** 该规则命中

### Requirement: 命中规则的响应改写

规则命中后，系统 MUST 用规则配置的状态码、响应头与响应体替换原响应：状态码为配置值；响应头包含配置的附加头（例如 `x-should-retry: true`、`Retry-After: 30`）且 `Content-Type: application/json`；响应体为按客户端协议选择的结构化错误对象。响应体 MUST NOT 包含会被下游判为「账号额度上限」而放弃重试的文案与错误码。

#### Scenario: 改写为可重试的限流响应

- **WHEN** 一条启用的规则命中上游错误，其输出配置为状态码 `429`、附加头 `x-should-retry: true` 与 `Retry-After: 30`、报文为限额提示
- **THEN** 客户端收到 `HTTP 429`，响应头含 `x-should-retry: true`、`Retry-After: 30`、`Content-Type: application/json`
- **AND** 响应体为协议对应的错误对象，且不含 `insufficient_quota`、`quota exceeded`、`usage limit` 等额度类文案或错误码

#### Scenario: 按客户端协议选择响应体结构

- **WHEN** 请求来自 Claude 协议客户端且规则命中
- **THEN** 响应体为 Claude 错误结构（`{"type":"error","error":{"type":"rate_limit_error","message":"..."}}`）
- **WHEN** 请求来自 OpenAI 协议或其他协议的客户端且规则命中
- **THEN** 响应体为 OpenAI 兼容结构（`{"error":{"message":"...","type":"rate_limit_error","code":"rate_limit_exceeded"}}`）

### Requirement: 上游以成功状态码返回错误体

当上游以 `200` 返回的响应体实际是错误对象时，系统 MUST 仍能对其应用状态映射规则，使该次请求不会以「成功」交付给客户端。

#### Scenario: 上游 200 携带错误体被改写

- **WHEN** 上游返回状态码 `200`，响应体为错误对象且错误文本命中一条启用的规则
- **THEN** 客户端收到的状态码为规则输出的状态码，而非 `200`

#### Scenario: 上游 200 且响应体为正常内容

- **WHEN** 上游返回状态码 `200` 且响应体为正常业务内容
- **THEN** 响应保持原样，不应用任何状态映射规则

### Requirement: 已提交响应的流式边界

响应头一旦提交（SSE 已开始输出），系统 MUST NOT 修改该响应的状态码，也 MUST NOT 追加可重试类响应头。流式请求的错误处理保持改写前的行为。

#### Scenario: 流已开始后发生错误

- **WHEN** SSE 已经开始输出，随后上游在流中返回命中规则限额的错误
- **THEN** 该 HTTP 响应的状态码仍为已提交的 `200`，且不追加 `x-should-retry` 响应头

#### Scenario: 流式请求在首个响应之前失败

- **WHEN** 流式请求在响应头提交之前就收到命中规则的上游错误
- **THEN** 规则照常生效，客户端收到规则配置的状态码与响应头

### Requirement: 限额规则预置内容

系统 MUST 预置覆盖上游限额措辞的规则（含 `exhausted`、`tpm/rpm`、`rps`、`rate limit`），其输出为状态码 `429`、响应头 `x-should-retry: true` 与 `Retry-After: 30`、以及协议对应的限额错误报文。预置规则 MUST NOT 默认启用。

#### Scenario: 预置限额规则默认关闭

- **WHEN** 系统首次部署，管理员未修改任何规则
- **THEN** 规则表中可见预置限额规则，但其状态为关闭，上游限额错误仍按原样返回

#### Scenario: 启用后覆盖四种实际措辞

- **WHEN** 管理员启用预置限额规则，上游分别返回报错文本 `inference exceeds tpm/rpm limit`、`tpm exhausted`、`rpm exhausted`、`rps exhausted`
- **THEN** 四次响应均被改写为 `429` 且携带 `x-should-retry: true`

### Requirement: 规则配置校验

保存规则时系统 MUST 校验其内容，拒绝无法解析的正则表达式；非法配置 MUST NOT 落库，且 MUST NOT 影响已生效的规则。

#### Scenario: 保存非法正则被拒绝

- **WHEN** 管理员提交一条报文正则为未闭合括号的规则
- **THEN** 保存失败并返回可读的错误说明，规则列表保持修改前的状态

#### Scenario: 保存后立即生效

- **WHEN** 管理员启用一条合法规则并保存成功
- **THEN** 后续请求无需重启服务即可按新规则改写

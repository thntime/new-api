# Proposal

## Why

管理员目前只能在「模型广场 → 模型详情 → 性能」中看到单个模型固定 24 小时的 TPS、延迟与成功率，无法按时间范围、渠道、分组、令牌等维度查看整体服务健康，也无法定位是哪个渠道或分组在拖慢请求。日志库里已有请求级数据（模型、渠道、分组、令牌、token 数、耗时、成功/错误类型），但缺少面向运维的聚合视图。

## What Changes

- 在侧边栏「使用日志」「审计日志」之后新增「性能健康」菜单，仅管理员可见，路由为 `/usage-logs/performance`。
- 新增管理员只读聚合 API，基于日志库（`LOG_DB`，含 ClickHouse 配置）按筛选条件计算性能指标，同时兼容 SQLite / MySQL / PostgreSQL / ClickHouse。
- 页面提供筛选区：时间范围（快捷 + 自定义）、时间粒度、模型、渠道、分组、令牌（API Key）、用户、是否流式。
- 页面提供展示区：
  - 概览指标卡：请求数、成功率/可用率、错误数、平均 TPS、最大/最小 TPS、平均延迟、P95 延迟、输入/输出 Token 总量。
  - 趋势图：TPS 趋势（MAX / MIN / AVG）、延迟趋势（平均 / P95）、可用率趋势（成功率 + 失败数）、请求量与 Token 趋势。
  - 维度分解表：分组性能、模型性能、渠道性能（请求数、成功率、平均延迟、平均/最大/最小 TPS、Token）。
  - 异常定位：最慢请求 Top N、错误日志 Top N。
- 指标口径：TPS = `completion_tokens / use_time`；延迟 = `use_time`；成功率 = 消费日志数 /（消费日志数 + 错误日志数）。
- 新增菜单项、路由、i18n key，并在访问令牌路由声明中登记新端点。

## Capabilities

### New Capabilities

- `performance-health`: 管理员侧的请求性能与可用性聚合视图，覆盖筛选条件、指标口径、趋势与维度分解展示、权限与多数据库兼容。

### Modified Capabilities

（无。现有 `usage-logs` 规格描述的是日志列表本身的展示，本次不改变其行为。）

## Impact

- 后端：`router/api-router.go`（新增只读路由）、`controller/`（新增聚合入口）、`model/log.go`（新增日志聚合查询）、`middleware/access_token_routes.go`（登记新路由）。
- 前端：`web/src/hooks/use-sidebar-data.ts`、`web/src/hooks/use-sidebar-config.ts`、`web/src/routes/_authenticated/usage-logs/`、新增 `web/src/features/performance-health/`、7 个 i18n locale 文件。
- 数据：不新增日志字段、不修改日志写入链路、不改动 `perf_metrics` 采样。
- 依赖与限制：成功率依赖错误日志开关（`constant.ErrorLogEnabled`）与错误日志记录策略；延迟为秒级 `use_time`，短请求（<1s）精度有限；聚合查询需设置时间范围与时间点上限以控制日志库压力。

## Non-Goals

- 不改造 `perf_metrics` 采样表，不把 token / 渠道维度写入采样表。
- 不提供普通用户视角（本次仅管理员看全量）。
- 不做实时推送或 WebSocket 刷新。
- 不修改「模型广场」的模型详情性能 Tab。
- 不新增日志表字段或迁移。

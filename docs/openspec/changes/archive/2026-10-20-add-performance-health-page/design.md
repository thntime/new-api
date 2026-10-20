---
comet_change: add-performance-health-page
role: technical-design
canonical_spec: openspec
archived-with: 2026-10-20-add-performance-health-page
status: final
---

# Design

## Context

现有性能数据有两条互不相通的数据源：

- `perf_metrics`（主库）：毫秒级 `total_latency_ms`、`ttft_sum_ms`、`output_tokens`、`generation_ms`、`success_count`，但维度只有 `model_name` + `group` + 时间桶，没有渠道与令牌。
- `logs`（日志库，可配置 ClickHouse）：有 `model_name`、`channel_id`、`group`、`token_name`、`prompt_tokens`、`completion_tokens`、`use_time`（整秒）、`type`（2 消费 / 5 错误），但没有毫秒级总耗时，也没有成功标记。

需求要求按时间、模型、渠道、分组、令牌筛选，因此只能以 `logs` 为唯一数据源。已有日志查询的方言适配集中在 `model/main.go`（`logGroupCol`）、`model/log.go`（ClickHouse 排序与 LIKE 转义），聚合查询可直接复用。

前端已有可复用资产：`features/pricing/components/model-details-charts.tsx` 的 VChart 折线/可用率图、`features/performance-metrics/lib/format.ts` 的 TPS/延迟/百分比格式化、`features/dashboard/components/ui/panel-wrapper.tsx`、`components/data-table` 的 `StaticDataTable`、`components/datetime-picker.tsx`。

## Goals / Non-Goals

**Goals:**

- 单一数据源、单一筛选快照，所有区块口径一致。
- 聚合查询在 SQLite / MySQL / PostgreSQL / ClickHouse 下语义一致，不使用专有函数。
- 查询成本可控：时间跨度与数据点数量都有上限。
- 页面区块尽量复用已有组件与图表主题。

**Non-Goals:**

- 不引入毫秒级延迟或 TTFT（日志没有该字段，解析 `other` 的 JSON 在四种数据库下不可移植）。
- 不做实时推送、不做导出、不做普通用户视角。
- 不修改日志写入链路与 `perf_metrics` 采样。

## Decisions

### D1. 以日志库为唯一数据源

按请求日志聚合，而不是 `perf_metrics`。理由：只有日志同时具备渠道、令牌、分组、模型、时间五个维度。代价是延迟为整秒、TPS 为粗算，已在 specs 的口径要求中明确。备选方案（`perf_metrics` 或混合口径）被否决：前者无法满足筛选需求，后者会让同一页面在切换筛选时改变数据来源，用户无法解释数值差异。

### D2. 单个聚合端点返回整页数据

`GET /api/log/performance` 一次返回 `summary`、`series`、`groups`、`models`、`channels`、`slow_requests`、`errors`。理由：一次查询对应一个筛选快照，避免多个端点返回不同时刻的数据；前端一次请求、一个 loading 状态。备选的分端点方案会增加往返并引入快照不一致。

### D3. 请求分类与成功率

`type = 2`（消费）计为成功，`type = 5`（错误）计为失败，其余类型不参与。成功率 = 成功 /（成功 + 失败）。这是“尝试级可用率”：一次请求在渠道 A 失败后重试到渠道 B 成功，会记为 1 次失败 + 1 次成功，能正确反映渠道健康度。该口径必须在页面说明中标注。

### D4. TPS 与延迟口径

- 单请求 TPS = `completion_tokens / use_time`，仅当 `use_time > 0` 且 `completion_tokens > 0` 时参与统计。
- 平均 TPS 用请求级 TPS 的算术平均（而非总 token / 总耗时），这样最大值、最小值与平均值同源。
- 延迟 = `use_time`，返回时换算为毫秒（`use_time * 1000`）以复用现有格式化工具。
- P95 精确计算：按 `(bucket, use_time)` 分组取计数，在 Go 中按耗时升序累加计数定位第 95 百分位。这样避免各数据库分位数函数差异（MySQL 5.7 无 `PERCENTILE_CONT`）。为限制直方图行数，超过 600 秒的耗时按 600 秒归并（用 `CASE WHEN`，四种数据库都支持）。

### D5. 时间分桶使用取模表达式

分桶表达式统一为 `created_at - (created_at % N)`，N ∈ {60, 300, 900, 3600, 86400}。四种数据库都支持 `%` 且都返回整数语义；MySQL 的 `/` 返回小数，因此不使用除法。天级分桶按 UTC 对齐，前端按用户时区展示标签。

### D6. 查询上限

- 最大时间跨度 30 天，超出返回参数错误。
- 数据点上限 120；前端提供 `auto` 粒度，后端按跨度自动选择最小可用粒度，并在响应中回传实际粒度。
- 聚合只扫描 `type IN (2,5)` 且带 `created_at` 范围条件，走 `idx_created_at_id`。

### D7. 保留字与维度命名

分组列使用 `logGroupCol`（`model/main.go`）；渠道返回 `channel_id`，名称通过现有渠道缓存回填（复用 `GetAllLogs` 的回填逻辑）；维度分解表默认按请求数降序。

### D8. 前端页面结构

```
+--------------------------------------------------------------------------+
| 性能健康                                                [刷新] [自动刷新] |
+--------------------------------------------------------------------------+
| 时间范围 | 粒度 | 模型 | 渠道 | 分组 | 令牌 | 用户 | 流式 | 查询 | 重置 |
+--------------------------------------------------------------------------+
| 请求数 | 成功率 | 错误数 | 平均TPS | 最大TPS | 最小TPS | 平均延迟 | P95 |
| 输入Token | 输出Token | 总Token                                      |
+--------------------------------------------------------------------------+
| TPS 趋势 (MAX/MIN/AVG)                 | 可用率趋势 (成功率 + 失败数)    |
+----------------------------------------+---------------------------------+
| 延迟趋势 (平均/P95)                    | 请求量与 Token 趋势             |
+----------------------------------------+---------------------------------+
| 分组性能表                                                               |
+--------------------------------------------------------------------------+
| 模型性能表                                                               |
+--------------------------------------------------------------------------+
| 渠道性能表                                                               |
+--------------------------------------------------------------------------+
| 最慢请求 Top N                         | 错误日志 Top N                  |
+--------------------------------------------------------------------------+
```

- 路由 `/usage-logs/performance`，静态路由优先于现有 `/usage-logs/$section` 动态路由。
- 菜单加入 `use-sidebar-data.ts` 的 general 组、`Audit Logs` 之后，带 `requiredRole: ROLE.ADMIN`；`URL_TO_CONFIG_MAP` 映射到现有 `{console, log}` 开关，不新增后端侧边栏模块字段。
- 页面放在 `web/src/features/performance-health/`：`api.ts`、`types.ts`、`components/filter-bar.tsx`、`components/kpi-grid.tsx`、`components/trend-charts.tsx`、`components/breakdown-tables.tsx`、`components/anomaly-lists.tsx`。
- 图表复用 `@visactor/react-vchart` + `useChartTheme()`；指标卡与表格复用 `PanelWrapper`、`StaticDataTable`、`IconBadge`。
- 筛选状态放在 URL search params（与 usage-logs 一致），便于分享与刷新保持。

### D9. 后端结构

- `controller/log_performance.go`：解析筛选、校验上限、调用聚合、回填渠道名。
- `model/log_performance.go`：四条聚合查询（概览、时间桶、维度、异常列表）与请求级 TPS/P95 计算。
- 路由：`router/api-router.go` 的 `logRoute` 组内 `GET /performance` + `middleware.AdminAuth()`。
- `middleware/access_token_routes.go` 登记 `"GET /api/log/performance": accessTokenScopeRule("log:read")`。

### D10. 响应结构

```json
{
  "granularity": "hour",
  "error_log_enabled": true,
  "summary": {
    "requests": 0, "successes": 0, "failures": 0, "success_rate": null,
    "avg_tps": null, "max_tps": null, "min_tps": null,
    "avg_latency_ms": null, "p95_latency_ms": null,
    "prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0
  },
  "series": [{ "ts": 0, "...": "与 summary 同名字段" }],
  "groups": [{ "group": "", "...": "同名字段" }],
  "models": [{ "model_name": "", "...": "同名字段" }],
  "channels": [{ "channel_id": 0, "channel_name": "", "...": "同名字段" }],
  "slow_requests": [{ "id": 0, "created_at": 0, "model_name": "", "channel_id": 0, "channel_name": "", "token_name": "", "use_time": 0, "completion_tokens": 0 }],
  "errors": [{ "id": 0, "created_at": 0, "model_name": "", "channel_id": 0, "channel_name": "", "token_name": "", "content": "" }]
}
```

无法计算的指标统一返回 `null`，由前端渲染空占位，避免 0 与“无数据”混淆。

### D11. 令牌筛选按名称匹配

日志已有 `token_name` 与 `token_id`，但 `LogFilter` 只支持 `token_name`。本次复用该过滤条件，按令牌名称精确匹配，不新增管理员级令牌列表接口。备选方案（新增 `token_id` 过滤 + 令牌下拉）会引入新的管理员接口与额外的跨表查询，超出本次范围。

### D12. 聚合查询骨架

```sql
SELECT created_at - (created_at % :n) AS bucket,
       COUNT(*) AS requests,
       SUM(CASE WHEN type = 2 THEN 1 ELSE 0 END) AS successes,
       SUM(CASE WHEN type = 5 THEN 1 ELSE 0 END) AS failures,
       AVG(CASE WHEN use_time > 0 THEN use_time END) AS avg_use_time,
       AVG(CASE WHEN use_time > 0 AND completion_tokens > 0
                THEN 1.0 * completion_tokens / use_time END) AS avg_tps,
       MAX(CASE WHEN use_time > 0 AND completion_tokens > 0
                THEN 1.0 * completion_tokens / use_time END) AS max_tps,
       MIN(CASE WHEN use_time > 0 AND completion_tokens > 0
                THEN 1.0 * completion_tokens / use_time END) AS min_tps,
       COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens,
       COALESCE(SUM(completion_tokens), 0) AS completion_tokens
FROM logs
WHERE type IN (2, 5) AND created_at >= :start AND created_at < :end
  -- 其余筛选条件按需追加；分组列使用 logGroupCol
GROUP BY bucket
```

维度表复用同一表达式，只把 `GROUP BY` 换成 `group` / `model_name` / `channel_id` 并 `LIMIT 50`；慢请求与错误列表各自独立查询。P95 直方图查询额外返回 `CASE WHEN use_time > 600 THEN 600 ELSE use_time END AS t, COUNT(*)`，按 `bucket, t` 分组。


## Risks / Trade-offs

- [延迟精度只有整秒] → 页面标注“延迟按秒级日志计算”，并用 P95 与平均值共同呈现；不伪造毫秒精度。
- [成功率依赖错误日志开关] → 当错误日志未开启时，成功率会偏高；接口在响应中回传 `error_log_enabled`，前端显示提示。
- [大范围聚合可能压日志库] → 30 天跨度 + 500 数据点上限 + 仅扫描 `type IN (2,5)`；如仍慢，后续可加日志库侧汇总表。
- [P95 依赖 `(bucket, use_time)` 分组行数] → 行数上限为 桶数 × 不同耗时值，受 500 桶上限约束；异常超长耗时不会显著增加行数。
- [重试导致尝试级成功率低于请求级] → 页面明确标注口径，避免被误解为“用户请求失败率”。
- [天级分桶按 UTC] → 前端标签按本地时区渲染，跨时区用户看到的分桶边界一致，不会出现数据重复或缺失。

## Migration Plan

- 无数据库迁移：不新增表、不新增字段、不改日志写入。
- 部署顺序：后端先行（新增只读端点），前端随后；旧版本前端不受影响。
- 回滚：移除路由与菜单即可，无数据副作用。

## Open Questions

（无）

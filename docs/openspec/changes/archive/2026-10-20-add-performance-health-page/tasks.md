# Tasks

## 1. 后端日志聚合层

- [x] 1.1 在 `model/log_performance.go` 定义筛选参数、响应结构与聚合查询（概览、时间桶、分组/模型/渠道维度、最慢请求、错误列表），分桶使用 `created_at - (created_at % N)`，分组列使用 `logGroupCol`；用 `model/log_performance_test.go` 的 SQLite 表驱动测试验证四类指标与筛选条件 <!-- comet-task:e3bc166d-2db3-4e33-922c-757e5a404918 -->
- [x] 1.2 实现请求级 TPS（`completion_tokens / use_time`，过滤 `use_time <= 0` 与 `completion_tokens <= 0`）与平均/最大/最小聚合；用测试验证 0 耗时被排除、最大最小值正确 <!-- comet-task:17f44579-443c-400d-8c5f-c5b74f3b0156 -->
- [x] 1.3 实现 P95：按 `(bucket, use_time)` 计数在 Go 中累加定位第 95 百分位；用测试验证 100 条样本的 P95 取值与单桶/多桶两种场景 <!-- comet-task:def0a307-2d8b-4216-8117-9e75b6388014 -->
- [x] 1.4 实现成功率口径（`type=2` 成功、`type=5` 失败、分母为零返回空值）；用测试验证 90/10 场景与零请求场景 <!-- comet-task:0cb63fd0-8c3a-4739-9792-9d5ca924c39b -->
- [x] 1.5 实现时间跨度与数据点上限校验、`auto` 粒度选择；用测试验证 30 天上限拒绝与自动粒度回传 <!-- comet-task:869f61d7-7082-4d49-b51a-c843332310c7 -->

## 2. 后端接口与权限

- [x] 2.1 新增 `controller/log_performance.go` 解析筛选参数、调用聚合、回填渠道名与 `error_log_enabled`；用测试验证参数解析与响应字段完整 <!-- comet-task:b37d4d1d-8ba1-472d-a5a7-0ca9c23a31ae -->
- [x] 2.2 在 `router/api-router.go` 的 `logRoute` 组注册 `GET /performance` 并挂 `AdminAuth()`；运行 `go build ./...` 验证路由注册 <!-- comet-task:185ef6f0-d4ec-40a9-a144-37825bde2989 -->
- [x] 2.3 在 `middleware/access_token_routes.go` 登记 `GET /api/log/performance` 的 `log:read` 作用域；运行 `go test ./router/ -run TestAccessTokenRouteRulesCoverEveryDashboardRoute` 验证覆盖检查通过 <!-- comet-task:bf08e5d4-e267-4ef5-9fa5-d75f67753e33 -->
- [x] 2.4 验证普通用户调用返回 403、管理员调用返回数据；用 controller 层测试覆盖两种身份 <!-- comet-task:10ae81e3-a4c0-494c-b87b-82d28e204667 -->

## 3. 前端数据层、路由与菜单

- [x] 3.1 新增 `web/src/features/performance-health/types.ts` 与 `api.ts`，封装聚合请求与筛选参数序列化；用类型检查 `bun run tsc --noEmit`（或项目等价命令）验证 <!-- comet-task:c81bc2a4-a025-4bdc-a92f-d85f12d86b10 -->
- [x] 3.2 新增 `/usage-logs/performance` 路由文件，筛选状态写入 URL search params；启动构建验证路由生成且不被 `$section` 动态路由吞掉 <!-- comet-task:a94d9113-476c-4ebc-b0c9-d756ed1d4f27 -->
- [x] 3.3 在 `use-sidebar-data.ts` 的 general 组「Audit Logs」之后加入「性能健康」，带 `requiredRole: ROLE.ADMIN`，并在 `use-sidebar-config.ts` 的 URL 映射中归入现有日志模块开关；验证管理员可见、普通用户不可见 <!-- comet-task:cda9dd39-2e35-4391-b58a-89b0955c68bc -->
- [x] 3.4 为 7 个 locale 文件补齐新增英文 key 的中文/繁中/法/俄/日/越翻译；运行 `bun run i18n:sync` 验证无缺失 key 报告 <!-- comet-task:042e6a1d-83ba-418f-8597-3a00a7c9c1c3 -->

## 4. 前端展示区块

- [x] 4.1 实现筛选栏（时间范围快捷项 + 自定义起止、粒度、模型、渠道、分组、令牌、用户、是否流式、查询/重置）；验证修改任一条件后 URL 与请求参数同步 <!-- comet-task:e1c4c40d-289a-4bb4-9119-baa1ffdbb078 -->
- [x] 4.2 实现概览指标卡（请求数、成功率、错误数、平均/最大/最小 TPS、平均/P95 延迟、输入/输出/总 Token），无数据时显示空占位；验证有数据与空数据两种渲染 <!-- comet-task:837e94e6-4d85-4e48-8749-c16dc4f20cf4 -->
- [x] 4.3 实现四张趋势图（TPS MAX/MIN/AVG、延迟平均/P95、可用率+失败数、请求量与 Token），复用 VChart 主题；验证图表随筛选刷新且粒度标签正确 <!-- comet-task:7b78da3b-bf2f-478f-a036-cb6a2aace4e5 -->
- [x] 4.4 实现分组、模型、渠道三张分解表，默认按请求数降序、支持切换排序；验证列与数据口径和概览一致 <!-- comet-task:20422a2a-aa59-4bd9-a644-cf6ccd50e64e -->
- [x] 4.5 实现最慢请求 Top N 与错误日志 Top N 列表，并提供跳转使用日志详情入口；验证空列表与跳转链接正确 <!-- comet-task:5658a57e-909b-443f-837b-70f22a0e71eb -->
- [x] 4.6 实现整页加载态、单区块错误提示与重试、错误日志未开启提示；验证接口失败时筛选条件保留 <!-- comet-task:352aa0de-dfa7-40a9-9e4b-7d1ffc13d765 -->

## 5. 集成验证

- [x] 5.1 运行后端全量相关测试与 `go build ./...`，记录命令与结果 <!-- comet-task:3207fbb8-2309-4f79-8d4e-44f44cf41b14 -->
- [x] 5.2 在可用的 SQLite、MySQL、PostgreSQL 实例上执行聚合查询验证（含保留字分组列与分桶表达式）；无法提供实例时明确报告阻塞，不声称已验证 <!-- comet-task:369faa3e-746d-4cfb-afbe-11a2b1d9a7cd -->
- [x] 5.3 在 ClickHouse 日志库配置下验证聚合查询无 SQL 错误；无实例时明确报告阻塞 <!-- comet-task:8a195bad-6e35-4c8e-a28e-f783166a0a46 -->
- [x] 5.4 运行前端构建与类型检查，记录命令与结果 <!-- comet-task:f99c0209-4f67-4997-9d6d-676d8f7e4e7a -->

## Workflow follow-up

- 完成 review 配置要求的审查后归档 change。
- 归档后验证归档结果。

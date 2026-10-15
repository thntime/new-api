## 现状与根因

- `router/api-router.go` 的 `logRoute` 组在上一轮升级中新增了 `GET /upstream_payload`（完整路径 `GET /api/log/upstream_payload`，挂 `middleware.AdminAuth()`）。
- `middleware/access_token_routes.go` 的静态规则表 `accessTokenRouteRules` 是 access token 作用域的权威声明表；同一 `/api/log` 段已声明 `GET /api/log/`、`GET /api/log/stat`、`GET /api/log/search`、`GET /api/log/export`、`GET /api/log/channel_affinity_usage_cache` 为 `log:read`，唯独漏了新路由。
- 该表同时被两处消费：
  - `middleware/auth.go` 的 `enforceAccessTokenRoute`：未声明的路由对作用域令牌 fail-closed，返回 403 `ACCESS_TOKEN_ROUTE_UNDECLARED` 并打日志 `access token route is not declared: ...`；
  - `router/access_token_scope_test.go` 的守卫测试：要求每条非豁免的 `/api/`、`/pg/` 面板路由在 `middleware.AccessTokenRouteRuleKeys()` 中恰好出现一次，缺失即失败——这就是 CI 的失败点。

## 修复方案

在 `middleware/access_token_routes.go` 的 `/api/log` 段补上与同类管理员日志路由一致的声明：

```go
"GET /api/log/upstream_payload": accessTokenScopeRule("log:read"),
```

依据：

| 同段已声明路由 | scope |
| --- | --- |
| `GET /api/log/` | `log:read` |
| `GET /api/log/stat` | `log:read` |
| `GET /api/log/search` | `log:read` |
| `GET /api/log/export` | `log:read` |
| `GET /api/log/channel_affinity_usage_cache` | `log:read` |

新路由同属管理员日志面板读取接口，故取 `log:read`。选它而不是 `log:write`/管理员专用 scope：该接口是只读查询，且 `log:read` 已在权限目录中，不引入新 scope。

不放宽鉴权：路由仍经 `AdminAuth()`，非管理员依旧被角色检查拒绝；作用域令牌没有 `log:read` 时仍走 `ACCESS_TOKEN_SCOPE_DENIED`。

## 验证方式

- 回归测试：`go test ./router/ -run TestAccessTokenRouteRulesCoverEveryDashboardRoute -count=1`，修复前失败（RED 证据已记录），修复后通过。
- 相关回归：`go test ./router/... ./middleware/... -count=1` 确认守卫测试与 access token 鉴权测试整体通过。
- 全量构建：按仓库既有方式运行 `go build ./...` 与 `make test` 中受影响的 Go 测试目标，确认 CI 的同一失败点消失。

## Open Questions

无。

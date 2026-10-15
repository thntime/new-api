## Why

升级 CI（run [38546607612](https://github.com/zhangjinglei137/new-api/actions/runs/38546607612) 的 Backend vet, build, and test）在 router 包测试 `TestAccessTokenRouteRulesCoverEveryDashboardRoute` 失败：

```
panel route GET /api/log/upstream_payload must be declared exactly once
```

上一次升级新增上游报文面板时注册了 `GET /api/log/upstream_payload`，但没有在 access token 路由规则表里声明它的 scope，导致「每条面板路由必须恰好声明一次」的守卫测试失败；同时该路由在运行时对作用域令牌一律返回 403（fail-closed 的未声明分支），管理员的作用域令牌即使持有 `log:read` 也无法读取自己有权访问的报文。

## What Changes

- 在 `middleware/access_token_routes.go` 的 `/api/log` 规则段补一条声明：
  `"GET /api/log/upstream_payload": accessTokenScopeRule("log:read")`
- 不改动路由注册、控制器、鉴权流程与任何已有 spec 的验收场景。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

（无。该修复只是补齐既有路由的 scope 声明，`upstream-raw-payload` 的「只能由管理员访问」要求及其验收场景不变，因此不产生 delta spec。）

## Impact

- 代码：`middleware/access_token_routes.go`（1 行）
- 行为：管理员的作用域 access token 持有 `log:read` 时可访问 `GET /api/log/upstream_payload`；无该 scope 的令牌仍返回 403，非管理员仍被 `AdminAuth` 拒绝
- 无 API 形状、数据库、依赖、前端变更

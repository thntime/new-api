# Tasks

## 1. 复现（RED）

- [x] 1.1 运行 `go test ./router/ -run TestAccessTokenRouteRulesCoverEveryDashboardRoute -count=1`，确认失败信息为 `panel route GET /api/log/upstream_payload must be declared exactly once`（与 CI job 115794089690 一致；守卫测试已存在，无需新增用例）

## 2. 修复

- [x] 2.1 在 `middleware/access_token_routes.go` 的 `/api/log` 规则段补 `"GET /api/log/upstream_payload": accessTokenScopeRule("log:read"),`，并运行 `gofmt -w middleware/access_token_routes.go`
- [x] 2.2 重跑步骤 1.1 的命令，确认该用例转绿

## 3. 收尾验证

- [x] 3.1 运行 `go test ./router/... ./middleware/... -count=1`，确认守卫测试与 access token 鉴权相关测试全部通过
- [x] 3.2 运行 `go build ./...` 与 `go vet ./...`，确认编译与静态检查通过

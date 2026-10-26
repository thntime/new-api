# Tasks

## 1. 后端上限参数化

- [x] 1.1 `common/page_info.go` 新增 `GetPageQueryWithMaxSize` 与默认上限常量，`GetPageQuery` 保持默认 100
- [x] 1.2 `controller/channel.go` 的 `GetAllChannels` 使用渠道上限 200

## 2. 回归测试

- [x] 2.1 新增 `common/page_info_test.go`，覆盖默认上限、渠道上限、超限夹取与非法上限回退
- [x] 2.2 运行 `go test ./common/` 与 `go build ./...`

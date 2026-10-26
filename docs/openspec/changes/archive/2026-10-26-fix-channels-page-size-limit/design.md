# Design

## 修复方案

1. `common/page_info.go`
   - 提取 `GetPageQueryWithMaxSize(c *gin.Context, maxPageSize int) *PageInfo`，承载现有解析逻辑与上限夹取。
   - `maxPageSize <= 0` 时回退到默认上限 `MaxPageSize = 100`。
   - `GetPageQuery(c)` 改为调用 `GetPageQueryWithMaxSize(c, MaxPageSize)`，42 个既有调用方行为不变。
2. `controller/channel.go`
   - 新增 `channelsMaxPageSize = 200`，`GetAllChannels` 改用 `common.GetPageQueryWithMaxSize(c, channelsMaxPageSize)`。
3. 测试 `common/page_info_test.go`
   - 默认 `GetPageQuery`：`page_size=200` 夹取为 100。
   - `GetPageQueryWithMaxSize(c, 200)`：`page_size=200` 保持 200。
   - `GetPageQueryWithMaxSize(c, 200)`：`page_size=500` 夹取为 200。
   - `GetPageQueryWithMaxSize(c, 0)`：回退默认上限 100。

## 取舍

- 不修改全局上限：用户、日志、令牌等接口的单页返回量保持不变，避免放大所有列表接口的查询与响应体积。
- 上限作为函数参数而非 PageInfo 字段：调用点显式声明，避免隐式状态。

## 验证

- `go test ./common/` 覆盖上限逻辑。
- `go build ./...` 通过。
- 手工核对 `GetAllChannels` 返回的 `page_size` 可为 200（由测试与构建覆盖）。

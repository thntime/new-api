# Proposal

## 问题

渠道列表页分页大小下拉已包含 `200`，选择后下拉显示 200，但表格实际仍只展示 100 行。

## 根因

`common/page_info.go` 的 `GetPageQuery` 对所有列表接口硬性夹取单页上限：

```go
if pageInfo.PageSize > 100 {
    pageInfo.PageSize = 100
}
```

渠道列表接口 `controller/channel.go` 的 `GetAllChannels` 使用 `common.GetPageQuery(c)` 取分页参数，因此前端传入 `page_size=200` 时被后端改写为 100，返回的 `items` 最多 100 条，响应中的 `page_size` 也是 100。

## 修复目标

- 渠道列表接口允许单页最大 200 条。
- 其他列表接口的单页上限保持 100，行为不变。
- 增加回归测试，覆盖默认上限、渠道上限与超限夹取。

## 范围

- `common/page_info.go`：新增可指定上限的取页函数，`GetPageQuery` 保持默认 100。
- `controller/channel.go`：`GetAllChannels` 使用渠道上限 200。
- 测试：新增 `common/page_info_test.go` 覆盖上限逻辑。

不涉及数据库、鉴权、计费或协议转换。

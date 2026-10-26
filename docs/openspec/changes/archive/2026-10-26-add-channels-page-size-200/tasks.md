# Tasks

## 1. 共享分页组件支持可选每页条数

- [x] 1.1 `pagination.tsx` 增加 `pageSizeOptions` 可选 prop，默认值保持 `[10, 20, 30, 40, 50, 100]`
- [x] 1.2 `data-table-page.tsx` 增加 `pageSizeOptions` 可选 prop 并透传给 `DataTablePagination`

## 2. 渠道列表页增加 200

- [x] 2.1 `channels/constants.ts` 的 `CHANNELS_TABLE_PAGE_SIZE_OPTIONS` 增加 200
- [x] 2.2 `channels-table.tsx` 向 `DataTablePage` 传入渠道分页选项

## 3. 验证

- [x] 3.1 扩展 `pagination.test.tsx`，覆盖自定义选项渲染 200 与默认选项不变
- [x] 3.2 运行涉及测试、`bun check -b` 与涉及文件 lint

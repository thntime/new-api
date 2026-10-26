# Design

## 实现方式

共享分页组件 `DataTablePagination` 的每页条数选项当前是模块常量 `PAGE_SIZE_OPTIONS`，所有调用方共用。为只在渠道页增加 200，采用可选配置透传，而不是直接改共享常量：

1. `pagination.tsx`
   - 保留 `DEFAULT_PAGE_SIZE_OPTIONS = [10, 20, 30, 40, 50, 100]` 作为默认值。
   - `DataTablePaginationProps` 增加可选 `pageSizeOptions?: readonly number[]`。
   - 选项下拉项由传入的 `pageSizeOptions ?? DEFAULT_PAGE_SIZE_OPTIONS` 计算，不再使用模块级 `PAGE_SIZE_SELECT_ITEMS` 常量。
2. `data-table-page.tsx`
   - `DataTablePageProps` 增加可选 `pageSizeOptions?: readonly number[]`，在 `renderPagination` 中透传给 `DataTablePagination`。
3. `channels-table.tsx`
   - 向 `DataTablePage` 传入 `pageSizeOptions={CHANNELS_TABLE_PAGE_SIZE_OPTIONS}`。
4. `constants.ts`
   - `CHANNELS_TABLE_PAGE_SIZE_OPTIONS` 改为 `[10, 20, 50, 100, 200]`（该常量此前未被引用，本次接入）。

## 取舍

- 不直接修改共享 `PAGE_SIZE_OPTIONS`：避免用户、日志、模型等页面同时出现 200，保持既有调用行为不变。
- 渠道页原有选项为 `10 / 20 / 50 / 100`，与共享默认值不完全一致（默认含 30、40），因此渠道页显式传入自己的选项列表，行为与现状一致并新增 200。

## 验证

- 组件测试：`DataTablePagination` 传入自定义 `pageSizeOptions` 时，下拉包含 200；不传时保持默认选项。
- 类型检查与涉及文件 lint。

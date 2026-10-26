# Proposal

## Why

渠道列表页当前每页最多只能选 100 条。渠道数量较多时，用户需要频繁翻页完成批量查看与操作，缺少更大的每页条数选项。

## What Changes

- 渠道列表页的分页大小选项在原有 `10 / 20 / 50 / 100` 基础上增加 `200`。
- 该选项仅作用于渠道列表页，其他使用共享分页组件的页面保持 `10 / 20 / 30 / 40 / 50 / 100` 不变。
- 共享分页组件新增可选的每页条数配置入口，默认值保持现状，不影响既有调用方。

## Capabilities

### New Capabilities

（无。）

### Modified Capabilities

（无。现有 `channels` spec 不包含分页条数要求，本次为 UI 配置调整，无 spec 级行为变化。）

## Impact

- 前端：`web/src/components/data-table/core/pagination.tsx`（新增可选每页条数配置）、`web/src/components/data-table/layout/data-table-page.tsx`（透传配置）、`web/src/features/channels/components/channels-table.tsx`（传入渠道选项）、`web/src/features/channels/constants.ts`（选项增加 200）。
- 测试：`web/src/components/data-table/core/__tests__/pagination.test.tsx` 覆盖自定义选项渲染。
- 不涉及后端、公共 API、数据库或 i18n 文案变更。

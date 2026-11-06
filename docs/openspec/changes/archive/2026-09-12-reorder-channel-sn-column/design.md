# Design: 调整渠道列表列数组顺序

## 修复方案

在 `web/src/features/channels/components/channels-columns.tsx` 的 `useChannelsColumns` 中，将「序号（Sort）」列的定义块（当前位于「优先级（Priority）」列之后）整体前移到「优先级」列之前，使列数组顺序变为：

1. 序号（`sort`）
2. 优先级（`priority`）
3. 权重（`weight`）

列定义块为纯数组字面量顺序，移动不涉及字段、cell 渲染、排序或尺寸配置的任何修改，行为零变更，仅展示顺序变化。

## 影响范围

- 仅 `web/src/features/channels/components/channels-columns.tsx` 一个文件
- 无后端、无 i18n 文案、无 schema 变更
- 表格列顺序属于 tanstack-table 数组顺序，无持久化顺序干扰该两列的相对位置
# Proposal: 渠道列表「序号」列前移至「优先级」列之前

## 问题描述

渠道管理列表当前的列顺序为「优先级（Priority）」→「序号（Sort Number）」→「权重（Weight）」。管理员期望「序号」列排在「优先级」列之前。

## 根因分析

`web/src/features/channels/components/channels-columns.tsx` 中 `useChannelsColumns` 定义列数组时，「优先级」列（`accessorKey: 'priority'`）位于「序号」列（`accessorKey: 'sort'`）之前，列定义顺序即表格展示顺序，因此「序号」出现在「优先级」后面。

## 修复目标

调整 `useChannelsColumns` 列数组顺序，使「序号」列位于「优先级」列之前，最终顺序为「序号」→「优先级」→「权重」。不改动任何列的数量、字段、排序能力或编辑交互，仅调整展示顺序。
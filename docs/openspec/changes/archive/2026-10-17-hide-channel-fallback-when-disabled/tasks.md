# Tasks: hide-channel-fallback-when-disabled

## 1. 读取兜底总开关的 hook

- [x] 1.1 新增 `web/src/features/channels/hooks/use-fallback-enabled.ts`，导出 `useFallbackEnabled()`：以 `useSystemOptions()` 的 `system-options` 数据配合 `getOptionValue(options, { 'channel_affinity_setting.fallback_enabled': true })` 返回布尔值。

## 2. 渠道列表按总开关隐藏兜底列

- [x] 2.1 `web/src/features/channels/components/channels-columns.tsx` 的 `useChannelsColumns` 增加 `showFallback?: boolean`（默认 `true`），仅在为 `true` 时把 `fallback` 列加入列数组。
- [x] 2.2 `web/src/features/channels/components/channels-table.tsx` 调用 `useChannelsColumns` 时传入 `showFallback: useFallbackEnabled()`。

## 3. 渠道对话框按总开关隐藏兜底模型开关

- [x] 3.1 `web/src/features/channels/components/drawers/channel-mutate-drawer.tsx` 用 `useFallbackEnabled()` 条件渲染「路由策略」内的 `channel_info.is_fallback` FormField。

## 4. 回归测试与检查

- [x] 4.1 `web/src/features/channels/components/__tests__/fallback-column.test.tsx` 增加用例：`useChannelsColumns({ showFallback: false })` 时列定义中不含 `fallback`。
- [x] 4.2 `web/src/features/channels/components/__tests__/channel-configuration.test.tsx` 增加用例：兜底总开关关闭时不渲染 `Fallback model` 开关，且提交的渠道负载仍保留原有兜底标记。
- [x] 4.3 运行 `cd web && bun run typecheck` 与本次涉及文件的 vitest 用例，确认通过。

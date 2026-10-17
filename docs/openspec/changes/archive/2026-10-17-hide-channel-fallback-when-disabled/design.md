# 设计：总开关关闭时隐藏兜底标记入口

## 结论

只改前端展示条件，不动 `channel_info.is_fallback` 的存储值、更新接口语义与后端兜底判定。

## 总开关取值

- 新增 `web/src/features/channels/hooks/use-fallback-enabled.ts`，导出 `useFallbackEnabled(): boolean`：
  复用 `useSystemOptions()`（query key `system-options`）与 `getOptionValue`，默认值取 `{ 'channel_affinity_setting.fallback_enabled': true }`；选项缺失时视为开启，与设置页 `routing-form.ts` 的 `!== 'false'` 语义一致。保存「会话与重试」时已 `invalidateQueries(['system-options'])`，因此新消费方无需额外失效逻辑。

## 渠道列表

- `channels-columns.tsx` 的 `useChannelsColumns` 增加 `showFallback?: boolean`（默认 `true`），用该文件既有的 `...(cond ? [col] : [])` 写法条件加入 `fallback` 列。保持默认 `true` 是为了让列定义不依赖 QueryClient：现有列测试（`fallback-column.test.tsx`、`sort-column.test.tsx`）直接调用该 hook 且未提供 Provider。
- `channels-table.tsx` 传入 `showFallback: useFallbackEnabled()`。
- 不用 `initialColumnVisibility`：`use-data-table.ts` 的合并顺序是存储值覆盖初始值，老用户的 `channels:column-visibility` 会盖掉初始值，且 View 下拉仍会列出该列。

## 渠道新增/编辑

- `channel-mutate-drawer.tsx` 中「路由策略」内的 `channel_info.is_fallback` FormField 改为条件渲染。
- 隐藏不重置表单值：RHF 保留 schema 默认值或编辑回填值，提交映射（`channel-form.ts` 的 `is_fallback: formData.channel_info?.is_fallback ?? false`）继续写回原值，不会因隐藏而丢标记。

## 影响与验证

- 无新增用户可见文案，locale 文件不动。
- 回归用例：列层面（`showFallback: false` 时不产生 `fallback` 列）；对话框层面（总开关关闭时不渲染 `Fallback model` 开关）。
- 检查：`bun run typecheck` + 相关 vitest 用例。

# Tasks: 调整渠道列表列数组顺序

## 任务清单

- [x] 在 `web/src/features/channels/components/channels-columns.tsx` 的 `useChannelsColumns` 中，将「序号（sort）」列定义块移动到「优先级（priority）」列之前
- [x] 运行 `bun run typecheck` 与受影响文件 lint，确认无类型错误与 lint error
- [x] 运行渠道列表相关测试（`web/src/features/channels/components/__tests__/`），确认列顺序行为正确
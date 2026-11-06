# Tasks

## 1. 复现（RED）

- [x] 1.1 在 `web/src/features/usage-logs/components/__tests__/retry-count-column.test.tsx` 新增回归用例，断言「渠道 + 用户 + 令牌」三列 width 之和等于加列前合计 460 减去「重试次数」列宽；运行 `bun run test src/features/usage-logs/components/__tests__/retry-count-column.test.tsx`（在 `web/` 下）确认该用例失败，且失败原因是三列之和仍为 460（对应总列宽超标 80）

## 2. 修复

- [x] 2.1 在 `web/src/features/usage-logs/components/columns/common-logs-columns.tsx` 为 `channel` 列设 `size: 130`、为 `user` 列设 `size: 120`、把 `token_name` 的 `size` 由 160 改为 130（保持 `link-count` 的 80），使管理员视图总 size 回到 1570；运行步骤 1.1 的用例确认转绿

## 3. 收尾验证

- [x] 3.1 运行 `bun run test src/features/usage-logs`（在 `web/` 下）确认 usage-logs 相关测试全部通过
- [x] 3.2 运行 `bun run typecheck` 与 `bun run lint`（在 `web/` 下）确认类型与 lint 无新增问题

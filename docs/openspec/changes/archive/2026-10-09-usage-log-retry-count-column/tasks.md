## 1. 前端列实现

- [x] 1.1 在 `web/src/features/usage-logs/components/columns/common-logs-columns.tsx` 的 `isAdmin` 分支内，于 `channel` 列之后新增「链路次数」列（`id: 'link-count'` 或等价唯一 id）：取 `other.admin_info.use_channel` 归一化后的长度减一作为重试次数；有链路时显示数字，无链路/序列缺失时显示 `-`；验证：管理员视角下该列紧跟在「渠道」列之后渲染，且单元格值符合口径。
- [x] 1.2 复用渠道列已有的 `Array.isArray(...).map(String).filter(Boolean)` 归一化方式对 `use_channel` 做容错，避免非数组、空数组或含空元素时报错；验证：非 relay 类日志与缺失序列时显示 `-` 且无运行时错误。

## 2. 国际化

- [x] 2.1 新增英文源键 `Chain Count`，并在 `web/src/i18n/locales/` 相关语言文件补充翻译（中文映射为「链路次数」）；验证：界面列头在 zh 显示「链路次数」、在 en 显示 `Chain Count`，且翻译键无缺失告警。
- [x] 2.2 运行 `bun run i18n:sync` 将 `Chain Count` 补齐到全部 7 个语言文件（并规范化既有重复键）；验证：7 个语言文件 JSON 合法且键集对齐（7046）。

## 3. 测试

- [x] 3.1 在 `web/src/features/usage-logs/` 的模块专属 `__tests__/` 目录新增/扩展列测试，覆盖：管理员可见且位置在渠道列之后、非管理员不可见、未重试显示 `0`、多次重试显示对应次数、无渠道链路显示 `-`；验证：运行该测试文件全部通过。

## 4. 验证与构建

- [x] 4.1 运行前端类型检查与 lint（`bun run typecheck`、针对改动文件的 lint），修复所有错误；验证：命令返回成功且无新增 error。
- [x] 4.2 运行受影响测试与前端生产构建（`bun run build`）；验证：测试通过、构建成功。

## Why

管理员使用日志（common 类）列表在「渠道」列之后新增「重试次数」列后，表格总列宽预算超出加列前的值（1650 > 1570），模型列被同比压缩；当模型名较长时，模型徽标尾部的小图标（实际模型 Route 图标 / 异常警示）被 `flex-wrap` 挤到第二行，行高不一致、可读性下降。

上一次修复 `5a9fddace` 只把「重试次数」列从默认宽度 150 收到 80，缺口仍有 80，因此问题依旧可见。

## What Changes

- 保持「重试次数」列宽度 80 不变，从「渠道」「用户」「令牌」三列各收回一部分宽度，使加列后的总列宽预算回到加列前的 1570：
  - 渠道：150 → 130
  - 用户：150 → 120
  - 令牌：160 → 130
- 新增一条 usage-logs capability 的列宽预算验收要求，并补充对应回归测试。
- 不改动 ModelBadge 组件（上一轮已处理的截断行为保留）。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `usage-logs`: 新增「重试次数列 MUST NOT 扩大表格总列宽预算」要求，约束模型列宽度不被新列挤压。

## Impact

- 代码：`web/src/features/usage-logs/components/columns/common-logs-columns.tsx`
- 测试：`web/src/features/usage-logs/components/__tests__/retry-count-column.test.tsx`
- 无后端、API、数据库、依赖变更；非管理员视图不含这三列，不受影响。

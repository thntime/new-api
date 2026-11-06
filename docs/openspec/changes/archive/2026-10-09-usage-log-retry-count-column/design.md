## Context

见 `proposal.md` 的 Why。现有使用日志「渠道」列（`web/src/features/usage-logs/components/columns/common-logs-columns.tsx`，`id: 'channel'`）已能读取 `other.admin_info.use_channel` 并在长度大于 1 时通过 Popover 展示重试链路。该数据在每次渠道尝试时由后端追加（`service/channel_select.go` 的 `AppendUsedChannel`），并通过 `service/log_info_generate.go` 写入日志 `other.admin_info`。因此「重试次数」无需后端新增字段即可从现有数据派生。

约束：`admin_info` 仅对管理员可用（非管理员日志经角色投影剥离）；「渠道」列当前也仅在 `isAdmin` 时加入列集合。

## Goals / Non-Goals

**Goals:**

- 在不改动后端、数据库与日志 API 的前提下，新增一个管理员可见的重试次数列。
- 复用现有渠道列的数据来源与可见性规则，保证口径一致。

**Non-Goals:**

- 不新增后端字段、数据库列或 API 返回字段。
- 不改变现有「渠道」列及其重试链路 Popover 的展示与交互。
- 不新增服务端排序或筛选能力。
- 不为非管理员用户展示该列。

## Decisions

### D1: 数据来源复用 `admin_info.use_channel`，前端派生重试次数

重试次数 = `use_channel` 数组长度 - 1。理由：数据已由后端记录且与渠道列同源，零后端/数据库改动，口径天然一致。
备选：后端在日志中新增独立计数字段并落库——需要改动日志模型与迁移、且与现有链路数据存在冗余，放弃。

### D2: 可见性与位置：仅管理员，紧跟「渠道」列

新列放入现有 `if (isAdmin)` 的列 push 代码块中，位置紧接 `channel` 列之后。理由：与渠道列可见范围一致；非管理员日志本就剥离 `admin_info`，无数据可用。

### D3: i18n 键名采用 `Chain Count`（中文「链路次数」）

使用英文源串 `Chain Count` 作为键，中文映射为「链路次数」，与现有 `Chain`/`Retry Chain` 的中文术语保持一致。
备选：使用 `Retry Count`（中文更常见为「重试次数」）——与用户明确要求的中文标签「链路次数」不一致，放弃。

### D4: 取值与空值展示

有渠道链路时显示数字（0/1/2…）；无渠道链路或序列缺失时显示空占位 `-`，不显示 `0`。展示形式沿用列表内紧凑单元格风格，不新增交互控件。

### D5: 不新增排序/筛选

该列仅用于展示，不新增列头排序或筛选；如表格默认对 `accessorFn` 提供客户端排序则顺带保留，不作为交付要求。

## Risks / Trade-offs

- [旧日志或非 relay 类日志缺少 `use_channel`] → 前端做空值兜底显示 `-`，不回填历史数据。
- [数值语义可能被误解为总尝试次数] → 采用「不含首次」的口径，并在该列的中文文案/说明中体现；design 记录该口径，spec 场景锁定。
- [`use_channel` 元素在后端以字符串写入、前端需容错] → 沿用渠道列已有的 `Array.isArray(...).map(String).filter(Boolean)` 归一化方式。

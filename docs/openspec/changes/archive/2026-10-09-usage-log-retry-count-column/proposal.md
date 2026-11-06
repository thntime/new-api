## Why

使用日志的「渠道」列虽然能在发生重试时通过弹层展示完整的重试链路（`admin_info.use_channel`，如 `2 → 5 → 7`），但管理员必须逐个展开才能知道某次请求到底重试了几次，无法在列表上快速对比、定位频繁重试的请求与渠道。缺少一个直接呈现重试次数的指标列。

## What Changes

- 在使用日志（common 类）表格中，于「渠道」列之后新增「链路次数」列，仅管理员可见，与现有「渠道」列的可见范围保持一致。
- 该列展示本次请求在渠道链路中的重试次数（不含首次尝试），即 `admin_info.use_channel` 的长度减一：未重试显示 `0`，无渠道链路（非 relay 类日志，如登录/充值）显示为空。
- 新增英文源文案与对应多语言翻译键。
- 不改动后端日志模型、数据库 schema 或日志查询 API 的返回结构；数据完全来自日志中已有的 `other.admin_info.use_channel`。

## Capabilities

### New Capabilities

- `usage-logs`: 使用日志列表的列展示行为，覆盖新增的「链路次数」列及其可见性、取值与空值规则。

### Modified Capabilities

<!-- 无现有 capability 的需求发生变化 -->

## Impact

- 前端：`web/src/features/usage-logs/`（common 日志列定义、移动端日志卡片字段、i18n locale 文件、相关测试）。
- 后端 / 数据库 / API：无改动；复用现有日志 `other.admin_info.use_channel` 数据。
- 权限：仅管理员可见；非管理员视角不渲染该列，日志接口仍按现有角色投影剥离 `admin_info`。

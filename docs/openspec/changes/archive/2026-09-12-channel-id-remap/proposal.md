## Why

生产库渠道 id 混乱：14 个现存渠道的 id 分散在 1-33（含已删除测试渠道留下的 id 空洞与历史残留），同时 `logs`、`quota_data` 表中残留大量已删除测试渠道的孤儿记录（约 8070 + 1151 行），导致数据看板按渠道统计时找不到对应渠道、展示异常。需要把渠道 id 重排为紧凑布局并清理孤儿数据。

## What Changes

- **渠道 id 重映射**（**BREAKING**，渠道 id 为外部可见标识，管理 API、日志查询、看板统计均引用）：
  - `1←6, 2←7, 3←8, 4←9, 5←32, 6←33`（sensenova，按现有 id 升序）
  - `11←27`（AMD）、`12←1`（opencode-go）、`13←22`（command-code）、`14←15`（deepseek）
  - `16←23, 17←25, 18←31`（火山方舟）、`21←17`（kimi-su）
  - 预留空位：`7-10`、`15`、`19-20`
- 同步更新所有引用渠道 id 的表：`channels`、`abilities`、`logs`、`quota_data`、`tasks`、`midjourneys`
- **删除孤儿数据**：`logs` 中 `channel_id` 不属于任何现存渠道的记录（含 `channel_id=0` 的失败记录，共约 8070 行）；`quota_data` 中同类孤儿记录（约 1151 行）
- **重置自增序列**：`channels_id_seq` 下次新增渠道从 22 开始（`setval(..., 22, false)`）
- **备份与演练**：库内创建两份备份 schema（备份 1 存档保留、备份 2 用于演练），先在备份 2 上执行迁移 SQL 并验证，验证通过后再对生产库执行，最后删除备份 2、保留备份 1 便于回滚

## Capabilities

无（`skip_specs: true`）。本 change 为纯数据库数据维护，不改变任何应用行为或 API 规格；现有 `channels` capability spec（序号排序）与本变更无关。

## Impact

- **数据库**：PostgreSQL（`new-api` 主库），涉及 6 张表（channels/abilities/logs/quota_data/tasks/midjourneys）+ 序列
- **应用**：不涉及代码改动；渠道 id 变化影响所有引用渠道 id 的调用方（看板统计、日志查询、管理 API）
- **执行窗口**：生产库执行迁移期间需确保应用暂停写入（或处于无写入窗口），避免迁移期间新日志使用旧渠道 id 造成漏映射
- **服务**：迁移完成后可能需要重启应用以刷新渠道内存缓存（由用户操作）

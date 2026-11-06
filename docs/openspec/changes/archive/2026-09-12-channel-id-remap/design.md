## Context

动机见 `proposal.md`。当前状态与约束：

- 主库为 PostgreSQL，渠道相关表共 6 张引用渠道 id：`channels`（id 为主键）、`abilities`、`logs`、`quota_data`、`tasks`、`midjourneys`；其中 `tasks`、`midjourneys` 当前为空表。
- 现存渠道 14 个（id 集合 `{1,6,7,8,9,15,17,22,23,25,27,31,32,33}`）；孤儿数据约 8070 行（logs）+ 1151 行（quota_data）。
- `channels_id_seq` 当前 `last_value=33`，需重置为「下次从 22 开始」（`setval(seq, 22, false)`）。
- 数据库访问通道（MCP `new_api_db_mcp`）为只读；生产写操作需要单独的可写通道（psql 授权或用户手工执行）。
- 执行迁移期间应用仍在写 `logs` 表，需确认写入窗口。

## Goals / Non-Goals

**Goals:**
- 幂等、可回滚、可验证地完成渠道 id 重映射 + 孤儿数据清理 + 序列重置。
- 库内备份 schema 演练：先在备份 2 上完整执行迁移并验证，再对生产库执行。
- 迁移后任何引用渠道 id 的表与主表完全一致，无孤儿、无错映射。

**Non-Goals:**
- 不改应用代码、不改表结构（仅重置序列值）、不迁移 users/tokens 等无关表。
- 不处理应用内存缓存（迁移完成后由用户重启应用刷新）。

## Decisions

### D1: channels 主键重映射采用「两阶段更新」（临时偏移 +1000）

`channels.id` 是主键，直接 `UPDATE` 会在目标 id 被占用时违反唯一约束（如 `6→1` 时 1 号位仍有 opencode-go）。方案：同一事务内先将 14 个渠道 id 全部 `+1000`（目标段 1-33 已空、无冲突），再按映射表一次到位。

```sql
UPDATE channels SET id = id + 1000 WHERE id IN (1,6,7,8,9,15,17,22,23,25,27,31,32,33);
UPDATE channels SET id = 1 WHERE id = 1006;  -- ... 按映射表逐条
```

替代方案（已排除）：拓扑排序单步换位（正确性难证明、易错）；临时负 id（业务层可能误判负数）。

### D2: 非唯一引用列单步映射，与主表同事务

`abilities`、`logs`、`quota_data` 的 `channel_id` 无唯一约束，可用 `CASE` 单步映射到新 id，无冲突风险。全部 UPDATE + DELETE + setval 放在**同一个事务**内，任一步失败整体 ROLLBACK，保证原子性。

### D3: 先删孤儿、再重映射

孤儿判定（`channel_id NOT IN (SELECT id FROM channels)`）依赖**旧 id**；必须先 DELETE 再 UPDATE，否则重映射后判定条件失效、孤儿无法识别。

### D4: 备份采用库内 schema，复制相关 6 表

- `backup_chan_remap_1`（存档保留）与 `backup_chan_remap_2`（演练）各复制 `channels/abilities/logs/quota_data/tasks/midjourneys` 六表（`CREATE TABLE backup_x.t AS SELECT * FROM public.t` + 索引/序列值记录），并记录 `channels_id_seq` 当前值。
- 演练 = 在 `backup_chan_remap_2` 上执行与生产完全相同的迁移 SQL（表名 schema 限定），验证通过后再对 `public` 执行。
- 备份 1 保留用于回滚比对；备份 2 演练验证后删除。

### D5: 备份 schema 命名与保留

- 备份 1：`backup_chan_remap_1`（迁移成功后长期保留，作为回滚/审计源）
- 备份 2：`backup_chan_remap_2`（演练验证通过、生产执行成功且复核无误后 `DROP SCHEMA ... CASCADE`）

### D6: 迁移 SQL 以脚本文件交付

迁移 SQL 写为可重复执行的文件（`docs/openspec/changes/channel-id-remap/` 下），表名全部 schema 限定；演练与生产执行使用同一份脚本（生产执行时去掉备份 schema 前缀或通过 search_path 切换），避免两套脚本不一致。

## Risks / Trade-offs

- [迁移窗口内应用写入新日志（旧 id）导致漏映射] → 生产执行前需应用暂停写入或确认无写入窗口；迁移脚本在事务内完成，迁移后立即校验孤儿计数。
- [删除 `channel_id=0` 的 1571 条失败记录，丢失失败请求统计] → 用户已明确确认「全部孤儿数据都删」；备份 1 保留可回溯。
- [渠道 id 为外部可见标识，变更影响调用方] → **BREAKING**，交付说明中明确提示引用渠道 id 的 API/看板/脚本需同步。
- [备份 schema 复制耗时] → logs 表约 7 万行、quota_data 数千行，复制在秒级，可接受。
- [演练与生产脚本漂移] → 同一份脚本文件、schema 参数化，产出前 diff 校验。

## Migration Plan

1. 用户确认执行窗口（应用暂停写入）与 SQL 执行通道（psql 授权或用户手工执行）。
2. 生成迁移 SQL 脚本（映射表 + 孤儿清理 + 序列重置）。
3. 创建备份 schema 1/2 并复制六表数据。
4. 在备份 2 上执行迁移脚本（演练）→ 运行验证 SQL（映射正确、孤儿为 0、序列 nextval=22）。
5. 演练通过后，对 `public` 生产库执行相同脚本（同一事务）。
6. 生产验证（同演练验证项 + 抽查日志看板关联）→ 用户重启应用刷新缓存。
7. 复核无误后删除备份 2，保留备份 1。

回滚策略：备份 1 六表快照可用于恢复迁移前状态（手工还原 channels/abilities/logs/quota_data）。

## Open Questions

- SQL 执行通道的最终确定（psql 授权给本会话执行，或用户自行执行生成好的脚本）——不影响任务拆分，执行前确认即可。
- 生产执行的具体时间窗口由用户安排。

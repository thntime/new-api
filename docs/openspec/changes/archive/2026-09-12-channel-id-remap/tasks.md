## 1. 方案准备

- [x] 1.1 生成迁移 SQL 脚本（14 条 id 映射 + 孤儿清理 DELETE + 序列 setval），在备份 2 上 EXPLAIN/预检语法无误
- [x] 1.2 生成验证 SQL 脚本（孤儿计数、新旧 id 对应关系、序列 nextval 断言），在备份 2 上可运行
- [x] 1.3 与用户确认生产执行窗口（应用暂停写入）与 SQL 执行通道（psql 授权或用户手工执行），确认结果记录在案

## 2. 备份

- [x] 2.1 创建备份 schema `backup_chan_remap_1` 并复制六表（channels/abilities/logs/quota_data/tasks/midjourneys）+ 记录 channels_id_seq 当前值，验证各表行数与 public 一致
- [x] 2.2 创建备份 schema `backup_chan_remap_2` 并复制六表，验证各表行数与 public 一致

## 3. 演练（备份 2）

- [x] 3.1 在 `backup_chan_remap_2` 上执行迁移脚本，确认无错误且事务提交
- [x] 3.2 在备份 2 上运行验证脚本：孤儿数据为 0、14 个渠道新旧 id 一一对应、`channels_id_seq` nextval=22

## 4. 生产执行

- [x] 4.1 对 public 生产库执行迁移脚本（同一事务），确认无错误
- [x] 4.2 生产验证：孤儿数据为 0、14 个渠道映射正确、序列 nextval=22、抽查 logs 与 channels 关联正常（看板可找到渠道）

## 5. 收尾

- [x] 5.1 通知用户重启应用以刷新渠道内存缓存（由用户操作）
- [x] 5.2 复核无误后删除备份 2（`DROP SCHEMA backup_chan_remap_2 CASCADE`），保留备份 1，确认备份 1 六表可查
- [x] 5.3 汇总交付说明：记录映射表、执行时间、验证结果，并提示渠道 id 为 BREAKING 变更（外部引用渠道 id 的 API/看板/脚本需同步）

---

## 交付说明（5.3）

**执行时间**：2026-09-12（备份 1/2 创建、演练、生产迁移、收尾均于当日完成）

**映射表（14 条，现 id → 新 id）**：

| 现 id | 渠道 | 新 id |
|---|---|---|
| 6 | sensenova | 1 |
| 7 | sensenova | 2 |
| 8 | sensenova | 3 |
| 9 | sensenova | 4 |
| 32 | sensenova | 5 |
| 33 | sensenova | 6 |
| 27 | AMD | 11 |
| 1 | opencode-go | 12 |
| 22 | command-code | 13 |
| 15 | deepseek | 14 |
| 23 | 火山方舟 | 16 |
| 25 | 火山方舟 | 17 |
| 31 | 火山方舟 | 18 |
| 17 | kimi-su | 21 |

预留空位：7-10、15、19-20。自增序列从 22 开始。

**数据变更**：
- 删除孤儿日志：logs 8070 行、quota_data 1151 行（含 channel_id=0 失败记录）
- 修正迁移窗口期 quota_data 孤儿 4 行（旧 id 8/7/23/32 → 新 id 3/2/16/5）
- `channels_id_seq` setval(22, false)

**验证结果（生产，MCP 只读 + bash 客户端双通道）**：
- V1 孤儿计数：logs=0、quota_data=0 ✓
- V2 映射：14 渠道新旧 id 一一对应 ✓
- V3 引用一致：logs/quota_data/abilities 无悬空 channel_id ✓
- V4 序列：(22, false) ✓
- 看板抽查：logs 全部关联到渠道名，无「未知渠道」✓

**遗留说明**：
- 备份 schema `backup_chan_remap_1` 保留（迁移前快照，channels 14 / abilities 210 / logs 73002 / quota_data 3596），可查可回滚
- 备份 2 `backup_chan_remap_2` 已删除
- **BREAKING**：渠道 id 为外部可见标识，引用渠道 id 的 API 调用方、看板脚本、监控配置需同步新 id
- 应用已重启刷新渠道缓存；如需回收删除空间可执行 `VACUUM (ANALYZE)`（由用户决定）
- 演练中发现并修复：abilities 复合主键 (group,model,channel_id) 需两阶段映射（v3 脚本），logs/quota_data 等 channel_id 无唯一约束单步安全

**已知限制（集成审查接受偏差，用户已确认）**：
- `logs.other` JSON 内嵌的 `channel_id`（顶层遗留键 + `admin_info.channel_affinity.channel_id`，约 6 万行）**未随迁移更新**，为迁移前历史审计快照；管理员展开日志详情时可能看到旧渠道 id，以日志列 `channel_id` 为准（审计记录保留迁移前事实，未改写）。用户视图不受影响（该键在 user 可见性下被剥离）。
- `midjourneys.billing_channel_id` 未纳入迁移（当前 `midjourneys` 为空表，无实际影响）；若未来该表有数据，需对 `billing_channel_id` 应用相同映射。
- 窗口期孤儿修正（quota_data 4 行）为手动 UPDATE，未脚本化；最终全表孤儿扫描确认无遗漏。
- 迁移脚本的 `SET statement_timeout/lock_timeout` 位于事务外（会话级），迁移专用会话无影响。

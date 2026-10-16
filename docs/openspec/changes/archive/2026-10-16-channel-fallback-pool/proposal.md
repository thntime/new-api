# Proposal

## Why

现有四种会话模式（`off` / `prefer` / `strict` / `descent`）只描述「正常候选池」内的选择与重试语义。当正常候选池的优先级层被走完（重试预算耗尽、环形回绕后仍全部失败，或候选池本身为空）时，请求直接失败，缺少最后一道防线。运维上存在一类「备用上游」——单价更高但更稳定，或属于不同供应商——只在所有常规渠道都不可用时才值得使用。当前只能把它们塞进正常候选池，这会让它们在正常情况下就被选中，违背「兜底」本意。

## What Changes

- 新增全局「兜底渠道池」配置：一组渠道 ID。这些渠道 MUST NOT 再作为正常优先级层的候选。
- 新增「兜底阶段」：一次请求的正常候选优先级层全部失败（或正常候选为空）后，无条件追加兜底阶段，按兜底渠道自身的优先级降序逐层尝试；同优先级层内按渠道权重加权随机。
- 兜底尝试不计入 `RetryTimes` 预算：正常层沿用现有语义用满预算（含 `descent` 的环形回绕），随后必然进入兜底阶段。
- 四种会话模式全部启用兜底阶段。`strict` 下原渠道失败或亲和不可用时不再重试其他正常渠道，直接进入兜底阶段；只有兜底阶段也全部失败才返回错误。
- 兜底渠道 MUST NOT 写入渠道亲和缓存，也 MUST NOT 成为 `descent` 的环形锚点。
- 未配置兜底渠道池时，所有模式的行为与现状逐位一致。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `channel-affinity`: 在既有「原渠道失败后如何在候选池中继续选择渠道」的契约上，新增兜底阶段与兜底渠道池的配置、排除与缓存语义；同时扩展配置接受范围以容纳兜底渠道 ID 列表。

## Impact

- 后端：
  - `model/channel_cache.go`（优先级层选择与候选来源）
  - `service/channel_select.go`（初始选择与重试选择路径）
  - `service/relay_error.go`（重试预算判定，使兜底阶段不被预算截断）
  - `setting/operation_setting/channel_affinity_setting.go`、`model/request_policy.go`（新增配置字段与白名单校验）
  - `service/channel_affinity.go`（屏蔽兜底渠道写亲和缓存与锚点）
- 前端：渠道亲和 / 请求策略设置页新增兜底渠道配置项；7 个语言包补齐文案。
- API：`GET` / `PATCH /api/option/request_policy` 的扁平 option 键新增一个字段，向后兼容，旧客户端不受影响。
- 数据库：无 schema 迁移；配置沿用现有 option 快照持久化机制。

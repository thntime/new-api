# 提案

## 为什么

渠道亲和性现有会话模式 `inherit` / `prefer` / `strict` / `off` 都无法表达「优先原渠道，原渠道失败后只按当前优先级逐层向下」这一诉求。

亲和命中渠道占用首次尝试（retry=0）后，现有实现通过 `service.AffinityAdjustedRetry` 把重试序号整体前移一位，于是第一次重试（retry=1）直接落在候选池的最高优先级层。在按优先级分层的部署（例如 50 层、每层 2 台）中，后果是原渠道失败后所有重试都反复砸向最高层，而不是就地逐层向下分摊，既压不出层间的负载分散，也让低优先级层形同虚设。

## 变更内容

- 新增会话模式取值 `descent`（枚举值与既有 `off` / `prefer` / `strict` / `inherit` 同级），语义为「优先上次成功使用的原渠道；原渠道失败后按当前优先级顺序逐层向下重试，走到最低层后回绕到最高层，形成闭环」。层内保持现有按权重加权随机。
- 环形推进以「优先级层」为单位，而不是单个渠道：同一层的多台渠道仍由现有权重逻辑选择，本变更不引入「排除本轮已失败渠道」的机制。
- 环形锚点取首轮（retry=0）实际使用渠道所在的优先级层。亲和命中时即原渠道所在层；未命中亲和时首轮本就是最高层，环序退化为与现有 `prefer` 逐位一致。
- `auto` 分组跨组时沿用现有语义：进入新分组后从该分组最高优先级层重新开始。
- 后端在全局与规则级两个 session mode 白名单同时接受 `descent`；前端全局单选项与规则下拉同时可选。
- 修订 `channels` 能力中既有需求「亲和渠道失败后的重试回归正常优先级选择」的适用范围，使该需求仅约束未显式选择 `descent` 的既有路径。
- 非破坏性：`inherit` / `prefer` / `strict` / `off` 的现有行为、渠道 pin（`PinRetryMode` / `SuppressesRetry`）语义、计费路径均不变。

## 能力

### 新增能力

- `channel-affinity`: 渠道亲和性的会话行为契约——会话如何绑定原渠道、绑定失败后如何在候选池中继续选择，以及各会话模式（`off` / `prefer` / `strict` / `descent`）对重试推进方式的约定。本变更在其中新增 `descent` 模式的需求与验收场景。

### 修改的能力

- `channels`: 修订既有需求「亲和渠道失败后的重试回归正常优先级选择」——该需求要求重试必须回到候选池最高优先级层，而 `descent` 模式要求就地逐层向下；需把该需求的适用范围限定为未显式选择 `descent` 的路径，避免同一 specs 目录内出现互相矛盾的需求。

## 影响

- 后端选路：`service/channel_select.go`（`CacheGetRandomSatisfiedChannelWithPriority`）、`model/channel_cache.go`（`GetRandomSatisfiedChannel`）、`controller/relay.go`（`getChannel`，主重试循环与 task 重试循环共用）、`service/channel_affinity.go`（模式判定与首轮标记读取）。
- 后端配置与校验：`setting/operation_setting/channel_affinity_setting.go`、`model/request_policy.go`（两个白名单 `switch` 为 fail-closed，不同步新增取值会导致配置保存直接被拒）、`service/request_policy.go`（`EffectiveSessionMode`）。
- 前端：`web/src/features/system-settings/general/channel-affinity/`（类型定义、规则编辑器下拉、会话规则表格的图标与文案回退）、`web/src/features/system-settings/request-policies/`（全局模式的 zod 校验与单选项、`policy-label`）。
- 国际化：`web/src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json` 七个语言包新增 `descent` 对应文案。
- 数据库：无 schema 变更；`session_mode` 是既有配置 JSON 中的字符串字段，新增取值不需要数据迁移。
- 计费：不涉及。选路结果的变化会影响上游请求分布，但本变更不读写任何计费量、价格或配额字段。

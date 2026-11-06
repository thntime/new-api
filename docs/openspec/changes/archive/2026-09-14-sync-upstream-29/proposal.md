# Proposal：sync-upstream-29

## Why

本地 fork 自 9/11 手工 merge（`e902cb2bd` 融合上游 rc.36）后停在 bdef11750，此后上游 QuantumNous/new-api main 又推进了 **29 个提交**（含插件计费表达式 PluginBillingExpr、legacy 价格转表达式、passkey 多 RP、渠道设置 UI 统一等）。sync-upstream 自动任务因 `controller/model_management_test.go` 的 modify/delete 冲突连续 4 天失败（#1/#4/#5/#6）——本地删除了该测试文件而上游持续修改，且 `-X ours` 兜底对 modify/delete 冲突无效（已实验证实）。需要完整追赶上游，让 sync 恢复自动。

## What Changes

采用**融合优先**策略合并上游 29 个提交，逐个解决冲突。经 merge-tree 预演，真实冲突面为 37 个文件（Go 侧 10 + 前端 27）：

- **核心恢复（用户已决策）**：
  - `controller/model_management_test.go`：从上游恢复（1669 行），消除 modify/delete 冲突
  - 插件计费表达式 PluginBillingExpr 功能链：`setting/billing_setting/tiered_billing.go`（`PluginBillingExprOption` 等 6 符号）、`pkg/jsplugin/`（`ValidPluginKey` 等）、`pkg/billingexpr/`（`TokenParams.ImgCR` 等）、`model/model_pricing_conversion.go`（本地缺失的独立文件）、`model/model_meta.go`（`ModelSquareState`）
- **Go 侧其余冲突**：`constant/channel.go`、`controller/audit.go`、`controller/channel.go`、`controller/security_enrollment_test.go`、`model/model_pricing_config.go`、`model/option.go`、`model/pricing.go`、`relay/common/relay_info.go`、`router/*` —— 逐个判断：本地特有改动（如 `DeleteModelPricingConfig`）保留，上游新功能吸收
- **前端（27 文件）**：渠道设置 UI 统一、价格编辑器扩展、i18n 7 语言并集等 —— 沿用 rc36 模式：上游重构核心优先 + 本地扩展重挂
- **验证**：`go build`/`vet`、`relaykit` 独立构建（GOWORK=off）、`bun typecheck`/`build`/`test`/`i18n:sync`、三数据库兼容验证（涉及 prefill group 索引迁移 `007d69942`）

## Capabilities

### New Capabilities

（无——本 change 为上游代码同步/集成任务，行为契约由上游定义，本项目不新增 spec 契约）

### Modified Capabilities

（无——保留本地扩展，不改变本地 spec 契约；`.openspec.yaml` 设置 `skip_specs: true`）

## Impact

- 合并上游 29 个提交（bdef11750 → 04c64734c），含：
  - **billing/plugins**：`74629e29f`（task streaming + 插件计费表达式 PluginBillingExpr）、`25ec832fa`（legacy 价格转表达式草稿）、`f256e40bc`（GPT image 表达式默认值）、`f362c7c51`（image cache/quantity 变量）——计费安全审查：涉及 `pkg/billingexpr` 新变量，需按计费安全不变量复核
  - **auth**：`385d2dfd1`（多 RP passkey）、`7fd063819`（兼容 passkey 域恢复）、`33142f0ae`（登录页行为）——OWASP 审查范围
  - **model**：`007d69942`（prefill group 唯一索引迁移）、`043ff99a5`（legacy 数据库约束）——三库迁移验证
  - **前端**：渠道设置/模型编辑器 UI 统一（`505805a4c`）、价格编辑器（`39294418a`/`b3e279464`）——bun 生态，检查新依赖
- 保留本地特有改动：`DeleteModelPricingConfig`、channel-sort 系列、SQLite 测试锁修复、AGENTS.md 不受影响
- 最终 push origin/main 后，sync-upstream 工作流应恢复自动合并（该文件从 modify/delete 变为普通内容冲突，`-X ours` 可解）

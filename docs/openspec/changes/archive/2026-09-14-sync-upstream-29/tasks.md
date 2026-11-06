## 1. 合并准备

- [x] 1.1 确认工作区干净（仅保留 Comet 注入的 AGENTS.md 管理块），记录 merge-base = bdef11750，`git merge upstream/main` 开始合并
- [x] 1.2 列出完整冲突文件清单（预期 37 个：Go 侧 10 + 前端 27），与设计 D2/D3 对照

## 2. Go 侧冲突解决

- [x] 2.1 `controller/model_management_test.go` 取上游版本恢复，确认文件存在且非空
- [x] 2.2 `pkg/jsplugin/*`、`pkg/billingexpr/*` 取上游版本，确认 `ValidPluginKey`、`TokenParams.ImgCR` 等符号存在
- [x] 2.3 `setting/billing_setting/tiered_billing.go` 融合：上游 PluginBillingExpr 6 符号 + 本地 smoke-test 基建
- [x] 2.4 `model/model_pricing_conversion.go` 取上游（新文件），确认 `ModelPricingConversion` 类型存在
- [x] 2.5 `model/model_meta.go` 融合：保留本地 11 列架构，补回 `ModelSquareState` 类型与 4 常量
- [x] 2.6 `model/model_pricing_config.go` 融合：上游重构 + 保留本地 `DeleteModelPricingConfig`
- [x] 2.7 `model/pricing.go`、`model/option.go`、`relay/common/relay_info.go` 融合上游新类型与本地改动
- [x] 2.8 `controller/channel.go`、`controller/audit.go`、`constant/channel.go`、`controller/security_enrollment_test.go` 融合
- [x] 2.9 `router/api-router.go`、`router/channel-router.go`、`router/channel_router_test.go` 融合

## 3. 前端冲突解决

- [x] 3.1 渠道设置/模型编辑器相关 17 个前端文件融合（上游重构核心 + 本地 channel-sort 扩展重挂）
- [x] 3.2 i18n 7 语言 locale 文件做并集合并，`web/src/i18n/static-keys.ts` 同步
- [x] 3.3 `web/bun.lock`、`web/package.json` 合并后运行 `bun install` 锁定依赖

## 4. Go 侧验证

- [x] 4.1 `go build ./...` 与 `go vet ./...` 全部通过
- [x] 4.2 `cd relaykit && GOWORK=off go build ./...` 通过（relaykit 独立模块）
- [x] 4.3 计费安全复核：PluginBillingExpr 计费路径使用 `QuotaRoundChecked` 等饱和处理、无裸 int 转换，relay_task.go 接入点符合计费不变量
- [x] 4.4 OWASP 复核：passkey 多 RP 改动（385d2dfd1/7fd063819）审计事件不含凭证/token，域移除需确认，记录 ASVS 版本与验证结果
- [x] 4.5 三库迁移验证：`007d69942` prefill group 索引迁移——PG 用 backup_chan_remap_1 实例验证；MySQL 按用户指示跳过；SQLite 本地可跑则跑（glebarez/sqlite)，跑不了一并跳过；记录版本与结果（用户已授权：PG=backup_chan_remap_1、MySQL=跳过、SQLite=尽力而为）
- [x] 4.6 上游新增 billing 相关 Go 测试运行通过（`go test ./setting/billing_setting/ ./pkg/billingexpr/ ./pkg/jsplugin/`）

## 5. 前端验证

- [x] 5.1 `bun run typecheck` 通过
- [x] 5.2 `bun run build` 通过
- [x] 5.3 `bun run i18n:sync` 后 locale 文件与 static-keys 一致（无遗漏 key）
- [x] 5.4 前端关键测试通过（`bun run test` 渠道/定价相关用例）

## 6. 收尾

- [x] 6.1 整体回归：本地主要 Go 测试（controller/model/service 关键包）运行通过
- [x] 6.2 merge 提交完成（push 推迟：用户选择 verify 通过后推送）
- [x] 6.3 验证 sync 恢复（推迟至 verify 通过后执行 push 时完成：workflow_dispatch 或下个自然周期观察 rerere 生效）

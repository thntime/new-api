# Design：sync-upstream-29

## Context

- 本地 main 停在 9/11 17:05 的手工 merge（`e902cb2bd`，融合上游至 bdef11750）。上游此后推进 29 个提交至 04c64734c
- 冲突根因：`controller/model_management_test.go` 在历史 merge 中被处理为「删除」，上游持续修改 → modify/delete 冲突，`-X ours` 兜底无效（已实验证实）
- 上一轮 rc36 merge 决策：model_meta.go 保留本地 11 列富元数据架构、**放弃上游 ModelSquareState**——而待恢复的测试文件依赖 ModelSquareState，需一并补回
- 本地另有 194 个特有提交（channel-sort 系列、DeleteModelPricingConfig、SQLite 测试锁修复等）必须保留

## Goals / Non-Goals

**Goals:**
- 完整合入上游 29 个提交（bdef11750 → 04c64734c），恢复 PluginBillingExpr 插件计费表达式、legacy 价格转换、passkey 多 RP、渠道设置 UI 统一等功能
- 恢复 `controller/model_management_test.go`（消除 modify/delete 冲突），补齐其依赖符号（PluginBillingExpr、ModelSquareState、ModelPricingConversion 等）
- push 后 sync-upstream 工作流恢复自动合并
- 保留全部本地特有改动

**Non-Goals:**
- 不反向修改上游行为契约
- 不引入上游之外的新功能
- 不处理上游 29 个提交之外的落后（无）

## Decisions

### D1: merge 执行方式——直接 `git merge upstream/main`，逐冲突融合
- 基于 merge-base bdef11750 做三方合并，37 个两边都改的文件逐个判断
- 备选（rebase / cherry-pick 逐提交）被否：提交链有大量相互依赖（74629e29f → 25ec832fa 链式重构），逐提交 replay 会产生中间态编译失败且无法验证；单次 merge 可整体验证

### D2: 冲突分类处置（Go 侧 12 文件）
| 文件 | 处置 |
|------|------|
| `controller/model_management_test.go` | **取上游**（modify/delete，恢复文件） |
| `setting/billing_setting/tiered_billing.go` | 上游为主（PluginBillingExpr 6 符号），保留本地 smoke-test 基建 |
| `pkg/jsplugin/*`、`pkg/billingexpr/*` | 取上游（本地只是停留旧版，无特有改动） |
| `model/model_pricing_conversion.go` | 取上游（本地缺失的独立新文件） |
| `model/model_meta.go` | 融合：本地 11 列架构 + 补回 `ModelSquareState` 类型/常量（测试依赖） |
| `model/model_pricing_config.go` | 上游 227 行重构 + 保留本地 `DeleteModelPricingConfig` |
| `model/pricing.go`、`model/option.go` | 融合（上游新类型 + 本地小改动） |
| `controller/channel.go`、`controller/audit.go`、`constant/channel.go` | 融合（上游 passkey 审计/URL 占位 + 本地改动） |
| `relay/common/relay_info.go`、`router/*` | 融合（上游 token 明细 + 本地路由扩展） |

### D3: 前端处置（27 文件）
沿用 rc36 模式：上游重构核心优先（渠道设置统一、价格编辑器）+ 本地扩展重挂（channel-sort 相关 UI、i18n 并集）。`web/bun.lock`/`package.json` 以合并结果为准后跑 `bun install` 锁定。

### D4: 计费/安全合规复核（merge 后强制）
- PluginBillingExpr 涉及计费路径：复核 `QuotaRoundChecked` 等饱和处理、`ResolveTaskBillingExpr` 在 relay_task.go 的接入点符合计费安全不变量
- passkey 多 RP（385d2dfd1/7fd063819）：按 OWASP ASVS 复核审计事件不含凭证、域移除需确认
- prefill group 索引迁移（007d69942）：三库（SQLite/MySQL/PostgreSQL）迁移验证

## Risks / Trade-offs

- [37 文件冲突面大，前端文件多] → 逐文件小步提交，Go 与前端分批验证；关键文件先 oracle/设计评审
- [测试文件依赖本地曾放弃的 ModelSquareState] → merge 后显式补回该类型，跑 `go vet` 全量验证
- [PluginBillingExpr 计费逻辑回归风险] → 复核计费不变量 + 跑上游新增 billing 测试用例
- [passkey 多 RP 改动触及 OWASP 审计要求] → 按 ASVS 复核审计事件字段
- [三库迁移回归] → 007d69942 在三种数据库各跑一次 AutoMigrate（含升级旧库）
- [web/bun.lock 冲突] → 合并后 `bun install --frozen-lockfile` 校验，必要时重新生成

## Migration Plan

1. `git merge upstream/main` 开始合并，逐个解决 37 个冲突文件（先 Go 后前端）
2. Go 验证：`go build ./...`、`go vet ./...`、`cd relaykit && GOWORK=off go build ./...`
3. 计费/安全/迁移复核（D4）
4. 前端验证：`bun install`、`bun run typecheck`、`bun run build`、`bun run i18n:sync`
5. 三数据库迁移验证（涉及 007d69942）
6. 提交 merge，`git push origin main`
7. 观察下一次 sync-upstream 自动运行结果（应成功，rerere 缓存自动更新）

## Open Questions

无——用户已确认完整追赶 + 恢复测试/插件计费功能，D2 冲突分类已覆盖全部 Go 冲突文件。

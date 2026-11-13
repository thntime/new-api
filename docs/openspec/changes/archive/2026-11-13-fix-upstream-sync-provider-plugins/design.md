# 设计：合并 provider plugins 批次的上游冲突

## 背景与目标

`upstream/main` 新增 5 个提交（provider/request 插件与社区市场、moejs 升级、前端依赖升级、vertex-ai 插件版本、前端文案修复），与本地定制在 39 个文件冲突。本设计在 `main` 上完成三方合并，按"上游新能力保留 + 本地定制保留"的原则逐类处理，验证通过后推送。需求与验收以 `proposal.md` 为准。

## 冲突分类与处理策略

### A. i18n（7 个文件，各 36 处）

沿用已验证方案（2026-10-23 `fix-sync-upstream-conflict`、2026-11-12 `fix-upstream-sync-conflicts`）：

1. 取 `:1:`（base）/`:2:`（ours）/`:3:`（theirs）的 `translation` 对象，按键集并集输出；`theirs` 独有键插入到其在 `theirs` 中的前一个键之后。
2. 三方逐键取值：ours==theirs 取任一；ours==base 取 theirs（上游改动）；theirs==base 取 ours（本地改动）；三者互异则停止并报告。
3. 写入后 `bun run i18n:sync` 统一键序与 `stableStringify` 格式；验收为二次运行无变化、报告 `missingCount=0`/`extrasCount=0`。

### B. 前端依赖（`web/package.json`、`web/bun.lock`）

- `package.json` 语义融合：保留本地新增依赖，接受上游版本升级；同名依赖取上游版本（本次就是升级目标）。
- `bun.lock` 不手工解冲突：`git checkout --theirs web/bun.lock` 后执行 `bun install`，由 bun 依据合并后的 `package.json` 重新生成；验收 `bun install --frozen-lockfile` 无变化。

### C. 后端（12 个 Go 文件）

原则：上游新增的接口、字段、常量与插件路由全部保留；本地定制不丢失。重点文件：

| 文件 | 关注点 |
| --- | --- |
| `constant/api_type.go` | 上游新增渠道类型常量与本地新增类型并存，编号冲突时以上游为准并核对本地引用 |
| `model/channel.go` / `model/option.go` | 渠道字段与选项字段并集；本地分页上限、接入方式字段保留 |
| `model/pricing.go` | 上游计费相关字段/表达式改动优先；本地价格管理定制保留，合并后核对计费表达式语义 |
| `controller/channel.go`、`controller/channel_upstream_update.go`、`controller/channel-billing.go` | 上游插件与渠道更新逻辑保留，本地分页 200、接入方式校验保留 |
| `relay/relay_adaptor.go`、`relay/channel/api_request.go`、`relay/responses_websocket.go` | 上游 provider plugin adaptor 选择与请求构造保留，本地渠道分支保留 |
| `router/channel-router.go` | 路由并集，避免重复注册 |
| `relaykit/dto/channel_settings.go` | 本地 CodeBuddy region 等校验保留；上游新增字段保留；`relaykit` 必须可独立构建 |

测试文件（`model/json_column_test.go`、`model/channel_constraint_test.go`、`controller/*_test.go`）按"保留两侧用例、以能编译且断言语义一致"处理；上游用例依赖的本地过滤逻辑需补 fixture。

### D. 前端业务代码（12 个文件）

| 区域 | 策略 |
| --- | --- |
| `channels/lib/channel-form.ts`（6 处，本地 103 行 vs 上游 23 行） | 以上游 zod 结构为骨架，回接本地接入方式枚举与解析 |
| `channels/api.ts`（本地 328 行 vs 上游 63 行） | 逐段核对：上游新增插件/市场接口保留，本地分页与查询参数保留 |
| `channel-mutate-drawer.tsx` | 保留上游新增字段与逻辑，保留本地 VolcEngine/Zhipu/Moonshot 回显与 provider 目标选择 |
| `channels/types.ts`、`channel-actions.ts`、`channels-columns.tsx`、`data-table-row-actions.tsx`、`balance-query-dialog.tsx` | 类型与操作并集 |
| `system-settings/*`（operations、request-policies、maintenance、types） | 上游新增设置项与本地新增项并存，section-registry 去重 |
| `__tests__/*` | 保留两侧用例，必要时按合并后 API 调整调用 |

## 验证

1. `git diff --name-only --diff-filter=U` 为空；全仓无冲突标记。
2. 根模块、`relaykit`、`tokenkit`：`GOWORK=off go vet ./...` 均 0。
3. `bunx tsc -b web/tsconfig.json` 通过；`bun --cwd web run test` 跑渠道/系统设置相关测试文件。
4. `bun run i18n:sync` 幂等；`bun install --frozen-lockfile` 无变化。
5. 重点人工核对：`model/pricing.go`、`controller/channel-billing.go`、`relay/channel/api_request.go` 的合并结果是否改变计费或请求语义（涉及计费的文件按项目规则先读 `.agents/rules/billing.md`）。

## rerere 种子回填

解决冲突并 `git add` 后，把 `.git/rr-cache` 新增记录复制到 `.github/rerere-cache/`，折入合并提交（`git commit --amend --no-edit`），确认两目录 `diff -r` 无差异。

## 风险与授权

- 本次触发 hotfix 的多模块协调与文件数阈值，用户已在决策点明确选择"继续 hotfix（授权超阈值）"，范围仅限本次上游合并冲突解决，不新增功能；授权记录见 `.comet/rulings.md`。
- `review_mode: off`（hotfix 预设），不安排独立审查；通过逐文件核对 + 全量 vet/tsc/测试实跑降低风险。

# 修复 Sync Upstream 冲突：合并 provider plugins 批次

## 问题

`upstream/main` 在 issue #21 修复后继续前进 5 个提交，本地 `main` 落后 5、领先 349。预演（`git merge-tree --write-tree HEAD upstream/main`）显示 **39 个冲突文件**，远大于上次的 8 个。上游提交：

- `0bda086fb` chore(web-deps): upgrade frontend dependencies (#6999)
- `f2aa91232` chore(plugins): release vertex-ai 1.3.1 (#8313)
- `7b857f5a1` feat(plugins): add provider plugins, request plugins and a community marketplace (#8271)
- `f3d8b737b` perf: 升级 MoeForge moejs/moego，请求体按需流式读取 (#8311)
- `a9932d1f9` fix(web): sync risk confirmation text on composition end and blur

冲突分布：

- 7 个 `web/src/i18n/locales/*.json`：各 36 处（两侧在同一批锚点新增键）
- 前端渠道与系统设置：`channel-form.ts`（6 处）、`api.ts`（本地 328 行 vs 上游 63 行）、`channel-configuration.test.tsx`（4 处）、`channel-mutate-drawer.tsx` 等 12 个文件
- 前端依赖：`web/package.json`、`web/bun.lock`
- 后端：`controller/channel.go`（4 处）、`controller/channel_upstream_update.go`、`controller/channel-billing.go`、`model/channel.go`、`model/option.go`、`model/pricing.go`、`relay/relay_adaptor.go`、`relay/channel/api_request.go`、`relay/responses_websocket.go`、`router/channel-router.go`、`constant/api_type.go`、`relaykit/dto/channel_settings.go` 及 3 个测试文件

## 根因

1. 上游本批次引入 provider/request 插件与社区市场，改动了渠道类型常量、渠道模型、relay adaptor 选择与前端渠道表单/API，与本地已有的渠道接入方式定制（VolcEngine/Zhipu/Moonshot/Cline/OpenCode Go/SenseNova）、分页上限 200、provider 目标选择等改动落在同一批代码区间。
2. 前端依赖升级与本地 `package.json`/`bun.lock` 冲突。
3. locale 文件继续双向插入，与上次同源。

## 修复目标

1. 在本地 `main` 完成 `upstream/main` 三方合并，39 个冲突文件全部按两侧语义融合，不使用 `-X ours` 或整侧覆盖。
2. locale：7 个语言键集并集（沿用 2026-10-23/2026-11-12 已验证的脚本方案），`bun run i18n:sync` 幂等，`missingCount=0`、`extrasCount=0`。
3. 依赖：`package.json` 语义融合后由 `bun install` 重新生成 `bun.lock`，不手改锁文件。
4. 后端：保留上游插件/渠道新接口与字段，保留本地渠道接入方式、分页与计费相关定制；`relaykit/dto/channel_settings.go` 的本地校验（如 CodeBuddy region）不得丢失。
5. 验证：根模块、`relaykit`、`tokenkit` 的 `GOWORK=off go vet ./...` 全 0；`bunx tsc -b` 通过；渠道/插件相关前端测试通过；`i18n:sync` 幂等。
6. 回填 rerere 种子并折入合并提交；推送 `origin/main`。

## 范围外

- 不实现上游新功能之外的本地改造，不调整插件市场与 provider plugins 的设计。
- 不修改 `sync-upstream.yml` 流程。
- 不处理与本次合并无关的工作区改动。

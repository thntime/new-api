# Tasks

## 1. 合并与冲突解决

- [x] 1.1 记录基线（`HEAD`、`upstream/main`、merge-base）并执行 `git merge --no-edit upstream/main`，确认 `git diff --name-only --diff-filter=U` 与预演一致的 39 个冲突文件 <!-- comet-task:df71977f-8672-43ef-aefc-e3ccffee417f -->
- [x] 1.2 解决 7 个 `web/src/i18n/locales/*.json`（各 36 处）：键集并集 + `bun run i18n:sync` 归一；验收：无冲突标记，报告 7 语言 `missingCount=0`/`extrasCount=0`，二次运行无变化 <!-- comet-task:afcad513-f367-430e-974f-dad16dae084b -->
- [x] 1.3 解决 `web/package.json` 与 `web/bun.lock`：语义融合 `package.json`，锁文件取上游后 `bun install` 重新生成；验收：`bun install --frozen-lockfile` 无变化 <!-- comet-task:8e9a7fa1-1564-497a-9b63-3688c130adc8 -->
- [x] 1.4 解决 12 个后端 Go 冲突文件：保留上游插件/渠道新接口与本地定制，重点核对 `model/pricing.go`、`controller/channel-billing.go`、`relay/channel/api_request.go`；验收：无冲突标记，`GOWORK=off go build ./...` 通过 <!-- comet-task:9e3c53a7-4a85-4c02-926b-610d23201ef4 -->
- [x] 1.5 解决 12 个前端业务冲突文件（channels 与 system-settings）；验收：无冲突标记，`bunx tsc -b web/tsconfig.json` 通过 <!-- comet-task:82eeed15-8f72-4001-bcaf-490513f4f142 -->
- [x] 1.6 合并结果模块验证：根模块、`relaykit`、`tokenkit` 分别 `GOWORK=off go vet ./...`，退出码均为 0 <!-- comet-task:06afd87f-44f0-4b9c-a83d-b73cd92a7d8a -->
- [x] 1.7 前端回归：运行渠道与系统设置相关测试文件，确认通过 <!-- comet-task:3b5509d2-3407-4f1e-b353-cc7ddc937f85 -->

## 2. 提交与种子

- [x] 2.1 回填 rerere 种子到 `.github/rerere-cache/` 并折入合并提交；验收：`git log --oneline -1` 为 merge commit，两目录 `diff -r` 无差异 <!-- comet-task:9232ec1e-0fbf-45bd-8add-d395294351f5 -->

## Workflow follow-up

交付不在 build 任务范围，由归档阶段执行并记录：

- 推送 `origin/main`，确认 `git rev-parse origin/main` 等于本地 `HEAD`
- 观察一次 CI / Build Docker Image / Release 的推送触发结果
- 若本次合并修复了同步失败，确认 `sync-upstream` 下一次定时任务短路

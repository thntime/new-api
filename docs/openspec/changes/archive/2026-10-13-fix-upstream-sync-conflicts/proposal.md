# Proposal: fix-upstream-sync-conflicts

## 问题描述

`.github/workflows/sync-upstream.yml` 每日自动把上游 `QuantumNous/new-api` 的 `main` 合并进本 fork（`thntime/new-api`）。本次合并产生内容冲突，自动流程无法解决，CI 创建 issue #11 要求人工处理。

## 背景与现状

- 本地 `main`（`dd97fdb76`）相对 `upstream/main` **领先 236 个提交**（大量 fork 定制），**落后 48 个提交**；属于真实三方合并，不能 fast-forward。
- 上游 tip 为 `633b4c5b1`（2026-10-12）。issue #11 记录时的冲突清单已过期（当时仅 2 个前端文件）；当前 dry-run（`git merge-tree`）显示 **6 个文件冲突**。

## 根因分析

fork 本地定制与上游最新改动在同区域重叠，git 无法自动合并：

| 冲突文件 | 本地（fork）改动 | 上游改动 |
|----------|------------------|----------|
| `.github/workflows/ci.yml` | 新增 `push` 触发 + checkout `repository` 回退到 `github.repository` | checkout `ref` 改为 `${{ github.sha }}` 并加注释 |
| `controller/security_enrollment_test.go` | 新增 `newSecurityTestDatabase` helper，db/logDB 均启用 `_txlock=immediate` | 内联仅对 `db` 启用 `_txlock=immediate`，`logDB` 保持共享 helper |
| `web/package.json` | 新增 `test` 脚本、`@testing-library/dom`，升级 `user-event` | 批量升级多个依赖版本（`@lobehub/icons`、`ai`、`katex`、`dompurify` 等） |
| `web/bun.lock` | 随本地依赖调整 | 随上游依赖升级大量变化 |
| `channel-mutate-drawer.tsx` | +740/−93 的本地大改 | +28 行上游新增 |
| `model-cards.test.tsx` | 改为断言「单页渲染、无分页控件」（fork `MODEL_CARD_GRID_PAGE_SIZE = 1000`） | 重写多处测例（稀疏成功率样本、cache read 价格、请求带 `recent_model` 等）并保留分页测例 |

## 修复目标

完成一次人工合并：解决 6 个冲突文件（**融合优先**：保留本地定制并吸收上游改动），确保 `GOWORK=off go vet ./...`（含 `relaykit`）通过；提交合并结果并回填 rerere cache，推送后使同步 workflow 可正常重跑。

## 非目标

- 不新增功能、不改变对外 API/schema。
- 不重构 fork 既有的分页行为或其他定制。

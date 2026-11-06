# 任务：解决 sync-upstream 冲突

- [x] 1. 记录 RED 证据：`git merge-tree --write-tree --name-only HEAD upstream/main` 复现与 run 39894144682 / issue #16 一致的 8 个冲突文件 <!-- comet-task:2de2fde1-1643-4e7d-89a5-f28867b1e75c -->
- [x] 2. 执行 `git merge --no-edit upstream/main`（开启 rerere），进入冲突状态 <!-- comet-task:a0339318-d09a-457b-8ce3-651bd2b08a1b -->
- [x] 3. 解决 `controller/ratio_sync_test.go`：恢复上游版本并补 `setupRatioSyncTestDB` 与 5 个模型种子 <!-- comet-task:3411290d-21b0-4bb6-bd72-6139cee66923 -->
- [x] 4. 解决 7 个 `web/src/i18n/locales/*.json`：键集并集（本地 278 + 上游 22），en 顺序为基准 <!-- comet-task:f9ef2f1c-52f4-47ad-8309-ba44db9eacc6 -->
- [x] 5. 运行 workflow 同款 vet 关卡（root / relaykit）与 `go test ./controller -count=1` <!-- comet-task:4d8d8f12-b9b7-475e-97e7-355bf9eb78ce -->
- [x] 6. 运行 `bun run typecheck` 与 `bun run i18n:sync` 幂等校验 <!-- comet-task:984d1c93-83d1-448b-9b01-ec48f4fdadcb -->
- [x] 7. 回填 `.github/rerere-cache/` 并把种子折入合并提交 <!-- comet-task:86274695-decf-4ca5-b057-57876d12c34e -->

交付（不在 build 任务范围，由归档阶段的 `comet state delivery` 记录）：推送 main、重跑 sync-upstream 确认短路成功、关闭 issue #16。

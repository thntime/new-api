# 修复 Sync Upstream 每日同步冲突

## 问题

`Sync Upstream` workflow（run [39894144682](https://github.com/thntime/new-api/actions/runs/39894144682)，headSha `304a68e`）在「Merge upstream into main (three-way + rerere)」步骤失败，并自动创建 issue #16。本地 `git merge-tree --write-tree --name-only HEAD upstream/main` 预演结果与 CI 完全一致，冲突文件为：

- `controller/ratio_sync_test.go`：修改/删除冲突（本地侧不存在该文件，上游 `eb169e74d` 修改了它）
- `web/src/i18n/locales/{en,fr,ja,ru,vi,zh-TW,zh}.json`：内容冲突（两侧在同一位置新增翻译键）

上游待合并提交为 `7d8b91268..7fb6e57a5` 共 13 个提交（含价格同步汇率调整、12 小时缓存写入计费、任务插件请求头透传等）。

## 根因

1. **`controller/ratio_sync_test.go`**：该文件由上游在本仓库分叉之后新增，本地 main 的 first-parent 链上从未存在过它，因此每次合并都表现为「我方删除」。此前上游未再修改该文件，三方合并直接沿用删除；本次上游 `eb169e74d` 为 `TestPricingSyncCompleteSourcesAndArrayFormats` 追加了 `include_unchanged` 断言，触发修改/删除冲突。
   - 本地 `controller/ratio_sync.go` 已被本地提交 `e0880740a` 改造：`FetchUpstreamRatios` 新增 `model.GetAllModelNames()` 过滤，只保留模型库中已存在的模型。上游测试没有初始化 `model.DB`，直接取回上游版本会 panic（`gorm.(*DB).getInstance` nil），即使补上 DB 初始化，测试数据中的模型名也不在模型库中，断言会失败。
   - 上游 `include_unchanged` 实现（`controller/ratio_sync.go` 中的 `IncludeUnchanged`）在本次合并中已自动并入本地，因此该测试覆盖的行为在本地是真实存在的。
2. **locale JSON**：两侧都向同一批位置新增翻译键。经比对，本地新增 278 个键、上游新增 22 个键，**没有**任何同名键取值不同，冲突纯粹是插入位置重叠。`web/scripts/sync-i18n.mjs` 以 `en.json` 为基准顺序重排其余语言，因此顺序本身不是语义契约，但必须保证键集完整。

## 修复目标

1. 在本地 main 完成 `upstream/main` 三方合并，冲突按两侧语义融合，不做整侧覆盖、不使用 `-X ours`。
2. `controller/ratio_sync_test.go` 恢复上游版本，并适配本地「只同步模型库已有模型」的过滤逻辑（补 DB fixture 与模型种子），使上游新增的 `include_unchanged` 断言在本地真实执行并通过。
3. 7 个 locale JSON 取键集并集：本地 278 个键与上游 22 个键全部保留，无取值冲突；`en.json` 保持本地既有顺序并把上游新增键插到其上游相邻位置，其余语言按 `en.json` 顺序归一（与 `i18n:sync` 行为一致）。
4. 通过 workflow 同款关卡：根模块与 `relaykit` 的 `GOWORK=off go vet ./...`；额外验证 `controller` 包测试、`bun run typecheck`、`bun run i18n:sync` 幂等。
5. 回填 rerere 种子到 `.github/rerere-cache/`（唯一事实源），把种子折入合并提交，推送 main 后重跑 workflow，确认其在「Check if upstream already merged」短路成功，并手工关闭 issue #16。

## 范围外

- 不实现上游未合并的新功能，不调整本地价格同步业务规则。
- 不修改 `sync-upstream.yml` 的流程设计。

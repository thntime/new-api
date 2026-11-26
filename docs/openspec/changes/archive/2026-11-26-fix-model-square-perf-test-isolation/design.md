# 设计：清空 perf 热缓存以隔离模型广场测试

## 背景

`pkg/perf_metrics` 用包级 `sync.Map` 保存尚未落库的分钟/小时样本（`hotBuckets`）。兜底测试通过 `Relay` 写入样本后未清理，`TestModelSquareGroupVisibility` 的 `QuerySummaryAll` 会把这些样本合并进摘要，导致断言看到 7 个额外模型。

## 方案

- 在 `pkg/perf_metrics/metrics.go` 增加：

  ```go
  // ResetHotBuckets drops all in-memory samples. Tests use it to isolate the
  // process-global cache between cases.
  func ResetHotBuckets() {
      hotBuckets.Clear()
  }
  ```

- 在 `TestModelSquareGroupVisibility` 开头调用 `perfmetrics.ResetHotBuckets()`，并用 `t.Cleanup(perfmetrics.ResetHotBuckets)` 在结束后再次清空，避免本测试影响后续用例。
- 不修改 `QuerySummaryAll`、`flushCompletedBuckets` 或任何生产路径。

## 取舍

- 不在兜底测试的 `setupFallbackStageTest` 中清理：清理放在依赖干净缓存的测试侧，能同时隔离其他可能写入 `hotBuckets` 的用例，且改动只落在一个测试文件。
- 不通过放宽断言绕过：保留“摘要只包含本测试模型”的严格断言，确保组可见性语义仍被验证。
- `ResetHotBuckets` 是测试夹具入口，不参与生产刷新生命周期；生产缓存仍由 `flushLoop` 管理。

## 验证

1. 复现：先跑 `TestFallbackSequenceMatchesSpecExample`，再跑 `TestModelSquareGroupVisibility`，确认修复前失败。
2. 修复后同一命令通过；单独跑 `TestModelSquareGroupVisibility` 通过。
3. 回归：`go test ./pkg/perf_metrics -count=1` 与相关 controller 用例通过。
4. 反向验证：临时移除清空调用，确认目标测试重新失败；恢复后全绿。

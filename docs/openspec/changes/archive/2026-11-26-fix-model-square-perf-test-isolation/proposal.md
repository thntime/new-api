# Proposal

## 问题

CI 的 `Backend vet, build, and test` job 在 `TestModelSquareGroupVisibility/sqlite` 失败：`controller/model_list_test.go:244` 期望性能摘要只有 1 个模型，实际返回 7 个 `fallback-*` 模型。该失败在本次 workflow 改动前的提交 `9d2fec66a` 已存在，不是本次改动引入。

## 根因

`pkg/perf_metrics` 的 `hotBuckets` 是进程级全局内存缓存：

1. `controller/channel_fallback_test.go` 的兜底测试调用 `Relay(...)`，触发 `perfmetrics.RecordRelayResult`，把 `fallback-*` 模型的样本写入 `hotBuckets`。
2. 这些测试结束后没有清空 `hotBuckets`。
3. `TestModelSquareGroupVisibility` 随后调用 `QuerySummaryAll`，该函数把 `hotBuckets` 的样本合并进摘要，导致 7 个兜底模型出现在结果中。

本地复现：

```bash
GOWORK=off go test ./controller \
  -run '^(TestFallbackSequenceMatchesSpecExample|TestModelSquareGroupVisibility)$' \
  -count=1
# FAIL: summary.Models 多出 fallback-sequence-model
```

## 修复目标

- 为 `hotBuckets` 提供一个明确的测试隔离入口，并在 `TestModelSquareGroupVisibility` 开始前清空、结束后恢复。
- 保持生产行为不变：`QuerySummaryAll` 的合并逻辑、缓存刷新与指标计算均不修改。
- 目标测试与相关 controller / `pkg/perf_metrics` 回归通过。

## 范围

- `pkg/perf_metrics/metrics.go`：新增 `ResetHotBuckets`（测试隔离用，清空进程级缓存）。
- `controller/model_list_test.go`：在 `TestModelSquareGroupVisibility` 开始前与 `t.Cleanup` 中调用。
- 不新增业务能力，不改变公开 API 行为、数据库 schema 或跨模块接口。

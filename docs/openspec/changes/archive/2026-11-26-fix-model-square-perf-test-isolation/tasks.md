# Tasks

## 1. 修复与验证

- [x] 1.1 在 `pkg/perf_metrics/metrics.go` 增加 `ResetHotBuckets`（清空 `hotBuckets`），并在 `controller/model_list_test.go` 的 `TestModelSquareGroupVisibility` 开头调用、用 `t.Cleanup` 再次清空；验收：先复现失败，再跑 `GOWORK=off go test ./controller -run '^(TestFallbackSequenceMatchesSpecExample|TestModelSquareGroupVisibility)$' -count=1` 通过 <!-- comet-task:b30de293-92b6-45aa-a939-a42bccfb4bb0 -->
- [x] 1.2 回归：`GOWORK=off go test ./controller -run '^TestModelSquareGroupVisibility$' -count=1`、`GOWORK=off go test ./controller -run '^(TestFallback|TestModelSquare)' -count=1`、`GOWORK=off go test ./pkg/perf_metrics -count=1` 全部通过 <!-- comet-task:a854227b-a2aa-47f6-a187-4b8778850810 -->
- [x] 1.3 反向验证：临时移除 `ResetHotBuckets` 调用，确认目标测试重新失败并保存输出；恢复后确认全绿 <!-- comet-task:cb86184c-5fc5-4acd-b58c-64eacaafddd0 -->

## Workflow follow-up

- 归档后推送 `origin/main`，观察下一次 CI 的 `Backend vet, build, and test` job。

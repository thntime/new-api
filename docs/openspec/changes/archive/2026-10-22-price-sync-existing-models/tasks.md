# Tasks

## 1. 价格同步按模型库过滤

- [x] 1.1 在 `model/model_meta.go` 新增 `GetAllModelNames()`（`Pluck("model_name")`）；验证：`go build ./...` 通过，且新增单测在 SQLite 下返回已创建模型的名称。
- [x] 1.2 在 `controller/ratio_sync.go` 的 `FetchUpstreamRatios` 中，`buildDifferences` 之后按模型库名称集合过滤差异，查询失败返回 500；验证：新增测试用「库中已有模型 + 上游独有模型」断言 `differences` 与 `prices` 只含前者。
- [x] 1.3 确认过滤对所有来源生效（opencode-go 预设、models.dev、自定义渠道）；验证：测试中同时提供两个来源，断言两者结果都只保留库中模型。

## 2. opencode 目录来源优先级

- [x] 2.1 调整 `service/opencodego_models.go` 的段遍历顺序为 `opencode-go` → `opencode` → 其余段名升序，免费 `-free` 覆盖仍在最后；验证：`go test ./service/ -run OpenCodeGo` 通过。
- [x] 2.2 新增/扩展用例：同 id 出现在 `opencode` 与其他段时取 `opencode`；只在两个其他段时取段名靠前者；`opencode-go` 存在时仍最优先；验证：上述用例全部通过。

## 3. 集成验证

- [x] 3.1 后端：`go build ./...` 与 `go test ./service/ ./controller/ ./model/ -run 'OpenCodeGo|Ratio|ModelNames' -count=1` 全部通过。

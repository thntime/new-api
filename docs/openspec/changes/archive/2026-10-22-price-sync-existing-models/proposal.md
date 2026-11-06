# Proposal

## Why

上一个 change（`opencode-sync-provider-fallback`）让 OpenCode Go 来源覆盖全部 provider 段的模型后，价格同步会把上游目录里的数千个模型都列为差异，其中绝大多数在本地模型库中并不存在，真实需要关注的模型被淹没。

同时，来源优先级只按 provider 段名升序决定，`opencode` 官方段并不固定排在 `opencode-go` 之后。

## What Changes

- **价格同步只处理模型库中已有的模型**：`FetchUpstreamRatios` 在计算差异后，按本地模型库（`models` 表的 `model_name`）过滤差异集合；模型库中不存在的上游模型不再出现在 `differences` / `prices` 中。该过滤对全部价格同步来源（opencode-go 预设、models.dev、官方、自定义渠道）统一生效。
- **opencode 目录来源优先级**：解析 `models.opencode.ai/api.json` 时按 `opencode-go` → `opencode`（官方段）→ 其余 provider 段（段名升序）的顺序选择条目与补齐字段。
- 新增模型库查询：`model.GetAllModelNames()`，供价格同步读取本地模型名集合。

## Capabilities

### New Capabilities

- `model-price-sync`: 价格同步的候选范围与来源处理规则，包括只同步本地模型库中已有模型、上游模型集合与差异集合的关系。

### Modified Capabilities

- `opencode-catalog`: 新增来源优先级要求（opencode-go 优先，opencode 官方次之，其余 provider 段按段名升序）。

## Impact

- 后端：`controller/ratio_sync.go`（差异过滤）、`service/opencodego_models.go`（段遍历顺序）、`model/model_meta.go`（新增 `GetAllModelNames`）。
- 测试：`service/opencodego_test.go`（优先级用例）、`controller/ratio_sync_test.go` 或等价位置（过滤用例）。
- 不改变接口形状：`POST /api/ratio_sync/fetch` 的响应结构不变，只是 `differences` / `prices` 的模型集合被收窄。
- 模型同步（元数据）不受本次过滤影响，仍覆盖全量目录。

# Tasks

## 1. 兜底渠道池配置

- [x] 1.1 在 `setting/operation_setting/channel_affinity_setting.go` 的 `ChannelAffinitySetting` 新增兜底渠道 ID 列表字段（JSON 数组，默认空），并在 `model/request_policy.go` 的白名单与默认值中登记对应扁平 option 键。验证：`go build ./...` 通过，且在 `model/request_policy_test.go` 中新增用例断言该键被接受、非法取值被拒绝、保存后重新加载能还原列表。 <!-- comet-task:d8123b0e-0be7-4223-9a00-61e45e1c065d -->
- [x] 1.2 在 `web/src/features/system-settings/request-policies/` 的 `routing-form.ts`（zod 校验与序列化）、`defaults.ts` 与设置区组件中新增兜底渠道配置项，并在 `web/src/i18n/locales/*.json` 七个语言包补齐文案。验证：`cd web && bun run typecheck` 通过，且 `request-policies/__tests__/` 下新增用例断言该字段可编辑、保存与回显。 <!-- comet-task:a2c14028-fee4-47f0-b424-d982b54918ff -->

## 2. 兜底候选选择（模型层）

- [x] 2.1 在 `model/channel_cache.go` 的 `getRandomSatisfiedChannel` 收集候选时排除兜底渠道池中的渠道，使初始选择与重试选择都不再把兜底渠道当作正常优先级层候选。验证：在 `model` 现有测试中新增用例，断言同时配置正常与兜底渠道时正常层选择结果不含兜底渠道，且未配置兜底池时候选集合与改动前一致。 <!-- comet-task:eeef2a95-3ad7-43ee-98d3-a23c014a9a67 -->
- [x] 2.2 在 `model/channel_cache.go` 新增兜底专用选择入口：按兜底渠道自身优先级降序定位到指定层索引，层内按权重加权随机，并复用 `filterCandidateIDs` / `ChannelSatisfiesFilters` 做分组、模型与请求约束过滤。验证：新增表驱动用例断言层序为优先级降序、同层多台渠道在多次调用中命中不同渠道、不满足约束的渠道被跳过。 <!-- comet-task:3ae628c6-dc83-409b-b890-12b238f47435 -->

## 3. 阶段串联（服务层与控制器）

- [x] 3.1 在 `service/channel_select.go` 新增请求内兜底阶段状态与选择助手：`FallbackStageActive`、`EnterFallbackStage`、`NextFallbackChannel`（按兜底层索引推进并返回该层渠道，返回 nil 表示兜底池为空或层已耗尽），以及解析兜底选择所用分组（非 auto 用令牌分组，auto 用已选分组，尚未选中分组时取第一个 auto 分组）。验证：新增用例断言首次调用返回兜底池最高优先级层渠道并把层指针推进到下一层、层耗尽后返回 nil、同一请求内不重复返回同一层、未配置兜底池时始终返回 nil。 <!-- comet-task:b15d1e8e-a09f-4ea5-80ad-b4c6cd40def5 -->
- [x] 3.2 在 `controller/relay.go` 把一次渠道尝试主体抽成 HTTP 主循环与任务提交循环共用的执行单元，并在两者正常重试循环结束后追加独立的兜底阶段循环（循环条件、预算入参与停止判定保持不变）：兜底阶段按 3.1 的层指针逐层推进、每层尝试一次、不计入 `RetryTimes`；渠道 pin 请求不进入兜底阶段；每层失败记录 `fallback_retry` 决策事件，兜底池为空或全部被跳过时记录一条 `fallback_exhausted` 决策事件。验证：新增用例断言正常阶段失败后按兜底渠道自身优先级降序继续尝试、重试预算用尽后仍进入兜底、pin 请求不进入兜底，且既有 `DecideRelayRetry` 的调用次数与停止原因与改动前一致。 <!-- comet-task:6d07ba4e-4eb7-44c7-bbd4-c6ce8597a174 -->
- [x] 3.3 在 `service/channel_select.go` 的 `SelectChannelForRequest` 中让首轮选择支持兜底阶段：`strict` 会话亲和渠道不可用时、以及正常候选池为空时，若存在满足本次请求约束的兜底渠道则改用兜底渠道并推进层指针，否则保持既有 503 错误；`service/relay_error.go` 不修改。验证：在 `service/relay_error_test.go` 的表驱动用例中新增「存在兜底渠道时 `strict` 失败后仍可继续尝试」「无兜底池时停止原因与改动前一致」「无兜底池时 `strict` 仍直接终止」三组断言，并在新增用例中断言 `strict` 亲和不可用且有兜底渠道时不再返回 503。 <!-- comet-task:50ad0854-3571-4277-94b6-51efd69d40cc -->
- [x] 3.4 在 `relay/responses_websocket.go` 的重试循环后追加与 3.2 同形态的兜底阶段循环：抽出一轮尝试闭包，正常阶段以失败结束后按兜底层推进重试，成功则结束；渠道 pin 请求不进入兜底阶段。验证：新增用例断言 WS 路径在正常阶段失败后使用兜底渠道重试，且未配置兜底池时该路径行为与改动前一致。 <!-- comet-task:83d061c8-fc86-4ff0-a23d-75796b1b8415 -->

## 4. 亲和缓存与锚点隔离

- [x] 4.1 在 `service/channel_affinity.go` 的 `RecordChannelAffinity` 与 `rememberDescentAnchor` 入口按渠道身份判定：渠道属于兜底渠道池时直接返回，不写亲和缓存、不更新会话绑定、不成为 descent 环形锚点（`middleware/distributor.go` 不改动）。验证：新增用例断言兜底渠道成功后的亲和缓存内容与锚点均未变化，且正常段渠道成功仍照常写入。 <!-- comet-task:129c1e2f-d22a-44cb-97e6-9d50a586273c -->

## 5. 集成验证

- [x] 5.1 覆盖验收场景①的端到端尝试顺序：候选池优先级层 10/20/30/40/50、重试次数 5、兜底渠道 66（优先级 6）与 55（优先级 5）全部失败时，尝试顺序为 50、40、30、20、10、50、66、55。验证：新增用例断言完整顺序，并在手工联调中核对 `use_channel` 重试链路日志与顺序一致。 <!-- comet-task:63febe43-cc60-4404-836d-196983a14a23 -->
- [x] 5.2 覆盖剩余边界场景：正常层任一渠道成功时不触碰兜底、正常候选池为空时使用兜底渠道、未配置兜底池时 `descent` 环形回绕行为与改动前逐位一致、渠道 pin 请求失败后不进入兜底段。验证：新增用例逐条断言，并确认既有 `service/channel_select_auto_groups_test.go` 与 `service/channel_affinity_template_test.go` 全部通过。 <!-- comet-task:13fe283b-2212-4c36-9abc-172cb173bac7 -->
- [x] 5.3 覆盖「`auto` 分组下所有分组跑完后只执行一次兜底」的验收场景（最终集成审查 F1）：令牌分组为 `auto` 且解析出多个分组、各分组正常候选按优先级层降序全部失败后，兜底阶段在所有分组跑完后只执行一次，并按进入兜底阶段时的当前分组过滤兜底渠道。验证：新增端到端用例（真实 `Relay` 主循环 + `auto` 分组夹具 + 兜底池）断言兜底渠道在每个兜底优先级层仅出现一次、且所有兜底尝试都发生在最后一个正常分组尝试之后；无兜底池时该场景不出现任何兜底渠道。 <!-- comet-task:dc0f1b47-f75c-403c-9744-f6d061b8779b -->

## Workflow follow-up

- 完成 review 后归档本 change。
- 验证归档结果。

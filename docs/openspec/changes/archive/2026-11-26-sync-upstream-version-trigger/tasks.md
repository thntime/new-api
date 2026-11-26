# Tasks

## 1. 版本门禁实现

- [x] 1.1 修改 `.github/workflows/sync-upstream.yml`：`Fetch upstream` 增加 `--tags`，把 `Check if upstream already merged` 改为 `Check upstream version gate`，按设计解析最新 `v*` tag、排除 12 位时间戳后缀的 fork 发布 tag，并输出 `skipped`；验收：用临时 git 仓库运行内联脚本，覆盖「最新版本已在 HEAD / 上游新增版本 / 只有新提交 / 无版本 tag / 手动触发」五种输入，输出与设计一致 <!-- comet-task:06e6e408-0a2e-4f6a-945f-496eaab9d92d -->
- [x] 1.2 更新 workflow 顶部注释、`version` 失败类型的 issue 标题与处理指引；验收：`version` 失败时 issue 标题与指引包含版本解析线索，`auth` / `conflict` / `vet` / `push` 文案保持不变，且 workflow 通过 YAML 解析 <!-- comet-task:cdb367af-4b8f-4f1f-8fa6-9ba1c52a8cb3 -->

## 2. 规格与文档

- [x] 2.1 更新 `docs/openspec/specs/upstream-sync/spec.md` 的 Purpose 为「上游发布新版本时同步」；验收：Purpose 与 delta spec 的版本门禁语义一致 <!-- comet-task:6d634802-c325-4941-a318-f5e0281fd2f4 -->
- [x] 2.2 校验 delta spec 与 workflow 行为一致；验收：`comet classic openspec -- validate sync-upstream-version-trigger --strict` 通过 <!-- comet-task:7939d13c-7765-4b46-b5ff-d9620efbc2a3 -->

## Workflow follow-up

- 归档后由用户决定是否推送并观察下一次定时运行或手动 `workflow_dispatch`。
- issue #24 的待合并冲突不在本 change 范围；可在上游发布下一个版本时处理，或手动 dispatch 强制同步。

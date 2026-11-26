# Spec Delta

## MODIFIED Requirements

### Requirement: 上游提交三方合并

当版本门禁判定需要同步时，同步任务 SHALL 使用三方合并把 `upstream/main` 并入本仓库 `main`，且 MUST NOT 使用 `-X ours` 或整侧覆盖来掩盖冲突。版本门禁判定不需要同步时，MUST NOT 合并。

#### Scenario: 上游发布新版本

- **WHEN** 上游最新 `v*` 版本 tag 的提交不在本地 `HEAD` 中
- **THEN** 任务执行三方合并，把该 tag 及其后的上游提交并入 `main`

#### Scenario: 只有新提交、版本未变

- **WHEN** `upstream/main` 包含本地 `HEAD` 尚未包含的提交，但上游最新版本 tag 已是 `HEAD` 祖先
- **THEN** 任务不合并、不推送，并以成功状态报告等待下一个上游版本

#### Scenario: 无冲突合并

- **WHEN** 上游新增提交与本地改动不重叠
- **THEN** 合并自动完成并生成合并提交

#### Scenario: 存在冲突

- **WHEN** 三方合并产生未解决冲突
- **THEN** 任务中止合并、报告冲突文件清单，并且不推送任何结果

### Requirement: 失败原因可区分

同步任务 SHALL 在失败上报中区分合并冲突、vet 失败、推送权限失败、凭证缺失与版本解析失败，并给出对应的处理线索。

#### Scenario: 推送权限失败

- **WHEN** 推送被远端以权限原因拒绝
- **THEN** 上报内容标明这是凭证权限问题，而不是冲突无法自动解决

#### Scenario: 版本解析失败

- **WHEN** 无法从 `upstream/main` 解析出最新的 `v*` 版本 tag
- **THEN** 上报内容标明版本解析失败，并提示检查上游 tag 与 fetch 配置

## ADDED Requirements

### Requirement: 上游版本门禁

同步任务 SHALL 在合并前解析上游 `main` 上最新的 `v*` 版本 tag，并仅在该 tag 的提交不是 `HEAD` 祖先时继续自动合并。定时触发 MUST 遵守该门禁；手动 `workflow_dispatch` 视为明确要求同步，SHALL 跳过该门禁。带 12 位时间戳后缀的 fork 发布 tag MUST NOT 参与上游版本判定。

#### Scenario: 最新版本已同步

- **WHEN** 定时触发且上游最新版本 tag 的提交已是 `HEAD` 祖先
- **THEN** 任务跳过合并与推送，以成功状态结束，并报告等待下一个上游版本

#### Scenario: 上游出现新版本

- **WHEN** 定时触发且上游最新版本 tag 的提交不在 `HEAD` 中
- **THEN** 任务继续执行三方合并

#### Scenario: 手动触发

- **WHEN** 通过 `workflow_dispatch` 手动触发
- **THEN** 任务跳过版本门禁并执行同步，即使最新版本 tag 已在 `HEAD` 中

#### Scenario: 无法解析版本

- **WHEN** 无法从 `upstream/main` 解析出 `v*` 版本 tag
- **THEN** 任务以版本解析失败结束，不执行合并

#### Scenario: fork 发布 tag 不参与判定

- **WHEN** 仓库存在形如 `v1.0.0-rc.51-202611211222` 的 fork 发布 tag
- **THEN** 该 tag 不被当作上游版本，门禁只依据上游 `main` 可达的 `v*` tag

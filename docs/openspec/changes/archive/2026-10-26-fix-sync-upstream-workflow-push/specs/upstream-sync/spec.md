# Spec Delta

## Purpose

定义每日把上游 `QuantumNous/new-api` 的 `main` 同步进本仓库 `main` 的自动化行为，包括合并策略、推送前构建关卡、推送凭证要求、失败上报与短路条件。

## ADDED Requirements

### Requirement: 上游提交三方合并

当 `upstream/main` 包含本地 `HEAD` 尚未包含的提交时，同步任务 SHALL 使用三方合并把上游提交并入本仓库 `main`，且 MUST NOT 使用 `-X ours` 或整侧覆盖来掩盖冲突。

#### Scenario: 无冲突合并

- **WHEN** 上游新增提交与本地改动不重叠
- **THEN** 合并自动完成并生成合并提交

#### Scenario: 存在冲突

- **WHEN** 三方合并产生未解决冲突
- **THEN** 任务中止合并、报告冲突文件清单，并且不推送任何结果

### Requirement: 推送前构建关卡

合并成功后，同步任务 SHALL 在根模块和 `relaykit` 分别执行 `GOWORK=off go vet ./...`，只有两者都通过才允许推送。

#### Scenario: 双模块 vet 通过

- **WHEN** 根模块与 `relaykit` 的 vet 均以退出码 0 结束
- **THEN** 任务继续执行推送

#### Scenario: vet 失败

- **WHEN** 任一模块的 vet 返回非零退出码
- **THEN** 任务丢弃未推送的合并结果、报告 vet 日志，并且不推送

### Requirement: 可更新 workflow 文件的推送凭证

同步任务 SHALL 使用具备 `contents: write` 与 `workflows: write` 权限的凭证执行 checkout 与 push，使包含 `.github/workflows/` 变更的合并提交能够推送成功。

#### Scenario: 合并包含 workflow 变更

- **WHEN** 待推送的合并提交修改了 `.github/workflows/` 下的文件
- **THEN** 推送成功，不因凭证缺少 `workflows` 权限被拒绝

### Requirement: 推送凭证预检

同步任务 SHALL 在合并前检查专用凭证是否存在；凭证缺失或为空时 MUST 立即失败并输出可操作的配置提示，而不是进入合并或推送阶段。

#### Scenario: 凭证未配置

- **WHEN** 专用凭证 secret 缺失或为空
- **THEN** 任务在合并前失败，错误信息指明需要配置的 secret 名称与所需权限

### Requirement: 失败原因可区分

同步任务 SHALL 在失败上报中区分合并冲突、vet 失败和推送权限失败，并给出对应的处理线索。

#### Scenario: 推送权限失败

- **WHEN** 推送被远端以权限原因拒绝
- **THEN** 上报内容标明这是凭证权限问题，而不是冲突无法自动解决

### Requirement: 上游已包含时短路

当 `upstream/main` 已经是 `HEAD` 的祖先时，同步任务 SHALL 跳过合并与推送并以成功状态结束。

#### Scenario: 上游已同步

- **WHEN** `git merge-base --is-ancestor upstream/main HEAD` 成立
- **THEN** 任务不产生新提交、不推送，并报告无需同步

### Requirement: rerere 种子持久化

同步任务 SHALL 把人工解决冲突产生的 rerere 记录从 `.git/rr-cache` 回填到仓库内的 `.github/rerere-cache/`，使后续运行可以自动重放同一冲突的解决方案。

#### Scenario: 人工解决冲突后回填

- **WHEN** 合并提交包含人工解决的冲突
- **THEN** 对应 rerere 记录被提交到 `.github/rerere-cache/`，并在后续运行前恢复到 `.git/rr-cache`

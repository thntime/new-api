# 设计：sync-upstream 按上游版本门禁触发

## 背景与目标

`sync-upstream` 每日把上游 `main` 的全部新提交合并进本 fork，普通提交也会触发合并与冲突处理（issue #24）。本设计把自动合并的触发条件改为「上游出现新的 `v*` 版本 tag」，定时触发在版本未变时短路；手动触发保留强制同步能力。需求与验收以 `proposal.md` 和 delta spec 为准。

## 版本判定

- fetch：`git fetch upstream main --tags`，确保 `upstream/main` 可达的 tag 已在本仓库。
- 解析：

  ```bash
  git tag --merged upstream/main --list 'v*' --sort=-v:refname \
    | grep -Ev -- '-[0-9]{12}$' \
    | head -n 1
  ```

  - `--merged upstream/main` 只保留上游 `main` 可达的 tag，位于 fork 合并提交上的 fork 发布 tag 自然被排除。
  - `grep -Ev -- '-[0-9]{12}$'` 额外排除形如 `v1.0.0-rc.51-202611211222` 的 fork 发布 tag，避免 fork tag 被上游包含时误判。
  - `--sort=-v:refname` 取版本序最高者；`v1.0.0-rc.51` 高于 `v1.0.0-rc.9` 与 `v0.x`。
- 门禁：`git merge-base --is-ancestor "${TAG}^{commit}" HEAD` 成立 → 已同步，短路；否则 → 合并 `upstream/main`。
- 解析为空 → 失败类型 `version`，不合并。

## workflow 改动

- `Fetch upstream` 增加 `--tags`。
- 步骤 `Check if upstream already merged` 改为 `Check upstream version gate`：
  - `workflow_dispatch` → `skipped=false`，跳过门禁。
  - 否则解析 tag 并判断祖先关系，输出 `skipped`。
  - 解析失败时写入 `/tmp/failure-kind` 为 `version` 并以非零退出。
- 顶部注释、失败标题与处理指引增加 `version` 类型。
- 其余步骤（merge / vet / rerere / push / issue）保持不变。

## 取舍

- 不使用 GitHub Releases API：共享 runner 易限流，且本地 tag 已能表达版本；与 `release.yml` 的 `v1.0.0-rc.*` 版本口径一致。
- 不记录「上次同步版本」状态文件：tag 祖先关系已能表达「该版本是否已合并」，无需额外状态。
- 手动触发保留为显式绕过：日常自动同步收敛到版本边界，但仍可在需要时立即复核或追赶。
- 版本 tag 只看 `upstream/main` 可达者：只合并 `main`，不合并游离 tag 或 release 分支。

## 验证

用临时 git 仓库模拟门禁脚本的五种输入：

1. 最新 tag 已在 `HEAD` → `skipped=true`；
2. 上游新增 tag 未在 `HEAD` → `skipped=false`；
3. 只有新提交、无新 tag → `skipped=true`；
4. 无 `v*` tag → 退出码非零且写出 `version` 失败标记；
5. `workflow_dispatch` → `skipped=false`。

再用 YAML 解析器校验 workflow 语法；不实际触发 GitHub Actions，推送后由下一次定时运行或手动 dispatch 观察。

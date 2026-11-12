# 设计：解决 Sync Upstream 合并冲突（issue #21）

## 背景与目标

`Sync Upstream`（run [42960292980](https://github.com/thntime/new-api/actions/runs/42960292980)）在 `git merge --no-edit upstream/main` 步骤因 8 个冲突文件失败，issue #21 待人工处理。本设计在本地 `main`（`ed9175de8`）上完成 `upstream/main`（`be5e3153d`，merge-base `2de51a19e`）的三方合并，冲突按两侧语义融合，验证通过后推送并恢复 workflow。需求与验收以 `proposal.md` 为准。

## 冲突一：7 个 locale JSON（内容冲突）

### 事实

- 文件：`web/src/i18n/locales/{en,fr,ja,ru,vi,zh-TW,zh}.json`，各 5 处冲突。
- 三方比对（`translation` 层）：base 7392 键、本地 7676 键、上游 7410 键；本地新增 284、上游新增 18，两侧同增键 0 个，**同名键取值冲突 0 个**，并集 7694 键。
- `web/scripts/sync-i18n.mjs`：以叶子键最多的 `en.json` 为基准，把 7 个语言按 `en` 键序重排；`stableStringify` 使用 2 空格缩进、末尾换行，并把 `footer.newapi.projectAttributionSuffix` 序列化为 `footer.new\u0061pi.projectAttributionSuffix`；缺失键会从 `en` 回填，因此必须先让每个语言各自包含上游 18 个键，再归一顺序。

### 采用方案：逐语言键集并集 + 脚本归一

1. 对每个语言取 base/ours/theirs 的 `translation` 对象，按 `ours` 键序输出并集；`theirs` 独有键插入到其在 `theirs` 中的前一个键之后（前一个键不存在时置于开头），减少与上游的 diff 噪声。
2. 断言两侧同增键无取值冲突（有冲突则停止并报告）。
3. 写入后运行 `bun run i18n:sync`（在 `web/`），由脚本统一为 `en` 键序与 `stableStringify` 格式；验收标准是再次运行该命令工作区无变化，`_reports/_sync-report.json` 中 7 个语言 `missingCount=0`、`extrasCount=0`。

未采用「先只解 `en` 再让脚本回填其余语言」：会把上游 18 个键的英文原文写进 6 个非英文语言，丢失上游译文。未采用「整文件取上游/本地」：会丢弃另一侧的 284 或 18 个键。

## 冲突二：`channel-mutate-drawer.tsx`（内容冲突 3 处）

### 事实

- 本地该文件不含 `resetModelMappingSync`；上游 `be5e3153d` 之前的提交新增了该函数（定义在文件内 `useCallback`）并在「Load channel data into form」effect 的 3 个分支调用：`!open` 分支、`form.reset(defaults)` 之后（传 `defaults.model_mapping, defaults.models`）、`else if (!isEditing)` 分支，同时加入 effect 依赖数组。
- 本地在同一 effect 内保留：`!open` 分支的 `setDoubaoAccessMode('standard')` 与 `setConfirmDiscardOpen(false)`；`form.reset(defaults)` 之后的 VolcEngine(45) 接入方式回显（`coding` / `standard` / `custom`，含存量魔法键写回 `endpoint_profile` 与 `base_url` 归一）、Zhipu V4(26)/Moonshot(25) 的 `endpoint_profile` 反推。

### 采用方案：两侧语义都保留

- 以**上游结构**为骨架：`!open` 分支保留上游 `resetModelMappingSync()`，同时保留本地 `setDoubaoAccessMode('standard')` 与 `setConfirmDiscardOpen(false)`；`form.reset(defaults)` 后先执行上游 `resetModelMappingSync(defaults.model_mapping, defaults.models)`，再执行本地 VolcEngine/Zhipu 回显块；`else if (!isEditing)` 分支同时保留 `setDoubaoAccessMode('standard')` 与 `resetModelMappingSync()`。
- effect 依赖数组取上游版本（含 `resetModelMappingSync`），本地新增逻辑不引入新的依赖项。
- 不合并「Handle type change」相关 effect：该段未报冲突，按 git 自动合并结果处理，若出现重复实现再以实际 diff 复核。

## 验证

1. 合并后 `git diff --name-only --diff-filter=U` 为空，8 个文件均无 `<<<<<<<` / `>>>>>>>` 标记。
2. 根模块、`relaykit`、`tokenkit`：分别 `GOWORK=off go vet ./...` 退出码 0（workflow 同款关卡为根模块 + `relaykit`；`tokenkit` 为项目约定要求的独立模块校验）。
3. 前端：`bunx tsc -b web/tsconfig.json` 通过（`bun check -b` 在当前 bun 1.4 不可用）；运行 `bun --cwd web run test` 中与渠道抽屉相关的测试文件。
4. `bun run i18n:sync` 幂等（工作区无变化）。
5. `go vet` 需要 `web/dist/index.html` 占位文件（`main.go` 的 `//go:embed web/dist`），CI 也是这么做的；本地若无该目录，创建占位并在验证后移除未跟踪占位，不提交。

## rerere 种子回填

`sync-upstream.yml` 以 `.github/rerere-cache/` 为唯一事实源，CI 无法自动收录人工解决方案：

1. 合并前确认 `git config rerere.enabled true`（本地已开启）与 `rerere.autoupdate true`。
2. 解决冲突并 `git add` 后，`.git/rr-cache` 会新增对应记录。
3. 合并提交后 `cp -r .git/rr-cache/* .github/rerere-cache/`，把种子折入合并提交（`git commit --amend --no-edit`）或单独提交，确认两目录无差异。

## 交付

推送 `origin/main`（PAT 推送会触发 `build-image.yml` 重建镜像），关闭 issue #21，并触发一次 `workflow_dispatch` 复核 sync-upstream 在「Check if upstream already merged」短路成功。

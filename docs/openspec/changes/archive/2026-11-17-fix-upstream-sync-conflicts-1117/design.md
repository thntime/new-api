# 设计：解决 Sync Upstream 合并冲突（issue #22）

## 背景与目标

`Sync Upstream` 在 `git merge --no-edit upstream/main` 步骤因 8 个冲突文件失败，issue #22 待人工处理。本设计在本地 `main`（`fba0c90c1`）上完成 `upstream/main`（`669d77497`，merge-base `0bda086fb`）的三方合并，冲突按两侧语义融合，验证通过后推送并恢复 workflow。需求与验收以 `proposal.md` 为准。

## 冲突一：`AGENTS.md`

### 事实

- base 与 theirs 均为 192 行；ours 为 212 行。
- ours 相对 base 仅新增文件末尾的 `<comet-ambient-resume>` 块（20 行，由 Comet 管理）。
- theirs 相对 base 只改了 Issues 与 Pull requests 两段：把冗长的内联说明替换为「先执行 `.agents/github/CHECKS.md` 的全部检查，再按该文件 Writing 段填写 `.agents/github/ISSUE.md` / `.agents/github/PR.md`」。上游同时新增了 `.agents/github/CHECKS.md`（随本次合并进入工作区）。

### 采用方案：以上游新版为骨架 + 保留本地 Comet 块

1. 取上游 `AGENTS.md` 全文作为主体（Issues / Pull requests 使用新版文字）。
2. 在文件末尾追加本地 `<comet-ambient-resume>` 块，保留 Comet 恢复协议。
3. 不保留 ours 中已被上游替换的旧 Issues / PR 内联说明，避免与 `CHECKS.md` 形成双重口径。

## 冲突二：7 个 locale JSON

### 事实

- 键集：base 7728、ours 8078（+350）、theirs 7782（+58），两侧同增键 0 个，并集 8136 键。
- 每个语言的键集都与其 `en` 完全一致；`en` 两侧共有键取值冲突 0 个。
- 非 `en` 语言共有键取值冲突：fr 7 个，ja/ru/vi/zh-TW/zh 各 8 个，集中在缓存计费文案（`Cache create (…h) price`、`Cache Write (…)`、`Cache read price`）与合规提示（`Payment, … root administrator …`）。
- `web/scripts/sync-i18n.mjs` 以叶子键最多的 `en.json` 为基准，把所有语言按 `en` 键序重排；目标语言中不在 `en` 的键会写入 `_extras` 并从产物中剔除，缺失键从 `en` 回填。因此每个语言必须与 `en` 拥有相同键集，且 `extrasCount=0` 才是稳定状态。

### 采用方案：逐语言键集并集 + 冲突键取上游 + 脚本归一

1. 对每个语言，按 `ours` 键序输出并集：ours 独有键保留，theirs 独有键按其在 theirs 中的前一个键之后插入（前一个键不存在时置于开头），减少与上游的 diff 噪声。
2. 共有键：取值相同则直接使用；取值冲突时采用 **theirs（上游）** 译文。理由：上游在同一批次统一了缓存计费术语（`bb30732b8`）并把合规提示改为 `super administrator`（`87c8848f3`），属于上游有意的文案修正，保留本地旧译会与新 UI 语义不一致。
3. 写入后运行 `bun run i18n:sync`（在 `web/`），由脚本统一为 `en` 键序与 `stableStringify` 格式。验收：再次运行该命令工作区无变化，`_reports/_sync-report.json` 中 7 个语言 `missingCount=0`、`extrasCount=0`。

未采用「整文件取上游/本地」：会丢失本地 350 个键或上游 58 个键。未采用「只解 en 再让脚本回填非 en」：会把英文原文写入 6 个非英文语言，丢失上游译文。

## 验证

1. 合并后 `git diff --name-only --diff-filter=U` 为空，8 个文件均无 `<<<<<<<` / `>>>>>>>` 标记。
2. 根模块、`relaykit`、`tokenkit`：分别 `GOWORK=off go vet ./...`，退出码 0（workflow 关卡为根模块 + `relaykit`；`tokenkit` 为项目约定要求的独立模块校验）。
3. 前端：`bunx tsc -b web/tsconfig.json` 通过（`bun check -b` 在当前 bun 1.4 不可用）；运行与渠道抽屉/渠道配置相关的测试文件。
4. `bun run i18n:sync` 幂等（工作区无变化）。
5. `go vet` 需要 `web/dist/index.html` 占位文件（`main.go` 的 `//go:embed web/dist`），本地若缺失则创建占位，验证后移除未跟踪占位，不提交。

## rerere 种子回填

`sync-upstream.yml` 以 `.github/rerere-cache/` 为唯一事实源，CI 无法自动收录人工解决方案：

1. 合并前确认 `git config rerere.enabled true` 与 `rerere.autoupdate true`。
2. 解决冲突并 `git add` 后，`.git/rr-cache` 会新增对应记录。
3. 合并提交后 `cp -r .git/rr-cache/* .github/rerere-cache/`，把种子折入合并提交（`git commit --amend --no-edit`），确认两目录 `diff -r` 无差异。

## 交付

推送 `origin/main`，关闭 issue #22，并触发一次 `workflow_dispatch` 复核 sync-upstream 在「Check if upstream already merged」短路成功。交付不在 build 任务范围，由归档阶段执行并记录。

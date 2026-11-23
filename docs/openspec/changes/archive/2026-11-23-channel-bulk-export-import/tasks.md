# Tasks

## 1. 后端导出

- [x] 1.1 在 `model/channel.go` 增加 `GetChannelsByIds(ids []int) ([]*Channel, error)`：用 GORM 批量查询并包含 key，返回前按传入 ids 顺序重排；验证：`go build ./...` 通过，且 1.3 的导出测试能按请求顺序取到渠道。 <!-- comet-task:ec75e0ad-7b7f-4ecf-b107-7684c018dfc1 -->
- [x] 1.2 在 `controller/channel_export.go` 实现 `.nacx` 信封：scrypt 派生密钥、AES-256-GCM 加解密、格式/版本/AAD 校验，以及导出 DTO 的字段裁剪、敏感 settings 解密、channel_info 裁剪、状态 3→1；验证：在 1.4 的测试文件里先写信封往返与篡改失败子测试，跑 `go test ./controller -run '^TestChannelExportImport$/^envelope/' -count=1 -v` 通过。 <!-- comet-task:fcb8a4e6-b724-40a7-878b-f8284f6d22f1 -->
- [x] 1.3 实现 `ExportChannels`（POST `/api/channel/export`，body `{ids,password}` → `{filename,content}`），加入 `channelPermissionRoutes`（`authz.ChannelSensitiveWrite`），加 `channel.export` 审计模板（只记数量），响应加 `Cache-Control: no-store`；验证：`go build ./...` 通过，1.4 的导出子测试通过。 <!-- comet-task:5dcc36ad-73c1-45f8-86ec-ca605b537e91 -->
- [x] 1.4 在 `controller/channel_export_test.go` 写导出子测试：3 个渠道按请求 ids 顺序输出、明文无 id、无运行时字段、敏感 settings 已解密、channel_info 只含配置字段；验证：`go test ./controller -run '^TestChannelExportImport$/^export/' -count=1 -v` 通过且子测试都执行。 <!-- comet-task:6c7dcc08-5407-46eb-889e-c456f54a0a34 -->

## 2. 后端导入

- [x] 2.1 在 `controller/channel_export.go` 实现 `ImportChannels`（POST `/api/channel/import`，body `{password,content}` → `{count,ids}`）：解密、解析、逐条构造渠道、用 `mergeSensitiveOtherSettings` 重新加密 settings、重算 multi_key_size、created_time 取当前时间、运行时字段清零、状态 3→1；验证：`go build ./...` 通过，2.3 的导入子测试通过。 <!-- comet-task:112d7f58-aa94-4c90-9466-7a3a9c40242b -->
- [x] 2.2 导入校验与整批回滚：复用 `validateChannel`、任务插件权限/存在性、request_plugins 存在性、空 key 规则；任一失败返回文件序号与渠道名，不插入；全部通过后按数组顺序调 `model.BatchInsertChannels`，成功后 `model.InitChannelCache()`；加 `channel.import` 审计模板（只记数量）；验证：`go build ./...` 通过，2.3 的非法项回滚子测试通过。 <!-- comet-task:dd7091a1-1d4f-4e0f-9be5-b730a7ab2760 -->
- [x] 2.3 在 `controller/channel_export_test.go` 写导入子测试：正确密码后新 id 严格递增、配置字段一致、created_time 变新、状态 3 变 1；错误密码失败且渠道数不变；密文改一字节失败且不落库；文件第 2 条非法时整批回滚；敏感 settings 重新加密后可解密；验证：`go test ./controller -run '^TestChannelExportImport$/^import/' -count=1 -v` 通过且子测试都执行。 <!-- comet-task:425afb95-64b0-4c03-821a-da0ed682854d -->

## 3. 前端

- [x] 3.1 在 `web/src/features/channels/api.ts` 增加 `exportChannels` / `importChannels` 请求，在 `types.ts` 增加返回类型；验证：`cd web && ./node_modules/.bin/tsgo -b` 退出 0。 <!-- comet-task:fef7e01f-06af-49c6-9340-96f37e9da59d -->
- [x] 3.2 新建 `web/src/features/channels/lib/channel-export.ts`：把导出内容转 Blob 并触发 `.nacx` 下载，读取导入文件文本；验证：3.4 的前端测试覆盖下载与文件读取。 <!-- comet-task:3bd7b375-8fb5-4025-9499-59b8d21c4e4d -->
- [x] 3.3 新建 `ExportChannelsDialog` 与 `ImportChannelsDialog`：导出输入密码后下载，导入选文件加密码后提交；批量操作栏加导出按钮（无选中不可点），「更多操作」加「导入渠道」（无敏感写权限置灰）；成功后刷新列表并提示数量；密码框 `type=password`，关闭即清空，不写 localStorage；验证：3.4 的前端测试通过。 <!-- comet-task:d6d894bc-1301-4f2f-8e70-ca5d0034d24f -->
- [x] 3.4 新建 `web/src/features/channels/components/__tests__/channel-export-import.test.tsx`：覆盖导出请求 ids 顺序与密码、下载触发、导入文件读取与提交、错误密码提示、无选中/无权限禁用；验证：`cd web && ./node_modules/.bin/vitest run src/features/channels/components/__tests__/channel-export-import.test.tsx` 通过。 <!-- comet-task:74d356ba-e5b5-4ec5-a39a-e4cd12d579fd -->

## 4. 文案与集成验证

- [x] 4.1 在 `web/src/features/usage-logs/lib/format.ts` 增加 `channel.export` / `channel.import` 展示模板；新增界面文案用 `t('英文键')`，后端错误键手动补进 7 个语言文件；跑 `cd web && bun run i18n:sync`；验证：`git diff --stat` 只含 7 个语言文件，再跑一次无新改动。 <!-- comet-task:f303c403-a093-4bc9-a17a-a831ffdb3fdd -->
- [x] 4.2 集成检查：`go build ./...` 退出 0；`go test ./controller -run '^TestChannelExportImport$' -count=1 -v` 通过且子测试都执行；`cd web && ./node_modules/.bin/tsgo -b` 退出 0；`cd web && ./node_modules/.bin/oxlint -c .oxlintrc.json <改动的前端文件>` 无 error；`cd web && ./node_modules/.bin/vitest run src/features/channels/components/__tests__/channel-export-import.test.tsx` 通过。 <!-- comet-task:42a21ff7-8805-475c-9d12-5a8cd3891e61 -->
- [x] 4.3 反向验证：把导入顺序临时改成倒序，跑顺序子测试贴失败输出，还原后贴全绿；把错误密码子测试的断言临时删掉，跑一次贴失败，还原后贴全绿。 <!-- comet-task:1a6d0d1b-e287-4710-84cf-49c01a277065 -->
- [x] 4.4 核对没有把密码、渠道 key 或明文导出内容写进日志、审计、测试快照或对话；验证：`git diff` 中审计参数只有数量，测试输出不含真实密钥。 <!-- comet-task:f2eea81f-04ba-4238-9e33-0ff5bfeb9b11 -->

## Workflow follow-up

- 项目审查要求满足后归档该 change。
- 归档后核对归档结果与交付记录。

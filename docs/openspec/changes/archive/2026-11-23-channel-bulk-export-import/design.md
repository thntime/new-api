---
comet_change: channel-bulk-export-import
role: technical-design
canonical_spec: openspec
archived-with: 2026-11-23-channel-bulk-export-import
status: final
---

# Design

## Context

- 渠道列表通过 `GetAllChannels` / `SearchChannels` 读取，列表响应用 `Omit("key")` 隐藏密钥；完整渠道需按 id 另查（`model.GetChannelById(id, true)`）。
- 新建渠道走 `controller.AddChannel` → `model.BatchInsertChannels`：事务、每 50 条一批、回填自增 id、同时写 abilities。
- 渠道路由集中在 `router/channel-router.go` 的 `channelPermissionRoutes`，权限使用 `service/authz` 的 `ChannelSensitiveWrite` 等。
- 渠道 `settings`（`Channel.OtherSettings`）中 5 个凭证字段由 `mergeSensitiveOtherSettings` 用 `common.EncryptSecret`（AES-GCM + 服务端 `CryptoSecret`）加密；导出到另一实例必须先在源实例解密，导入时按目标实例密钥重新加密。
- `golang.org/x/crypto v0.52.0` 已在 go.mod；前端渠道页已有批量操作栏、更多操作菜单和 `Dialog` 组件。
- 需求见 `proposal.md` 与 `specs/channel-transfer/spec.md`。

## Goals / Non-Goals

**Goals:**

- 用用户密码加密的 `.nacx` 文件在实例之间迁移选中渠道的完整配置（含密钥）。
- 导入自动分配新 id，且新 id 严格按文件数组顺序递增。
- 密码错误或文件被篡改时拒绝导入；任一渠道校验失败时整批回滚。
- 复用现有渠道校验、插入、权限和审计路径，不改变数据库结构。

**Non-Goals:**

- 不覆盖同名渠道，不做导入预览，不做「导出全部」。
- 不新增数据库表、字段、迁移或 raw SQL。
- 不兼容本次之前不存在的旧导出格式；v1 信封自描述，未来可扩展版本与 KDF 参数。
- 不做跨实例的插件注册表同步；插件不存在时按校验失败处理。

## Decisions

### 1. 文件格式与密码加密

`.nacx` 是 UTF-8 JSON 信封：

```json
{
  "format": "new-api-channel-export",
  "version": 1,
  "kdf": { "name": "scrypt", "n": 32768, "r": 8, "p": 1, "salt": "<base64>" },
  "cipher": { "name": "aes-256-gcm", "nonce": "<base64>", "data": "<base64>" }
}
```

- 密钥：`scrypt(password, salt, N=32768, r=8, p=1, keyLen=32)`；每次导出生成 16 字节随机 salt 和 12 字节随机 nonce。
- 加密：AES-256-GCM，AAD 固定为 `new-api-channel-export:v1`；明文是渠道对象数组的 UTF-8 JSON。
- 解密失败（密码错、密文被改、格式非法）统一返回同一个可翻译错误键，不区分原因。
- 选择 scrypt 而非 PBKDF2：内存硬，抗 GPU 暴力破解；参数写入信封，后续可升级。选择 AES-GCM 而非 CBC：自带完整性校验，篡改必然失败。

### 2. 导出 DTO 与字段裁剪

新增独立的导出结构体，不直接序列化 `model.Channel`，避免把运行时字段或未来新增的敏感字段带出去。

- 保留：`type`、`name`、`key`、`base_url`、`models`、`group`、`priority`、`sort`、`weight`、`auto_ban`、`tag`、`setting`、`settings`、`param_override`、`header_override`、`remark`、`other`、`status_code_mapping`、`openai_organization`、`test_model`，以及 `channel_info` 的配置部分。
- 排除：`id`、`created_time`、`test_time`、`response_time`、`balance`、`balance_updated_time`、`used_quota`、`other_info`。
- `status`：启用(1)、手动禁用(2)原样；自动禁用(3)按启用(1)导出。
- `settings`：用 `map[string]any` 解析原始 JSON，对 5 个凭证字段调用 `common.DecryptSecret` 还原明文，再序列化；未知字段原样保留。
- `channel_info`：只保留 `is_multi_key`、`multi_key_mode`、`is_fallback`；`multi_key_size` 按 `key` 的行数（或 JSON 数组长度）重算；`multi_key_status_list`、`multi_key_disabled_reason/time`、`multi_key_polling_index` 不导出。
- 顺序：前端按表格当前显示顺序提交 `ids`；后端查询后按 `ids` 重排，再写入数组。这样数组顺序就是列表顺序。

### 3. 导入流程与顺序保持

`ImportChannels` 按以下顺序执行：

1. 解析信封、校验格式与版本、用密码解密；失败直接返回，不查库。
2. 解析渠道数组；为空或超过合理上限时拒绝。
3. 对每条构造 `model.Channel`：
   - `settings` 的 5 个凭证字段用 `mergeSensitiveOtherSettings` 重新加密（其余字段保留）。
   - `channel_info` 重算 `multi_key_size`，运行时字段清零。
   - `created_time` 取当前时间；`balance`、`used_quota`、`test_time`、`response_time` 清零。
   - `status` 为 3 时改为 1。
4. 逐条执行与 `AddChannel` 一致的校验：`validateChannel`、任务插件绑定权限、任务插件存在性、请求插件实例存在性、空 key 规则。全部通过后才进入插入。
5. keyless 任务插件渠道保留文件中的 status（通常启用），不调用 `service.PluginLoginNeededStatus` 自动置为待登录（用户裁决，见 `rulings.md` R2）。导入也不逐条调用 `checkPluginChannelKeys`，避免最多 1000 次上游网络校验；规格的校验枚举不含该检查。
6. 条目校验失败时，`message_params` 附带内层错误的 `error_key` / `error_params`（内层为 `*common.Message` 时），前端导入弹窗二次翻译内层原因（用户裁决，见 R4）。
7. 调 `model.BatchInsertChannels`（单事务、50 条一批、回填 id）；成功后 `model.InitChannelCache()`。
8. 返回 `{count, ids}`；`ids` 按文件数组顺序排列。

全有或全无：校验阶段任一失败直接返回，不插入；插入阶段任一条失败由 `BatchInsertChannels` 回滚。错误信息包含文件中的序号和渠道名称，便于定位。

顺序保证：`BatchInsertChannels` 的 chunk 是原切片的子切片，`tx.Create(&chunk)` 会把自增 id 回填到原顺序的元素上；多行 INSERT 在 SQLite/MySQL/PostgreSQL 上按数组顺序分配自增 id。测试用「新 id 严格递增」锁定该行为。

### 4. 查询与路由

- 在 `model/channel.go` 新增 `GetChannelsByIdsOrdered(ids []int) ([]*Channel, error)`：用 GORM `Where("id IN ?", ids)` 查询，包含 key；返回前按 `ids` 顺序重排，找不到的 id 报错，避免静默丢数据。仓库已有无序且静默跳过缺失 id 的 `GetChannelsByIds`（被 ratio-sync 使用），不改其语义（见 `rulings.md` R1）。
- 路由：`POST /api/channel/export`、`POST /api/channel/import`，加入 `channelPermissionRoutes`，权限 `authz.ChannelSensitiveWrite`。
- 两个接口都用 POST，密码只在请求体，不进 URL、日志或审计。

### 5. 权限、审计与响应头

- 导出/导入都要求 `ChannelSensitiveWrite`（与新建、删除、复制渠道同级）；前端用 `hasPermission` 置灰入口。
- 审计动作 `channel.export` / `channel.import`，模板写入 `controller/audit.go`，参数只有 `count`；不记录 key、密码、文件名。
- 导出响应加 `Cache-Control: no-store`，避免中间层缓存加密文件。

### 6. 前端交互

- `api.ts` 增加 `exportChannels({ ids, password })` 与 `importChannels({ password, content })`；`types.ts` 增加返回类型。
- 导出：批量操作栏新增下载图标按钮；无选中时不可点。点击打开 `ExportChannelsDialog`，输入密码（`type=password`），成功后把 `content` 转成 Blob 下载为后端返回的文件名，清空密码并提示数量。
- 导入：`ChannelsPrimaryButtons` 的「更多操作」菜单新增「导入渠道」，无 `ChannelSensitiveWrite` 时置灰。`ImportChannelsDialog` 提供文件选择（`.nacx`）与密码输入，读取文件文本后提交；成功后关闭弹窗、刷新渠道列表、提示数量。
- 密码只保存在组件状态，关闭弹窗即清空；不写 localStorage、不打印到控制台。
- 下载复用现有 Blob + anchor 模式（参考 `web/src/features/usage-logs/lib/export.ts`），不新增依赖。

### 7. 国际化与审计展示

- 新增文案全部用 `t('英文键')`；`bun run i18n:sync` 补齐 7 个语言文件。
- 后端错误用 `common.NewMessage` / `common.ApiErrorT`，返回 `message_key` + `message_params`；前端 `getServerMessage` 会翻译，需手动把新错误键补进 7 个语言文件（i18n:sync 只扫描前端 `t()` 字面量）。
- 导入条目错误额外带 `message_params.error_key` / `error_params`；导入弹窗用 `t(error_key, error_params)` 翻译内层原因，再渲染外层键。内层错误不是 `*common.Message` 时回退 `error` 英文原文。
- `web/src/features/usage-logs/lib/format.ts` 增加 `channel.export` / `channel.import` 两条展示模板，键与 `controller/audit.go` 一致。

### 8. 测试策略

- 后端新增 `controller/channel_export_test.go`，一个测试函数 `TestChannelExportImport`，用子测试覆盖：
  - 导出顺序与 ids 一致、明文无 id；
  - 正确密码导入：新 id 严格递增、字段一致、created_time 变新、状态 3 变 1；
  - 错误密码：失败且渠道数不变；
  - 密文改一字节：失败且不落库；
  - 中途一条非法：整批回滚；
  - 敏感 settings 跨实例重新加密后可解密。
- 前端新增 `channel-export-import.test.tsx`，覆盖导出请求参数与下载、导入文件读取与提交、错误密码提示、无权限/无选中禁用。
- 不新增依赖、不改表结构；数据库写入复用现有 GORM 路径，不触发三数据库 schema 矩阵。测试在 SQLite 上跑真实 handler 与真实事务。

## Risks / Trade-offs

- [密码丢失导致文件不可恢复] → 导出弹窗明确提示「密码无法找回」；密码只用于本次加密，不落库。
- [导出文件含明文密钥（加密信封内）] → 文件整体用用户密码加密；接口要求敏感写权限；响应 `no-store`；审计不记录密钥。
- [跨实例插件/请求插件依赖缺失] → 校验失败整批回滚，错误指出第几条；用户可先在目标实例准备插件后重试。
- [大文件与请求体上限] → 只导出选中渠道（列表每页最多 200 条），请求体默认上限 32MB；超出时返回明确错误。
- [多行 INSERT 的 id 顺序假设] → 测试断言新 id 严格递增；若未来更换插入方式，测试会先失败。
- [导出格式演进] → 信封含 `version` 与 KDF 参数；新版本可拒绝或兼容旧版本，不影响 v1。
- [同名渠道] → 允许创建同名渠道，不做覆盖；与手工新建行为一致。

## Migration Plan

- 无数据库迁移、无配置项、无新依赖；后端与前端同版本部署即可。
- 回滚：移除两条路由与前端入口；已导出的 `.nacx` 文件仍可用旧版本代码导入（只要格式未变）。数据库中没有新增字段，回滚不需要数据修复。

## Open Questions

无。

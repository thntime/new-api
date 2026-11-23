# Proposal

## Why

渠道配置目前只能逐个手工重建；在实例迁移、备份恢复或批量复制渠道时，管理员无法把一组渠道的完整配置（含密钥）安全地搬到另一套实例。需要提供带密码加密的批量导出/导入，且导入后渠道的相对顺序不变。

## What Changes

- 渠道列表批量操作栏新增「导出选中渠道」：输入密码后生成加密的 `.nacx` 文件；文件不包含渠道 id。
- 渠道页「更多操作」新增「导入渠道」：选择 `.nacx` 文件并输入同一密码，密码正确才创建渠道；密码错误或文件被篡改时拒绝导入。
- 导入按文件数组顺序创建渠道，新 id 自动递增且严格保持顺序；任一渠道校验失败则整批回滚。
- 导出/导入要求渠道敏感写权限，并写入 `channel.export` / `channel.import` 管理审计日志（只记录数量，不记录密钥或密码）。
- 新增界面文案与审计展示文案，覆盖全部 7 种界面语言。

## Capabilities

### New Capabilities

- `channel-transfer`: 渠道批量加密导出与导入，包括密码校验、字段裁剪、顺序保持、权限与审计。

### Modified Capabilities

（无）

## Impact

- 后端：新增 `controller/channel_export.go`、`controller/channel_export_test.go`；修改 `router/channel-router.go`、`controller/audit.go`、`model/channel.go`（按 id 批量查询并保持传入顺序）。
- 前端：`web/src/features/channels/**`（API、类型、导出/导入弹窗、批量操作栏、更多操作菜单）、`web/src/features/usage-logs/lib/format.ts`、`web/src/i18n/locales/*.json`。
- 依赖：使用已有 `golang.org/x/crypto/scrypt` 与标准库 AES-GCM；不新增依赖、不改表结构、不新增迁移。
- 兼容性：导出文件为 v1 自描述格式；不存在需要兼容的旧导出格式。

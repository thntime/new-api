# Spec Delta

## Purpose

让管理员把选中的渠道配置以密码加密文件批量导出，并在另一套实例上输入同一密码导入；导入自动分配新 id 且保持导出顺序。

## ADDED Requirements

### Requirement: 加密导出选中渠道

管理员 SHALL 能在渠道列表的批量操作栏对选中渠道执行导出。导出 MUST 要求输入 8–128 字符的密码，并生成以该密码加密的 `.nacx` 文件。导出内容 MUST NOT 包含渠道 id、创建/测试/响应时间、余额、已用额度或其他运行时统计字段；MUST 包含类型、名称、密钥、模型、分组、优先级、序号、权重、标签、设置等配置字段。导出数组顺序 MUST 与选中行在列表中的显示顺序一致。

#### Scenario: 选中渠道导出成功

- **WHEN** 管理员选中 3 个渠道并输入有效密码执行导出
- **THEN** 下载一个 `.nacx` 文件，文件解密后不含渠道 id，数组顺序与列表显示顺序一致

#### Scenario: 未选中渠道时不可导出

- **WHEN** 管理员没有选中任何渠道
- **THEN** 导出入口不可用

#### Scenario: 密码过短

- **WHEN** 管理员输入的密码少于 8 个字符
- **THEN** 拒绝导出并提示密码长度要求

### Requirement: 密码加密与文件完整性

导出的 `.nacx` 文件 MUST 是自描述 JSON 信封，包含格式标识、版本、密钥派生参数、AES-256-GCM nonce 与密文；加密密钥 MUST 由用户密码通过 scrypt 派生。密码错误、文件被篡改或格式非法时，导入 MUST 失败且 MUST NOT 创建任何渠道。错误提示 MUST NOT 区分「密码错误」与「文件损坏」。

#### Scenario: 正确密码解密

- **WHEN** 使用导出时的同一密码导入文件
- **THEN** 解密成功并进入渠道创建流程

#### Scenario: 错误密码

- **WHEN** 使用错误密码导入文件
- **THEN** 导入失败，数据库中渠道数量不变

#### Scenario: 篡改密文

- **WHEN** 修改 `.nacx` 文件密文中的一个字节后导入
- **THEN** 导入失败，数据库中渠道数量不变

### Requirement: 导入创建与顺序保持

管理员 SHALL 能从渠道页「更多操作」菜单导入 `.nacx` 文件。导入 MUST 要求输入密码，并在密码正确后按文件数组顺序创建渠道。新渠道 id MUST 由数据库自动递增，且 MUST 严格按数组顺序递增。导入后 MUST 刷新渠道列表并提示导入数量。

#### Scenario: 顺序保持

- **WHEN** 导入包含 3 个渠道的文件
- **THEN** 创建 3 个渠道，新 id 严格递增，顺序与文件数组一致

#### Scenario: 配置字段保留

- **WHEN** 导入一个渠道
- **THEN** 类型、名称、密钥、base_url、models、group、priority、sort、tag、setting 与导出前一致

#### Scenario: 运行时字段重置

- **WHEN** 导入一个渠道
- **THEN** created_time 取当前时间，balance、used_quota、test_time、response_time 重置，自动禁用(3)按启用(1)导入，手动禁用(2)保持

### Requirement: 导入校验与整批回滚

导入 MUST 对每个渠道执行与手工新建一致的校验（渠道设置、任务插件权限与存在性、请求插件实例存在性、空 key 规则）。任一渠道校验失败时，整批导入 MUST 回滚，MUST NOT 创建任何渠道，并 MUST 报告失败渠道在文件中的序号和名称。

#### Scenario: 中途一条非法

- **WHEN** 文件第 2 个渠道引用不存在的请求插件实例
- **THEN** 整批回滚，渠道数量不变，错误信息指出第 2 条及渠道名称

#### Scenario: 任务插件空 key

- **WHEN** 任务插件且支持登录的渠道没有 key
- **THEN** 允许导入，并保留文件中的 status（通常启用），不自动置为待登录
- **AND** 其他类型渠道没有 key 时拒绝并整批回滚

#### Scenario: 全部合法

- **WHEN** 文件中所有渠道都通过校验
- **THEN** 整批创建成功

### Requirement: 导出导入权限与审计

导出和导入接口 MUST 要求渠道敏感写权限。每次成功导出 MUST 写入 `channel.export` 审计日志，每次成功导入 MUST 写入 `channel.import` 审计日志；审计内容 MUST 只包含渠道数量，MUST NOT 包含渠道密钥、导出密码或文件名。未授权用户 MUST NOT 看到可用的导出/导入入口。

#### Scenario: 有敏感写权限

- **WHEN** 有渠道敏感写权限的管理员执行导出或导入
- **THEN** 操作成功，审计日志记录渠道数量

#### Scenario: 无敏感写权限

- **WHEN** 没有渠道敏感写权限的管理员打开渠道页
- **THEN** 导出/导入入口不可用，直接调用接口被拒绝

#### Scenario: 审计不含秘密

- **WHEN** 导出或导入完成后查看审计日志
- **THEN** 日志中没有渠道密钥、密码或文件名

### Requirement: 敏感设置随文件迁移

渠道 `settings` 中的加密凭证字段 MUST 在导出前解密为明文并随文件一起加密，导入时 MUST 使用目标实例的密钥重新加密。导出/导入 MUST 保留 `settings` 中的未知字段，不得因解析为固定结构而丢失。

#### Scenario: 跨实例迁移凭证

- **WHEN** 导出含 commandcode_cookie、sensenova_username、sensenova_password、volc_coding_plan_access_key_id、volc_coding_plan_secret_access_key 的渠道，并在另一实例导入
- **THEN** 目标实例导入后仍能解密并使用这些凭证

#### Scenario: 未知字段保留

- **WHEN** 渠道 settings 中包含当前代码不认识的额外字段
- **THEN** 导入后这些字段仍然存在

### Requirement: 多 Key 渠道配置迁移

多 Key 渠道的 `channel_info` MUST 只迁移 `is_multi_key`、`multi_key_mode`、`is_fallback` 等配置；`multi_key_size` MUST 按密钥行数重新计算；多 Key 运行时状态（各 key 状态、禁用原因/时间、轮询索引）MUST NOT 导出。

#### Scenario: 多 Key 渠道导入

- **WHEN** 导入一个多 Key 渠道
- **THEN** 密钥行数不变，multi_key_size 正确，轮询索引从 0 开始

#### Scenario: 单 Key 渠道

- **WHEN** 导入一个单 Key 渠道
- **THEN** is_multi_key 为 false，行为与手工新建一致

### Requirement: 界面文案与错误提示

导出/导入的所有用户可见文案 MUST 支持 i18n，覆盖 en、zh、zh-TW、fr、ru、ja、vi 七种语言。导出成功、导入成功、密码错误/文件损坏、权限不足等提示 MUST 使用可翻译的文案键。

#### Scenario: 切换语言

- **WHEN** 用户切换到任一支持语言
- **THEN** 导出/导入弹窗与提示均显示对应语言

#### Scenario: 密码错误提示

- **WHEN** 用户用错误密码导入
- **THEN** 显示「密码错误或文件已损坏」的本地化文案

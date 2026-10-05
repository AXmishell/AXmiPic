# 图片安全、云处理、短信与邮件

这些能力都通过可插拔接口实现，未配置服务商时回退到安全的默认实现。

| 配置 | 说明 |
|------|------|
| `security.scanner` | 上传内容扫描器：`none`（放行）或 `builtin`（白名单 + 危险魔数检测，拒绝伪装成图片的可执行内容） |
| `security.cloud_processor` | 云处理器：`local`（使用内置成像流程）；接入外部云服务时实现 `security.CloudProcessor` |
| `sms.*` | 通用 HTTP 短信网关：`enabled`、`provider`、`endpoint`、`method`；未启用时记录到日志 |
| `email.*` | SMTP 邮件：`host`、`port`、`username`、`password`、`from`、`use_tls`；未启用时记录到日志 |

管理接口：

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/admin/security` | 当前启用的扫描器 |
| GET | `/admin/runtime` | 实例运行环境信息（站点地址、数据库/存储/处理器、配额与限流等，不含密钥） |
| GET | `/admin/process` | 进程实时运行时指标（Goroutine、堆内存、堆对象数、GC、运行时长等） |
| GET | `/admin/notify/channels` | 已配置的短信与邮件渠道 |
| POST | `/admin/notify/test` | 发送测试通知（`{"channel":"sms","to":"…","body":"…"}`） |
| GET | `/admin/notify/logs` | 通知发送日志（`?channel=email\|sms&page=&page_size=`，按时间倒序） |
| GET | `/admin/notify/smtp` | 读取 SMTP 邮件渠道设置（密码仅返回是否已设置） |
| PUT | `/admin/notify/smtp` | 保存 SMTP 设置并即时生效（`password` 为空表示保持原密码） |
| GET/PUT | `/admin/auth` | 在线读取/切换权限开关（`allow_registration`、`require_auth`、`allow_guest_upload`），保存即时生效、无需重启 |
| GET/PUT | `/admin/moderation` | 在线读取/保存图片广场 AI 审查设置（密钥脱敏），保存即时生效、无需重启 |
| GET | `/admin/payment` | 读取支付设置（密钥仅返回是否已设置） |
| PUT | `/admin/payment` | 保存支付设置并即时生效（密钥为空表示保持原值） |

扫描器在 `multipart` 上传与预签名直传确认两个入口都会执行；命中危险内容时返回 HTTP 422 并拒绝入库。通知渠道的兜底实现会把消息写入服务端日志，便于开发调试。管理端「系统设置 → 通知设置」提供 SMTP 配置、渠道查看与发送测试；SMTP 密码经主密钥加密后保存在数据库的 `settings` 表中，保存后邮件渠道即时切换，无需重启。每次发送（含验证码、通知与后台测试）都会记录到 `notify_logs` 表，可在管理端侧边栏的「邮件日志」页面按邮件/短信查看，日志保留 30 天后自动清理。

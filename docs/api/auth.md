# 认证

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/auth/register` | 注册普通用户（需邮箱验证码） |
| POST | `/auth/register/code` | 向邮箱发送注册验证码（无需登录） |
| POST | `/auth/login` | 普通用户登录（用户名或邮箱） |
| POST | `/admin/auth/login` | 管理员登录（独立入口） |
| POST | `/auth/totp/verify` | 完成登录时的 TOTP 二次验证（`challenge_token` + `code`） |
| GET | `/auth/me` | 当前账号信息 |
| GET | `/auth/security` | 当前账号的安全设置状态（TOTP、邮箱） |
| POST | `/auth/totp/setup` | 生成 TOTP 密钥并返回 `secret` 与 `otpauth` 链接 |
| POST | `/auth/totp/enable` | 校验动态码后启用二次验证 |
| POST | `/auth/totp/disable` | 关闭二次验证（动态码或密码） |
| POST | `/auth/email/code` | 向目标邮箱发送验证码（每账号每分钟 1 条、每天上限 10 条） |
| POST | `/auth/email/verify` | 校验验证码并绑定邮箱；换绑不同邮箱时需提供当前密码 |
| POST | `/auth/email/unbind` | 解绑邮箱（需当前密码） |
| POST | `/auth/password` | 修改当前账户密码（需 `current_password`、`new_password`） |
| POST | `/auth/password/reset/code` | 向已验证邮箱发送密码重置验证码（无需登录） |
| POST | `/auth/password/reset` | 校验验证码并重置密码（无需登录） |

启用 TOTP 后，`/auth/login` 与 `/admin/auth/login` 不再直接返回会话，而是返回
`{"totp_required": true, "challenge_token": "…"}`；客户端需携带该令牌与
身份验证器生成的 6 位动态码调用 `/auth/totp/verify` 换取正式会话。TOTP 密钥
经主密钥加密后保存，登录挑战令牌带有独立作用域，不能作为会话使用。邮箱绑定
通过验证码验证邮箱真实可用，未配置真实邮件渠道时验证码会回退记录到服务端日志。
普通用户与管理员均可使用以上安全能力。

邮箱换绑策略：绑定与换绑共用发码/验证接口，换绑只需验证新邮箱，但**当目标邮箱
与当前已验证邮箱不同时必须提供当前密码**；换绑成功后向旧邮箱发送一条变更通知；
验证码发送按账号限流（每分钟 1 条、突发 2 条、每天上限 10 条），超限返回
HTTP 429。验证码 10 分钟内有效、一次性、最多尝试 5 次，且与其他账号已绑定的
邮箱冲突时返回 HTTP 409。验证码与每日发送计数持久化在数据库中，可在服务重启
与多实例部署下保持一致。

密码管理：已登录用户通过 `POST /auth/password` 提供当前密码即可修改密码。忘记
密码时可调用 `POST /auth/password/reset/code` 向已验证邮箱发送验证码，再调用
`POST /auth/password/reset` 携验证码设置新密码；为避免账户枚举，对未注册邮箱
发送请求同样返回成功但不实际发信，重置时未注册邮箱与验证码错误返回同一错误。

注册与邮箱登录：普通用户注册需先调用 `POST /auth/register/code` 向邮箱发送验证码
（邮箱已被占用时返回 HTTP 409），再调用 `POST /auth/register` 携 `username`、
`email`、`code`、`password` 完成注册，注册成功后邮箱即标记为已验证。登录时
`POST /auth/login` 的 `username` 字段既可填用户名也可填邮箱。管理员账户由后台或
引导配置创建，不经过该注册流程。

为避免登录标识歧义，同一账户表内用户名与邮箱互不占用：用户名不能等于他人的
邮箱，邮箱也不能等于他人的用户名（登录按「先用户名、后邮箱」查找）。

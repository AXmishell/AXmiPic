# AXmiPic

轻量、可靠的自托管图床服务。提供图片上传、即时处理、多存储后端与后台管理，后端为单个 Go 二进制，前端为内嵌的单页应用。

## 特性

- **多种上传方式**：后台界面上传、批量上传、粘贴上传、拖拽上传、`multipart` 接口上传、对象存储预签名直传
- **一键嵌入代码**：复制图片的 URL、HTML、BBCode 或 Markdown（受角色功能开关控制）
- **图片与相册分享**：生成分享链接，可选访问密码、有效期与最大访问次数；公开分享页为 `/s/{token}`
- **站内公告与独立页面**：管理员发布公告（支持置顶与级别）并在仪表盘展示；维护可通过 `/p/{slug}` 公开访问的独立页面
- **举报管理**：用户举报图片，管理员在后台处理或驳回
- **套餐与计费**：套餐（价格、有效期、配额、角色组）与优惠券（固定/百分比、门槛、限次、时效），下单、订单管理与可插拔支付渠道
- **官方支付适配**：支付宝当面付（RSA2 签名/验签）与微信支付 v3（SHA256-RSA 签名、AES-GCM 回调解密），以及人工/模拟渠道
- **图片安全**：上传内容扫描器（可插拔），内置白名单 + 危险魔数检测，拒绝伪装成图片的可执行内容
- **通知系统**：可插拔的短信（通用 HTTP 网关）与邮件（SMTP/STARTTLS）渠道，支持后台发送测试
- **工单系统**：用户提交工单并对接客服，管理员回复与关闭
- **随机存储键与去重**：存储文件使用按日期分区、随机且不可猜测的键名；内容 `sha256` 单独入库，同一所有者上传相同内容时自动去重，不同所有者各自持有独立对象
- **保留原始文件名**：存储层使用随机命名后的文件，数据库中单独记录原文件名、存储文件名与哈希值
- **多图片处理驱动**：纯 Go（默认）、libvips（`-tags libvips`）与 ImageMagick（`magick` 命令）三种处理器，运行时按配置选择并在不可用时回退
- **即时图片处理**：通过 URL 查询参数实时缩放、裁剪、旋转、翻转、转灰度、模糊、锐化、文字水印与转码，带 ETag 缓存
- **多存储后端**：本地文件系统、S3 兼容对象存储（AWS S3 / MinIO / Cloudflare R2 / 阿里云 OSS / 腾讯云 COS）、七牛云 Kodo
- **运行中热切换存储**：后台可随时切换默认存储；已有图片按记录自动路由回其原存储读取，无需重启
- **数据库可选**：SQLite（默认，开箱即用）或 PostgreSQL
- **相册与图片广场**：用相册归类图片；图片与相册均可设为公开，出现在跨用户的图片广场与公开相册列表，并提供用户公开资料页
- **角色组与多策略**：按角色组控制用户的存储配额、上传限制、速率限制、图片处理能力与功能开关；策略可复用、按类型覆盖，注册用户自动加入默认角色组
- **账户体系**：管理员与普通用户分表管理；JWT 会话 + 长期 API 令牌
- **界面分离与安装向导**：公开首页 + 用户中心（`/user`）与管理员控制台（`/admin`）分离；内置 Guest 低权访客与 install 初始化向导
- **配额与限流**：按用户的存储配额，按用户/访客/IP 的速率限制
- **安全**：密钥 AES-256-GCM 加密存储、bcrypt 密码、纵深防御响应头

## 技术栈

| 层 | 技术 |
|----|------|
| 后端 | Go 1.26、chi 路由、GORM |
| 数据库 | SQLite（纯 Go 驱动） / PostgreSQL |
| 图片处理 | 纯 Go（`disintegration/imaging`），可选 libvips（`-tags libvips`） |
| 对象存储 | 本地 / S3 兼容（minio-go） / 七牛云 SDK |
| 支付 | 微信支付官方 SDK（`wechatpay-apiv3/wechatpay-go`）、支付宝 RSA2（原生）、易支付（原生） |
| 前端 | Vue 3、Pinia、Vue Router、Element Plus、TypeScript、Vite |

## 快速开始

### 环境要求

- Go 1.26 或更高版本
- （可选，仅重新构建前端时需要）Node.js 22+ 与 pnpm 9

前端产物已内嵌进 `internal/webui/dist`，仅构建后端时无需 Node 环境。

### 构建与运行

```bash
# 构建后端二进制（前端已内嵌）
make build          # 产物位于 bin/axmipic

# 使用示例配置运行
cp configs/config.example.yaml configs/config.yaml
./bin/axmipic -config configs/config.yaml
```

或直接运行：

```bash
make run            # 等价于 go run ./cmd/axmipic -config configs/config.example.yaml
```

默认监听 `0.0.0.0:8080`，访问 `http://localhost:8080` 打开控制台。

### 创建管理员

有两种方式：

1. **配置文件播种**（推荐）：在 `configs/config.yaml` 中设置 `auth.bootstrap_admin: "用户名:密码"`，仅当 admins 表为空时生效：

   ```yaml
   auth:
     bootstrap_admin: "admin:你的强密码"
   ```

2. **后台创建**：以已有管理员登录后，在「用户管理 → 管理员」标签页新建管理员。

> 首次部署建议先配置 `auth.bootstrap_admin` 再启动，避免无管理员可用。注册入口只创建普通用户。

## 配置说明

配置文件为 YAML，支持通过环境变量覆盖，前缀为 `AXMIPIC_`（例如 `AXMIPIC_SERVER_PORT=9000`）。完整示例见 `configs/config.example.yaml`。

### 服务器

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  base_url: "http://localhost:8080"   # 生成本地存储图片公开 URL 时使用
  trust_proxy: false                    # 位于可信反向代理之后时才设为 true
  read_timeout_sec: 30
  write_timeout_sec: 30
  shutdown_timeout_sec: 10
```

> `trust_proxy` 为 `true` 时才会解析 `X-Forwarded-For` / `X-Real-IP`。直连部署请保持 `false`，否则客户端可伪造来源 IP 绕过限流。

### 数据库

```yaml
database:
  driver: "sqlite"                      # sqlite | postgres
  dsn: "./data/axmipic.db"              # sqlite 为文件路径
  # postgres 使用 libpq 连接串或 URL：
  #   host=127.0.0.1 port=5432 user=axmipic password=secret dbname=axmipic sslmode=disable
  #   postgres://axmipic:secret@127.0.0.1:5432/axmipic?sslmode=disable
```

SQLite 会自动启用 `busy_timeout` 与 WAL 模式。

### 存储

```yaml
storage:
  driver: "local"                       # local | s3 | qiniu
  local:
    root: "./data/uploads"              # 本地文件存放目录
  s3:
    endpoint: "https://s3.amazonaws.com"
    region: "us-east-1"
    bucket: "axmipic"
    access_key_id: ""
    secret_access_key: ""
    secure: true
    use_path_style: false
    public_base_url: ""                 # 可选，CDN 或自定义公网域名
    presign_expiry_sec: 900
  qiniu:
    access_key: ""
    secret_key: ""
    bucket: "axmipic"
    domain: "https://cdn.example.com"
    upload_host: ""
    zone: ""
    private: false
    use_https: true
    presign_expiry_sec: 3600
```

配置文件中的存储始终作为兜底后端。更多后端可在后台「存储配置」中动态添加并在运行中切换。

### 上传与处理

```yaml
upload:
  max_size_mb: 20
  allowed_mime_types:
    - "image/jpeg"
    - "image/png"
    - "image/gif"
    - "image/webp"

processing:
  enabled: true
  max_width: 4096
  max_height: 4096
  default_quality: 82
  allowed_formats: ["jpeg", "png", "gif", "webp", "avif"]
```

### 认证与限流

```yaml
auth:
  jwt_secret: ""                        # 留空时自动生成并持久化（见下方说明）
  encryption_key: ""                    # 可选：仅用于加密存储后端密钥，留空回退 jwt_secret
  session_ttl_hours: 24
  allow_registration: true
  require_auth: true                    # true 时仅登录用户可上传
  default_quota_mb: 1024                # 每用户配额，0 表示不限
  bootstrap_admin: ""                   # 仅当无管理员时播种，格式 "用户名:密码"

limits:
  upload_per_minute: 30
  upload_burst: 5
  guest_per_minute: 6
  guest_burst: 2
  image_per_minute: 600                 # 公开图片读取/处理的每 IP 限流
  image_burst: 120
  share_per_minute: 30                  # 分享访问（密码校验）的每 IP 限流，防暴力破解
  share_burst: 10
```

生产环境与多实例部署请务必显式设置固定的 `auth.jwt_secret`。当日 `jwt_secret` 与 `encryption_key` 都留空时，服务会生成一个主密钥并持久化到磁盘（SQLite 场景为数据库同目录下的 `.axmipic-key`，否则为 `./data/.axmipic-key`），从而保证重启后已加密入库的存储密钥仍可解密；单实例部署可依赖此机制开箱即用。

### 访客与安装

```yaml
auth:
  # 允许未登录访客上传（使用 Guest 低权角色组）。
  allow_guest_upload: true
  # Guest 角色的配额（MiB）与单文件上限（MiB）。
  guest_quota_mb: 64
  guest_upload_max_mb: 5

install:
  # 该锁文件存在即视为已安装；删除它并重启可重新进入安装向导。
  lock_file: "./data/install.lock"
  # 安装向导会重写的配置文件路径。
  config_path: "./configs/config.yaml"
  # 安装授权令牌。留空且未禁用安装时，服务启动会生成随机令牌并输出到日志，
  # 需在安装页面填写后方可执行初始化。
  token: ""
  # 完全跳过安装检查（测试 / 容器编排）。
  disabled: false
```

## 图片处理

图片通过 `/i/<存储键>` 访问，可在查询串中附加处理参数：

```
http://localhost:8080/i/2026/10/05/6f1c…a2.png?w=400&h=300&fit=cover&f=webp&q=80
```

| 参数 | 含义 | 取值 |
|------|------|------|
| `w` | 目标宽度 | 像素 |
| `h` | 目标高度 | 像素 |
| `fit` | 缩放策略 | `contain`（默认，等比缩放）/ `cover`（裁剪填充）/ `fill`（拉伸填充，`cover`/`fill` 均需同时给出 `w`、`h`） |
| `q` | 输出质量 | 1–100，缺省用 `processing.default_quality` |
| `f` | 输出格式 | `jpeg`/`png`/`gif`/`webp`/`avif`（受 `processing.allowed_formats` 与处理器能力限制） |
| `r` | 旋转角度 | `90`/`180`/`270` |
| `flip` | 翻转 | `h`（水平）/`v`（垂直）/`hv`（同时） |
| `gray` | 转灰度 | `1`/`true` |
| `blur` | 高斯模糊半径 | 0–100 |
| `sharpen` | 锐化强度 | 0–100 |
| `wm` | 文字水印内容 | 任意文本（最长 200 字符） |
| `wm_pos` | 水印位置 | `top-left`/`top-right`/`bottom-left`/`bottom-right`/`center` |
| `wm_opacity` | 水印不透明度 | 0–100（默认 80） |
| `wm_size` | 水印字号 | 像素，0 表示自适应 |
| `wm_color` | 水印颜色 | 十六进制，如 `#ffffff` |
| `enlarge` | 是否允许放大 | `1`/`true`/`yes`/`on` |

带处理参数的响应会附带 `ETag`，支持 `If-None-Match` 返回 `304`。

`processing` 配置提供开关与默认值：

```yaml
processing:
  enabled: true
  max_width: 4096
  max_height: 4096
  default_quality: 82
  allowed_formats: ["jpeg", "png", "gif", "webp", "avif"]
  allow_enlarge: false      # 是否允许放大
  allow_effects: true       # 是否允许灰度/模糊/锐化
  allow_watermark: true     # 是否允许通过 URL 加水印
  watermark_text: ""        # 非空时为所有变换强制叠加该水印
```

前端「图片处理」页面（`/processing`）提供上述参数的实时预览，并生成可复制的处理链接。

## 界面与路由

前端按角色分离为三套界面，并内置安装向导：

| 路径 | 面向 | 说明 |
|------|------|------|
| `/` | 所有访客 | 公开首页：品牌页 + 图片上传（拖拽/点击/访客可用）+ 图片广场预览 + 登录/用户中心入口 |
| `/user/*` | 普通用户 | 用户中心：我的图片、图片处理、相册、图片广场、分享、令牌、套餐、工单、账号设置 |
| `/admin/*` | 管理员 | 管理控制台：仪表盘、用户管理、图片广场、站点内容、角色策略、计费管理、存储配置、系统设置 |
| `/login` | 所有 | 普通用户登录 / 注册 |
| `/admin/login` | 管理员 | 管理员独立登录页 |
| `/install` | 所有 | 安装向导（仅在未安装时可访问） |

登录后按角色跳转：普通用户进入 `/user`，管理员进入 `/admin`；普通用户访问 `/admin/*` 会被重定向到 `/user`，未登录访问 `/admin/*` 会被导向独立的 `/admin/login`。

### Guest 访客

系统内置一个低权 `guest` 账户与 `Guest 访客` 角色组（更小的配额与上传体积，且默认关闭广场、分享、令牌等功能）。当 `auth.allow_guest_upload` 为 `true` 时，未登录访客可在首页上传，此时按 Guest 角色组的策略限流与限量；该账户使用随机密码，无法用于登录。

### 安装向导

启动时若 `install.lock_file`（默认 `./data/install.lock`）不存在，则视为未安装：除 `/install` 外的所有前端路由都会跳转到安装向导，管理员与角色组的播种也会被跳过。向导完成后会：

1. 校验并连接数据库、运行迁移；
2. 创建管理员账户；
3. 播种默认角色组与 `Guest 访客` 角色组；
4. 创建内置 `guest` 账户；
5. 写入配置文件与 `install.lock`。

锁文件存在即视为已安装，访问 `/install` 会自动跳回首页。要重新安装，删除 `install.lock`（及数据库）后重启即可。相关接口：

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/install/status` | 安装状态（无需登录） |
| POST | `/api/v1/install` | 执行初始化（无需登录） |

设置 `install.disabled: true` 可完全跳过安装检查（适用于测试与容器编排）。

> 安全提示：未安装时 `POST /api/v1/install` 需要携带安装令牌（请求头 `X-Install-Token`）。未显式配置 `install.token` 时，服务启动会生成一个随机令牌并在日志中以 `install_token` 字段输出，需在安装页面填写后才能完成初始化，从而避免实例在初始化前被他人抢先接管。也可配置 `auth.bootstrap_admin` 跳过安装向导。

## API

所有接口以 `/api/v1` 为前缀，请求头使用 `Authorization: Bearer <令牌>`（会话 JWT 或 API 令牌）。响应统一为：

```json
{ "code": 0, "message": "ok", "data": {} }
```

`code` 为 0 表示成功；错误时 `code` 为对应 HTTP 状态码，`data` 为 `null`。

### 认证

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

### 上传

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/upload` | `multipart` 表单上传，字段名 `file` |
| POST | `/upload/presign` | 申请对象存储预签名直传 |
| POST | `/upload/confirm` | 确认直传完成并入库 |

`/upload` 返回示例：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "id": "9fa5bcf9-…",
    "key": "2026/10/05/6f1c3b8e-…-a2.png",
    "url": "http://localhost:8080/i/2026/10/05/6f1c3b8e-…-a2.png",
    "original_name": "新图.png",
    "filename": "6f1c3b8e-…-a2.png",
    "hash": "163053ece784c464ae8fe55e2532d2804a3762eeac80e5b6cbef4f7e2c16bce4",
    "permission": "private",
    "size": 73,
    "mime_type": "image/png",
    "width": 5,
    "height": 7,
    "created_at": "2026-10-04T18:57:00+08:00"
  }
}
```

使用 curl 上传：

```bash
curl -X POST http://localhost:8080/api/v1/upload \
  -H "Authorization: Bearer <API_TOKEN>" \
  -F "file=@./photo.png"
```

### 图片与令牌

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/images` | 图片列表（分页与过滤，见下） |
| GET | `/images/{id}` | 单张图片信息 |
| PATCH | `/images/{id}` | 重命名图片（仅修改展示用原文件名） |
| DELETE | `/images/{id}` | 删除图片 |
| POST | `/images/batch` | 批量设置可见性 / 所属相册 |
| POST | `/tokens` | 创建 API 令牌（明文仅返回一次） |
| GET | `/tokens` | 令牌列表 |
| DELETE | `/tokens/{id}` | 吊销令牌 |

`GET /images` 支持查询参数 `page`、`page_size`、`order`（`newest`/`earliest`/`largest`/`smallest`）、`keyword`（按文件名搜索）、`permission`（`public`/`private`）与 `album_id`（按相册过滤）。

`POST /images/batch` 请求体示例，`permission` 与 `album_id`/`clear_album` 至少提供其一：

```json
{ "ids": ["<id1>", "<id2>"], "permission": "public", "album_id": "<album_id>" }
```

### 相册与图片广场

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/albums` | 当前账号的相册列表（含图片数量、所有者与可见性） |
| POST | `/albums` | 新建相册（可带 `permission`: `private`/`public`） |
| GET | `/albums/{id}` | 相册详情（公开相册无需登录） |
| PATCH | `/albums/{id}` | 修改相册名称/简介/可见性（仅所有者） |
| DELETE | `/albums/{id}` | 删除相册（图片保留，仅移出相册） |
| GET | `/albums/{id}/images` | 相册中的图片（公开相册无需登录） |
| GET | `/plaza` | 公开图片广场（跨用户，返回 `permission=public` 的图片，支持 `user_id` 按作者过滤） |
| GET | `/plaza/albums` | 公开相册列表（支持 `user_id` 过滤） |
| GET | `/users/{id}` | 用户公开资料：用户名、公开图片数与公开相册 |

图片的 `permission` 取值为 `private`（默认，仅本人可见）或 `public`（可出现在图片广场，并在公开列表中附带 `owner_username`）。相册的 `permission` 同样为 `private`（默认）或 `public`：公开相册的详情与图片对未登录访客开放，并出现在 `/plaza/albums`。广场、公开相册与用户资料均为只读接口，无需登录即可访问。

### 分享

分享链接可指向单张图片或整个相册，支持可选的访问密码、有效期（小时）与最大访问次数。

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/shares` | 创建分享（体见下） |
| GET | `/shares` | 当前账号创建的分享 |
| DELETE | `/shares/{id}` | 撤销分享 |
| GET | `/shares/{token}` | 分享公开信息（无需登录，用于判断是否需要密码） |
| POST | `/shares/{token}/access` | 校验密码并返回内容（无需登录） |

创建请求体示例：

```json
{
  "target_type": "image",
  "target_id": "<image_id 或 album_id>",
  "password": "可选",
  "expires_in_hours": 24,
  "max_views": 100
}
```

`target_type` 取值为 `image` 或 `album`。`password` 为空表示无需密码；`expires_in_hours` 与 `max_views` 为 0 或省略表示不限制。访问受密码保护的分享时，`POST /shares/{token}/access` 需在请求体携带 `{"password":"…"}`。加密分享通过角色组的 `share` 与 `share_password` 功能开关控制。分享的公开页面为 `/s/{token}`。

### 站点内容（公告 / 举报 / 独立页面）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/announcements` | 已发布公告（无需登录） |
| GET | `/pages/{slug}` | 已发布独立页面（无需登录） |
| POST | `/reports` | 提交举报（`{"image_id":"…","reason":"…","detail":"…"}`，`image_id` 可选） |
| GET/POST | `/admin/announcements` | 公告列表 / 新建 |
| PUT/DELETE | `/admin/announcements/{id}` | 修改 / 删除公告 |
| GET | `/admin/reports` | 举报列表（可带 `?status=pending\|resolved\|rejected`） |
| PATCH | `/admin/reports/{id}` | 处理举报（`{"status":"resolved","note":"…"}`） |
| GET/POST | `/admin/pages` | 独立页面列表 / 新建 |
| PUT/DELETE | `/admin/pages/{id}` | 修改 / 删除页面 |

公告的 `level` 取值为 `info`/`success`/`warning`/`danger`，可置顶（`pinned`）与设为草稿（`published=false`）。独立页面的 `slug` 仅允许小写字母、数字与连字符，公开地址为 `/p/{slug}`，前端以轻量 Markdown 渲染内容。

### 套餐、优惠券、订单与工单

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/plans` | 启用的套餐（无需登录） |
| GET | `/admin/plans` | 全部套餐 |
| POST/PUT/DELETE | `/admin/plans[/{id}]` | 新建 / 修改 / 删除套餐 |
| GET | `/admin/coupons` | 优惠券列表 |
| POST/PUT/DELETE | `/admin/coupons[/{id}]` | 新建 / 修改 / 删除优惠券 |
| POST | `/coupons/validate` | 校验优惠券（`{"code":"…","amount_cents":1000}`） |
| GET/POST | `/orders` | 我的订单 / 下单（管理员可见全部订单） |
| POST | `/orders/{id}/pay` | 完成手动/模拟订单的支付 |
| GET/POST | `/tickets` | 我的工单 / 新建工单（管理员可见全部） |
| GET | `/tickets/{id}` | 工单详情（含消息） |
| POST | `/tickets/{id}/reply` | 回复工单 |
| PATCH | `/admin/tickets/{id}` | 更新工单状态 |

套餐字段：`price_cents`（价格，分）、`duration_days`（0 为永久）、`quota_mb`（0 为不限）、`role_group_id`（购买后应用的角色组，可空）、`active`、`sort_order`。优惠券的 `type` 取值为 `fixed`（金额，分）或 `percent`（百分比），并支持 `min_amount_cents` 门槛、`max_uses` 总量、`per_user_limit` 每用户限次与 `expires_at`。下单请求体为 `{"plan_id":"…","coupon_code":"…","provider":"manual"}`；支付成功（含免费订单）后自动应用套餐的配额与角色组。

当 `duration_days` 大于 0 时，购买会从当前到期时间（未到期时）或现在起顺延有效期，用户可通过 `plan_expires_at`（`GET /auth/me` 返回）查看；到期后服务会（最迟一小时内）自动把该用户回退到默认角色组与默认配额。`duration_days` 为 0 表示永久，会清除到期时间。

### 支付渠道

支付通过 `internal/payment` 的可插拔 `Gateway` 接口实现，内置五种渠道：

| 渠道 | 说明 |
|------|------|
| `manual` | 人工核销：下单后由管理员在后台确认付款 |
| `mock` | 模拟收银台：仅用于开发，普通用户可自助完成支付 |
| `alipay` | 支付宝当面付（扫码），RSA2 签名下单与回调验签 |
| `wechat` | 微信支付 v3 Native 扫码，基于官方 SDK（`wechatpay-apiv3/wechatpay-go`）完成下单与回调验签/解密 |
| `epay` | 易支付（彩虹易支付兼容）聚合支付，MD5 签名下单与异步通知验签 |

`payment.default_gateway` 选择默认渠道。支付宝在 `payment.alipay` 配置 `app_id`、`private_key`（应用私钥）、`public_key`（支付宝公钥）；微信在 `payment.wechat` 配置 `app_id`、`mch_id`、`serial_no`（商户证书序列号）、`private_key`（商户 API 私钥）、`api_v3_key`（32 字节）、`platform_public_key`（微信支付平台证书公钥，**必填**，用于对回调 `Wechatpay-Signature` 做 RSA 验签）与可选的 `platform_serial_no`；缺少平台公钥时微信渠道将拒绝构造，避免未验签的回调确认支付。易支付在 `payment.epay` 配置 `pid`（商户号）、`key`（MD5 密钥）、`gateway_url`（站点根地址），可选 `api_url`、`submit_url`、`pay_type`（`alipay`/`wxpay`/`qqpay` 等）。凭据齐备并置 `enabled: true` 后渠道会在启动时注册。

为避免绕过真实支付，普通用户只能自助完成 `mock` 订单；`manual` 订单需管理员通过 `POST /api/v1/admin/orders/{id}/pay` 核销；`alipay`/`wechat`/`epay` 订单只能由支付回调确认，且回调金额会与订单金额核对。

支付设置也可在后台「系统设置 → 支付设置」中在线维护：`GET /api/v1/admin/payment` 读取（密钥仅返回是否已设置），`PUT /api/v1/admin/payment` 保存并即时生效（密钥字段为空表示保持原值）。支付凭据经主密钥加密后保存在数据库的 `settings` 表中；配置文件中的支付配置作为首次启动的兜底并写入数据库。

下单请求体的 `provider` 字段选择渠道；支付结果回调地址为 `POST /api/v1/payments/{provider}/callback`（支付宝与易支付返回纯文本 `success`，微信返回 200）。`GET /api/v1/payment-gateways` 返回已注册渠道列表。

### 图片安全、云处理、短信与邮件

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
| GET | `/admin/notify/smtp` | 读取 SMTP 邮件渠道设置（密码仅返回是否已设置） |
| PUT | `/admin/notify/smtp` | 保存 SMTP 设置并即时生效（`password` 为空表示保持原密码） |
| GET | `/admin/payment` | 读取支付设置（密钥仅返回是否已设置） |
| PUT | `/admin/payment` | 保存支付设置并即时生效（密钥为空表示保持原值） |

扫描器在 `multipart` 上传与预签名直传确认两个入口都会执行；命中危险内容时返回 HTTP 422 并拒绝入库。通知渠道的兜底实现会把消息写入服务端日志，便于开发调试。管理端「系统设置 → 通知设置」提供 SMTP 配置、渠道查看与发送测试；SMTP 密码经主密钥加密后保存在数据库的 `settings` 表中，保存后邮件渠道即时切换，无需重启。

### 角色组与策略

角色组把一组带类型的策略绑定在一起并分配给普通用户，从而按角色控制资源与功能。每个类型在一个角色组内至多绑定一个策略，同一类型再次绑定时会替换旧策略。内置策略类型：

| 类型 | 作用 | 设置字段（JSON） |
|------|------|------------------|
| `quota` | 存储配额 | `quota_mb`（0 表示不限） |
| `upload` | 单文件大小与媒体类型 | `max_size_mb`、`allowed_mime_types` |
| `rate` | 上传与图片读取限流 | `upload_per_minute`、`upload_burst`、`image_per_minute`、`image_burst` |
| `processing` | 即时图片处理能力 | `enabled`、`max_width`、`max_height`、`default_quality`、`allowed_formats` |
| `feature` | 功能开关集合 | `features`（如 `plaza`、`albums`、`api_tokens`、`batch_upload`、`paste_upload`、`drag_upload`、`embed_code`、`share`、`share_password`） |

策略字段为可选覆盖值：未出现的字段沿用配置文件中的默认值。首次启动会依据 `auth`/`upload`/`processing`/`limits` 配置播种一个「默认角色组」及每个类型的一条策略；历史用户会自动归入默认组。

上传与图片读取的限流会在每次请求时按调用方所属角色组的 `rate` 策略解析执行（访客与匿名请求使用 Guest 组，回退默认组），管理员调整策略后最多约 30 秒生效；`limits.*` 配置仅作为策略默认值的种子。

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/auth/policies` | 当前账户最终生效的策略集合 |
| GET/POST | `/admin/role-groups` | 角色组列表 / 新建 |
| GET/PUT/DELETE | `/admin/role-groups/{id}` | 角色组详情 / 修改 / 删除 |
| POST | `/admin/role-groups/{id}/policies` | 绑定策略（体为 `{"policy_id":"…"}`，同类型替换） |
| DELETE | `/admin/role-groups/{id}/policies/{policyID}` | 解除绑定 |
| GET/POST | `/admin/policies` | 策略列表（可带 `?type=`）/ 新建 |
| GET/PUT/DELETE | `/admin/policies/{id}` | 策略详情 / 修改 / 删除 |

通过 `PATCH /admin/customers/{id}`，请求体可携带 `"role_group_id"`（空字符串表示回退默认组）为普通用户分配角色组；分配时其存储配额会按角色组的 `quota` 策略同步。修改配额策略后，引用该策略的所有角色组的成员配额也会被重新同步。

### 管理接口（需管理员）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/admin/stats` | 实例统计 |
| GET | `/admin/customers` | 普通用户列表 |
| GET/POST | `/admin/admins` | 管理员列表 / 新建管理员 |
| PATCH/DELETE | `/admin/customers/{id}` | 启用/禁用、删除普通用户 |
| PATCH/DELETE | `/admin/admins/{id}` | 启用/禁用、删除管理员 |
| GET/POST | `/admin/storage` | 存储后端列表 / 新建 |
| GET/PUT/DELETE | `/admin/storage/{id}` | 存储后端详情 / 修改 / 删除 |
| POST | `/admin/storage/{id}/activate` | 切换默认存储（热切换） |

存储后端的密钥（S3/七牛的 Secret）以密文入库，接口仅返回「是否已设置」，不回传明文。

## Docker 部署

仓库提供多阶段 `Dockerfile`：前端用 Node 构建，后端交叉编译为纯静态二进制（CGO 关闭），运行在非 root 的 distroless 镜像中，数据统一放在 `/app/data`。

### 构建与运行

```bash
docker build -t axmipic .

docker run -d --name axmipic \
  -p 8080:8080 \
  -v axmipic-data:/app/data \
  -e AXMIPIC_SERVER_BASE_URL=http://localhost:8080 \
  -e AXMIPIC_AUTH_BOOTSTRAP_ADMIN="admin:你的强密码" \
  axmipic
```

- 容器内默认读取 `/app/configs/config.yaml`（由 `configs/config.example.yaml` 生成）；可挂载自定义配置 `-v /path/config.yaml:/app/configs/config.yaml:ro`，或用 `AXMIPIC_*` 环境变量覆盖单项配置。
- `/app/data` 保存 SQLite 数据库、本地上传文件与自动生成的主密钥，务必挂载持久化卷，否则重启后加密密钥会变化。
- 镜像以非 root 用户（uid 65532）运行，`base_url` 请按实际对外地址通过 `AXMIPIC_SERVER_BASE_URL` 设置。

### 使用 GHCR 镜像

CI 在 `main` 分支、`v*` 标签与手动触发时构建多架构（linux/amd64、linux/arm64）镜像并推送到 GitHub Container Registry：

```bash
docker pull ghcr.io/axmishell/axmipic:latest
```

标签策略：`main`、默认分支的 `latest`、`sha-<short>`，以及版本标签对应的 `1.2`、`1.2.3`。Pull Request 只构建（amd64）不推送。

### 使用 Docker Compose

仓库根目录提供 `docker-compose.yml`，默认引用 `ghcr.io/axmishell/axmipic:latest`，并使用**安装向导模式**：

```bash
docker compose up -d
docker compose logs -f
```

启动后访问 **`http://<主机>:8080/install`**，按向导填写数据库连接、站点地址与管理员账号完成初始化。首次启动日志（`docker compose logs -f`）会输出 `install_token`，请在向导的「安装令牌」字段填写；也可通过 `AXMIPIC_INSTALL_TOKEN` 预先指定。

- 向导会把数据库连接等写入具名卷 `axmipic-config`（容器内 `/app/configs/config.yaml`）并初始化目标数据库；数据库切换在**重启容器后**生效。
- 数据（SQLite 数据库、本地上传文件、自动生成的主密钥）保存在具名卷 `axmipic-data`；`docker compose down` 不删除数据，`docker compose down -v` 会连同数据卷一并删除。
- 需要自定义时，可给服务添加 `environment:` 用 `AXMIPIC_*` 覆盖单项配置（如 PostgreSQL 用 `AXMIPIC_DATABASE_DRIVER=postgres`、`AXMIPIC_DATABASE_DSN=host=... port=5432 user=... password=... dbname=... sslmode=disable`），或挂载 `configs/config.yaml`；也可设 `AXMIPIC_INSTALL_DISABLED=true` + `AXMIPIC_AUTH_BOOTSTRAP_ADMIN=admin:<密码>` 跳过向导。
- 镜像为 distroless，未内置 healthcheck，可通过外部探测 `GET /healthz` 做健康检查。

## 开发

```bash
make build   # 构建后端
make vet     # go vet
make test    # go test ./...
make tidy    # go mod tidy
make test-sdk # 运行 Go SDK 测试
make sdk     # 构建并测试全部 SDK
make clean   # 清理 bin/ 与 data/
```

### 前端开发

```bash
cd web
pnpm install
pnpm dev        # 本地开发服务器（自动代理 /api 与 /i 到 :8080）
pnpm typecheck  # vue-tsc 类型检查
pnpm build      # 构建到 ../internal/webui/dist（供 Go 内嵌）
```

修改前端后需重新执行 `pnpm build`，并重新编译 Go 二进制，嵌入的资源才会更新。

### SDK

仓库提供两套覆盖全部 `/api/v1` 接口的客户端 SDK：

| 语言 | 位置 | 说明 |
|------|------|------|
| Go | `sdk/go` | 独立模块 `github.com/AXmishell/axmipic/sdk/go`，零第三方依赖 |
| TypeScript | `sdk/typescript` | 包 `@axmipic/sdk`，基于 `fetch`，支持浏览器与 Node 18+ |

Go SDK 示例：

```go
client, _ := axmipic.New("https://pic.example.com",
    axmipic.WithToken("会话或 API 令牌"))
image, err := client.Upload(ctx, "photo.png", data)
url := axmipic.TransformParams{Width: 400, Fit: "cover"}.TransformURL(image.URL)
```

TypeScript SDK 示例：

```ts
const client = new AxmipicClient({ baseUrl: 'https://pic.example.com' })
await client.login('alice', 'password123')
const image = await client.upload(file.name, file)
const url = transformUrl(image.url, { w: 400, fit: 'cover' })
```

两套 SDK 的方法一一对应，均覆盖认证、上传（含预签名直传）、图片、相册、分享、站点内容、套餐/订单/优惠券、工单与管理员接口，并把错误统一为 `*Error` / `AxmipicError`。详见各自的 `README.md`。

### 使用 libvips 处理器（可选）

默认使用纯 Go 处理器（支持 JPEG/PNG/GIF 输出）。如需 WebP/AVIF 输出与更高性能，可在安装 libvips 后使用：

```bash
go build -tags libvips -o bin/axmipic ./cmd/axmipic
```

### 图片处理驱动

`processing.driver` 选择运行时使用的处理器，未知或不可用的驱动会在启动时回退到 `purego`：

| 驱动 | 说明 | 依赖 |
|------|------|------|
| `purego` | 纯 Go（默认），支持 JPEG/PNG/GIF 输出 | 无 |
| `libvips` | libvips/bimg，额外支持 WebP/AVIF 与更高性能 | `-tags libvips` + CGO + libvips |
| `magick` | ImageMagick 命令行，额外支持 WebP/AVIF | 系统安装 `magick` 或 `convert` |

`GET /api/v1/admin/imaging/drivers` 返回当前二进制可用的驱动与正在使用的处理器。

## 持续集成

仓库包含 GitHub Actions 工作流：

- `.github/workflows/ci.yml`：后端执行 `go mod tidy` 整洁性、`gofmt`、`go vet`、`staticcheck`、构建与竞态测试；前端执行类型检查与构建；并做前后端端到端构建。
- `.github/workflows/docker.yml`：构建多架构容器镜像并推送到 GHCR（Pull Request 仅构建、不推送）。
- `.github/workflows/release.yml`：推送 `v*` 标签（或手动指定标签）时交叉编译多平台二进制并创建 GitHub Release；发行说明由该标签相对上一个标签的提交信息生成（首次发版则取该标签提交的信息）。

## 目录结构

```
cmd/axmipic/          程序入口
internal/
  api/                HTTP 路由与处理器
  auth/               主体身份、密码、JWT、令牌、限流
  config/             配置加载与校验
  imaging/            图片处理（纯 Go / libvips）
  secret/             AES-256-GCM 加密
  server/             HTTP 服务器生命周期
  service/            业务逻辑（账户、上传、处理、存储、管理）
  storage/            存储后端与管理器（本地 / S3 / 七牛）
  store/              数据持久化（GORM）
  webui/              内嵌前端资源
configs/              配置示例
web/                  前端源码（Vue 3）
```

## 许可证

本项目基于 [MIT 许可证](LICENSE) 开源。

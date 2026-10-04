# AXmiPic

轻量、可靠的自托管图床服务。提供图片上传、即时处理、多存储后端与后台管理，后端为单个 Go 二进制，前端为内嵌的单页应用。

## 特性

- **多种上传方式**：后台界面上传、批量上传、粘贴上传、拖拽上传、`multipart` 接口上传、对象存储预签名直传
- **一键嵌入代码**：复制图片的 URL、HTML、BBCode 或 Markdown（受角色功能开关控制）
- **内容寻址与去重**：按内容 `sha256` 生成存储文件名并入库，相同内容自动去重
- **保留原始文件名**：存储层使用重命名（哈希命名）后的文件，数据库中单独记录原文件名、存储文件名与哈希值
- **即时图片处理**：通过 URL 查询参数实时缩放、裁剪、旋转、转码，带 ETag 缓存
- **多存储后端**：本地文件系统、S3 兼容对象存储（AWS S3 / MinIO / Cloudflare R2 / 阿里云 OSS / 腾讯云 COS）、七牛云 Kodo
- **运行中热切换存储**：后台可随时切换默认存储；已有图片按记录自动路由回其原存储读取，无需重启
- **数据库可选**：SQLite（默认，开箱即用）或 PostgreSQL
- **相册与图片广场**：用相册归类图片；图片与相册均可设为公开，出现在跨用户的图片广场与公开相册列表，并提供用户公开资料页
- **角色组与多策略**：按角色组控制用户的存储配额、上传限制、速率限制、图片处理能力与功能开关；策略可复用、按类型覆盖，注册用户自动加入默认角色组
- **账户体系**：管理员与普通用户分表管理；JWT 会话 + 长期 API 令牌
- **配额与限流**：按用户的存储配额，按用户/访客/IP 的速率限制
- **安全**：密钥 AES-256-GCM 加密存储、bcrypt 密码、纵深防御响应头

## 技术栈

| 层 | 技术 |
|----|------|
| 后端 | Go 1.26、chi 路由、GORM |
| 数据库 | SQLite（纯 Go 驱动） / PostgreSQL |
| 图片处理 | 纯 Go（`disintegration/imaging`），可选 libvips（`-tags libvips`） |
| 对象存储 | 本地 / S3 兼容（minio-go） / 七牛云 SDK |
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
```

生产环境与多实例部署请务必显式设置固定的 `auth.jwt_secret`。当日 `jwt_secret` 与 `encryption_key` 都留空时，服务会生成一个主密钥并持久化到磁盘（SQLite 场景为数据库同目录下的 `.axmipic-key`，否则为 `./data/.axmipic-key`），从而保证重启后已加密入库的存储密钥仍可解密；单实例部署可依赖此机制开箱即用。

## 图片处理

图片通过 `/i/<存储键>` 访问，可在查询串中附加处理参数：

```
http://localhost:8080/i/16/30/163053…bce4.png?w=400&h=300&fit=cover&f=webp&q=80
```

| 参数 | 含义 | 取值 |
|------|------|------|
| `w` | 目标宽度 | 像素 |
| `h` | 目标高度 | 像素 |
| `fit` | 缩放策略 | `contain`（默认，等比缩放）/ `cover`（裁剪填充，需同时给出 `w`、`h`） |
| `q` | 输出质量 | 1–100，缺省用 `processing.default_quality` |
| `f` | 输出格式 | `jpeg`/`png`/`gif`/`webp`/`avif`（受 `processing.allowed_formats` 与处理器能力限制） |
| `r` | 旋转角度 | `90`/`180`/`270` |
| `enlarge` | 是否允许放大 | `1`/`true`/`yes`/`on` |

带处理参数的响应会附带 `ETag`，支持 `If-None-Match` 返回 `304`。

## API

所有接口以 `/api/v1` 为前缀，请求头使用 `Authorization: Bearer <令牌>`（会话 JWT 或 API 令牌）。响应统一为：

```json
{ "code": 0, "message": "ok", "data": {} }
```

`code` 为 0 表示成功；错误时 `code` 为对应 HTTP 状态码，`data` 为 `null`。

### 认证

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/auth/register` | 注册普通用户 |
| POST | `/auth/login` | 普通用户登录 |
| POST | `/admin/auth/login` | 管理员登录（独立入口） |
| GET | `/auth/me` | 当前账号信息 |

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
    "key": "16/30/163053….png",
    "url": "http://localhost:8080/i/16/30/163053….png",
    "original_name": "新图.png",
    "filename": "163053….png",
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

## 开发

```bash
make build   # 构建后端
make vet     # go vet
make test    # go test ./...
make tidy    # go mod tidy
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

### 使用 libvips 处理器（可选）

默认使用纯 Go 处理器（支持 JPEG/PNG/GIF 输出）。如需 WebP/AVIF 输出与更高性能，可在安装 libvips 后使用：

```bash
go build -tags libvips -o bin/axmipic ./cmd/axmipic
```

## 持续集成

仓库包含 GitHub Actions 工作流：

- `.github/workflows/ci.yml`：后端执行 `go mod tidy` 整洁性、`gofmt`、`go vet`、`staticcheck`、构建与竞态测试；前端执行类型检查与构建；并做前后端端到端构建。
- `.github/workflows/docker.yml`：构建多架构容器镜像并推送到 GHCR（Pull Request 仅构建、不推送）。
- `.github/workflows/release.yml`：推送 `v*` 标签时交叉编译多平台二进制并创建 GitHub Release。

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

# AXmiPic

轻量、可靠的自托管图床服务。提供图片上传、即时处理、多存储后端与后台管理，后端为单个 Go 二进制，前端为内嵌的单页应用。

## 特性

- **多种上传方式**：后台界面上传、`multipart` 接口上传、对象存储预签名直传
- **内容寻址与去重**：按内容 `sha256` 生成存储文件名并入库，相同内容自动去重
- **保留原始文件名**：存储层使用重命名（哈希命名）后的文件，数据库中单独记录原文件名、存储文件名与哈希值
- **即时图片处理**：通过 URL 查询参数实时缩放、裁剪、旋转、转码，带 ETag 缓存
- **多存储后端**：本地文件系统、S3 兼容对象存储（AWS S3 / MinIO / Cloudflare R2 / 阿里云 OSS / 腾讯云 COS）、七牛云 Kodo
- **运行中热切换存储**：后台可随时切换默认存储；已有图片按记录自动路由回其原存储读取，无需重启
- **数据库可选**：SQLite（默认，开箱即用）或 PostgreSQL
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
  jwt_secret: ""                        # 留空则启动时随机生成（重启后会话失效）
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

生产环境请务必设置固定的 `auth.jwt_secret`（否则每次重启都会使全部会话失效，且多实例部署无法共享）。

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
| GET | `/images` | 图片列表（分页 `page`、`page_size`） |
| GET | `/images/{id}` | 单张图片信息 |
| DELETE | `/images/{id}` | 删除图片 |
| POST | `/tokens` | 创建 API 令牌（明文仅返回一次） |
| GET | `/tokens` | 令牌列表 |
| DELETE | `/tokens/{id}` | 吊销令牌 |

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

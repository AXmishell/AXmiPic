# AXmiPic

轻量、可靠的自托管图床服务。提供图片上传、即时处理、多存储后端与后台管理，后端为单个 Go 二进制，前端为内嵌的单页应用。

> 📖 完整文档见 **[在线文档站](https://axmipic.gpcn.cc/)**。

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

## 文档

完整文档已迁移至在线文档站（VitePress 构建，源码位于 [`docs/`](docs/)）：

| 文档 | 内容 |
|------|------|
| [快速开始](https://axmipic.gpcn.cc/guide/getting-started) | 环境要求、构建运行、创建管理员 |
| [配置说明](https://axmipic.gpcn.cc/guide/configuration) | 服务器、数据库、存储、上传、认证与限流 |
| [图片处理](https://axmipic.gpcn.cc/guide/image-processing) | URL 处理参数、libvips 与处理驱动 |
| [界面与路由](https://axmipic.gpcn.cc/guide/ui-and-routes) | 用户中心 / 管理台路由、Guest 访客、安装向导 |
| [Docker 部署](https://axmipic.gpcn.cc/guide/deployment) | 镜像构建、GHCR、Docker Compose |
| [API 参考](https://axmipic.gpcn.cc/api/) | 全部 `/api/v1` 接口 |
| [开发指南](https://axmipic.gpcn.cc/dev/) | 本地开发、SDK、持续集成、目录结构 |

本地预览文档站：

```bash
cd docs
pnpm install
pnpm dev        # http://localhost:5173/AXmiPic/
```

## Docker 快速运行

```bash
docker run -d --name axmipic \
  -p 8080:8080 \
  -v axmipic-data:/app/data \
  -e AXMIPIC_SERVER_BASE_URL=http://localhost:8080 \
  -e AXMIPIC_AUTH_BOOTSTRAP_ADMIN="admin:你的强密码" \
  ghcr.io/axmishell/axmipic:latest
```

完整说明（镜像构建、GHCR、Docker Compose、安装向导）见 [Docker 部署](https://axmipic.gpcn.cc/guide/deployment)。

## 许可证

本项目基于 [MIT 许可证](LICENSE) 开源。

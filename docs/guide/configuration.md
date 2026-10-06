# 配置说明

配置文件为 YAML，支持通过环境变量覆盖，前缀为 `AXMIPIC_`（例如 `AXMIPIC_SERVER_PORT=9000`）。完整示例见 `configs/config.example.yaml`。

> **引导项 vs 运行设置**：`config.yaml` 只需保留**引导项**——`server`（监听/`base_url`）、`database`、初始 `storage`、`install`、`logging`，以及可选的 `auth.encryption_key`/`auth.jwt_secret`（主密钥）。其余设置（权限开关、上传/处理/限流/安全/短信/维护/站点等）保存在数据库中，可在后台「系统设置」修改；首次启动时以配置文件为兜底播种，之后以数据库为准。**请勿把密钥明文放进 `config.yaml`**：需要回读的密钥（存储密钥、支付密钥、SMTP 密码等）以密文存数据库，主密钥建议通过环境变量或 0600 密钥文件提供。

## 服务器

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  base_url: "http://localhost:8080"   # 生成本地存储图片公开 URL 时使用
  trust_proxy: false                    # 位于可信反向代理之后时才设为 true
  client_ip:
    source: "remote"                    # remote | x-forwarded-for | x-real-ip | cf-connecting-ip | true-client-ip | x-client-ip | forwarded | custom
    header: ""                          # source=custom 时的头名
    trusted_proxies: []                 # 可信代理 CIDR；代理来源下为空则回退 remote
    xff_depth: 0                        # 0=右起第一个不可信地址
  read_timeout_sec: 30
  write_timeout_sec: 30
  shutdown_timeout_sec: 10
```

> `trust_proxy` 为 `true` 时才会解析 `X-Forwarded-For` / `X-Real-IP`。直连部署请保持 `false`，否则客户端可伪造来源 IP 绕过限流。

**客户端真实 IP**：`server.client_ip` 控制限流、访客配额与日志所用的 IP。
- `source: remote`（默认）忽略所有转发头，直接使用对端地址，最安全。
- 位于反向代理/Cloudflare 之后时，选择对应来源（如 `x-forwarded-for`、`cf-connecting-ip`），并**必须**配置 `trusted_proxies`（可信代理 CIDR）。只有当直接对端位于可信网段内才解析转发头，否则回退对端地址，防止伪造。
- `x-forwarded-for` / `forwarded` 默认取「右起第一个不可信地址」；也可用 `xff_depth` 指定右侧跳过的可信代理数量。
- 非法头值会自动回退对端地址。

## 数据库

```yaml
database:
  driver: "sqlite"                      # sqlite | postgres | mysql
  dsn: "./data/axmipic.db"              # sqlite 为文件路径
  # postgres 使用 libpq 连接串或 URL：
  #   host=127.0.0.1 port=5432 user=axmipic password=secret dbname=axmipic sslmode=disable
  #   postgres://axmipic:secret@127.0.0.1:5432/axmipic?sslmode=disable
  # mysql 使用 go-sql-driver 的 DSN：
  #   axmipic:secret@tcp(127.0.0.1:3306)/axmipic
```

SQLite 会自动启用 `busy_timeout` 与 WAL 模式。

MySQL 支持 5.7 及以上（MariaDB 兼容）。连接串中的 `parseTime=true`、`charset=utf8mb4`、`loc=Local` 会自动补齐（已显式指定则不覆盖）。5.7 建议保持默认的 InnoDB 大前缀（`innodb_large_prefix=ON`、`innodb_default_row_format=DYNAMIC`），以支持 `utf8mb4` 下的长索引。

## 存储

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

## 上传与处理

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

## 认证与限流

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

## 访客与安装

```yaml
auth:
  # 允许未登录访客上传（使用 Guest 低权角色组）。
  allow_guest_upload: true
  # 每个访客（按签名 cookie）的配额（MiB）与单文件上限（MiB）。
  guest_quota_mb: 64
  guest_upload_max_mb: 5
  # 单个客户端 IP 在 24 小时窗口内的访客累计上传总量（MiB，0 表示不限）。
  guest_ip_quota_mb: 512

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

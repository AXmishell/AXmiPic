# Docker 部署

仓库提供多阶段 `Dockerfile`：前端用 Node 构建，后端交叉编译为纯静态二进制（CGO 关闭），运行在非 root 的 distroless 镜像中，数据统一放在 `/app/data`。

## 构建与运行

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

## 使用 GHCR 镜像

CI 在 `main` 分支、`v*` 标签与手动触发时构建多架构（linux/amd64、linux/arm64）镜像并推送到 GitHub Container Registry：

```bash
docker pull ghcr.io/axmishell/axmipic:latest
```

标签策略：`main`、默认分支的 `latest`、`sha-<short>`，以及版本标签对应的 `1.2`、`1.2.3`。Pull Request 只构建（amd64）不推送。

## 使用 Docker Compose

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

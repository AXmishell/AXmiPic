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

仓库根目录提供 `docker-compose.yml`，默认引用 `ghcr.io/axmishell/axmipic:latest`，并使用**初始化模式**。完整内容如下（使用具名卷，推荐）：

```yaml
services:
  axmipic:
    image: ghcr.io/axmishell/axmipic:latest
    restart: unless-stopped
    ports:
      - "8080:8080"
    # 数据库运行在宿主机时，可通过 host.docker.internal 访问。
    extra_hosts:
      - "host.docker.internal:host-gateway"
    volumes:
      - axmipic-data:/app/data
      - axmipic-config:/app/configs

volumes:
  axmipic-data:
  axmipic-config:
```

启动：

```bash
docker compose up -d
docker compose logs -f
```

启动后手动访问 **`http://<主机>:8080/install`**，在初始化模式中填写数据库连接、站点地址与管理员账号完成初始化。首页不会自动跳转到该页面，初始化不再需要令牌。

### 1Panel 网络编排

如果使用 1Panel 的 `1panel-network`，并且 PostgreSQL / MySQL 也是同一网络中的容器，可使用以下完整编排。数据库主机填写数据库容器名即可：

```yaml
services:
  axmipic:
    image: ghcr.io/axmishell/axmipic:latest
    restart: unless-stopped
    ports:
      - "127.0.0.1:18105:8080"
    # PostgreSQL / MySQL 运行在宿主机而不是容器中时保留此项，
    # 初始化向导里的数据库主机填写 host.docker.internal。
    extra_hosts:
      - "host.docker.internal:host-gateway"
    volumes:
      - axmipic-data:/app/data
      - axmipic-config:/app/configs
    networks:
      - 1panel-network

volumes:
  axmipic-data:
  axmipic-config:

networks:
  1panel-network:
    external: true
    name: 1panel-network
```

使用前确认网络已存在：

```bash
docker network ls | grep 1panel-network
```

端口 `18105` 可按需修改；若前面有 1Panel 反向代理，保持仅监听 `127.0.0.1` 即可。

### 绑定挂载宿主机目录

如果希望把配置和数据直接放在宿主机目录，可使用以下完整编排（以 1Panel 网络为例）：

```yaml
services:
  axmipic:
    image: ghcr.io/axmishell/axmipic:latest
    restart: unless-stopped
    ports:
      - "127.0.0.1:18105:8080"
    extra_hosts:
      - "host.docker.internal:host-gateway"
    volumes:
      - ./data:/app/data
      - ./config:/app/configs
    networks:
      - 1panel-network

networks:
  1panel-network:
    external: true
    name: 1panel-network
```

但**不能直接挂载空目录**。绑定挂载不会像具名卷那样自动复制镜像内预置的 `config.yaml`，且容器以非 root 用户（uid 65532）运行。请先在编排文件所在目录执行：

```bash
mkdir -p ./config ./data
cp configs/config.example.yaml ./config/config.yaml
chown -R 65532:65532 ./config ./data
```

如果宿主机上没有仓库源码，可从镜像中取出默认配置：

```bash
docker create --name axmipic-tmp ghcr.io/axmishell/axmipic:latest
docker cp axmipic-tmp:/app/configs/config.yaml ./config/config.yaml
docker rm axmipic-tmp
chown -R 65532:65532 ./config ./data
```

`./data` 与 `./config` 都必须对 uid 65532 可写，否则初始化时无法保存数据库文件、上传文件、主密钥或 `config.yaml`。无法调整宿主机目录权限时，请改用具名卷。

- 初始化模式会把数据库连接等写入具名卷 `axmipic-config`（容器内 `/app/configs/config.yaml`）并初始化目标数据库；当前进程会立即接管新连接，无需重启即可登录管理后台，后续重启也会沿用该配置。
- 使用宿主机目录绑定挂载时，请按上文「绑定挂载宿主机目录」先准备配置并授权；空目录会导致配置文件缺失或初始化无法写回配置。
- 数据（SQLite 数据库、本地上传文件、自动生成的主密钥）保存在具名卷 `axmipic-data`；`docker compose down` 不删除数据，`docker compose down -v` 会连同数据卷一并删除。
- 需要自定义时，可给服务添加 `environment:` 用 `AXMIPIC_*` 覆盖单项配置（如 PostgreSQL 用 `AXMIPIC_DATABASE_DRIVER=postgres`、`AXMIPIC_DATABASE_DSN=host=... port=5432 user=... password=... dbname=... sslmode=disable`），或挂载 `configs/config.yaml`；也可设 `AXMIPIC_INSTALL_DISABLED=true` + `AXMIPIC_AUTH_BOOTSTRAP_ADMIN=admin:<密码>` 跳过初始化模式。
- 镜像为 distroless，未内置 healthcheck，可通过外部探测 `GET /healthz` 做健康检查。该接口始终返回 HTTP 200 表示进程存活，响应体中的 `data.status`（`ok` / `degraded`）与 `data.database.status`（`ok` / `error`）反映数据库连接是否可用；首页会据此在数据库异常时展示提示，连接正常时不显示。

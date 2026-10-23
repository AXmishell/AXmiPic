# Docker Deployment

The repository provides a multi-stage `Dockerfile`: the frontend is built with Node, the backend is cross-compiled into a pure static binary (CGO disabled), and it runs in a non-root distroless image with all data placed under `/app/data`.

## Build and run

```bash
docker build -t axmipic .

docker run -d --name axmipic \
  -p 8080:8080 \
  -v axmipic-data:/app/data \
  -e AXMIPIC_SERVER_BASE_URL=http://localhost:8080 \
  -e AXMIPIC_AUTH_BOOTSTRAP_ADMIN="admin:your-strong-password" \
  axmipic
```

- Inside the container, `/app/configs/config.yaml` is read by default (generated from `configs/config.example.yaml`); you can mount a custom configuration with `-v /path/config.yaml:/app/configs/config.yaml:ro`, or override individual settings with `AXMIPIC_*` environment variables.
- `/app/data` holds the SQLite database, local uploads, and the automatically generated master key. Always mount a persistent volume, otherwise the encryption key changes after a restart.
- The image runs as a non-root user (uid 65532); set `base_url` to the actual public address via `AXMIPIC_SERVER_BASE_URL`.

## Using the GHCR image

CI builds multi-architecture (linux/amd64, linux/arm64) images on the `main` branch, `v*` tags, and manual triggers, and pushes them to GitHub Container Registry:

```bash
docker pull ghcr.io/axmishell/axmipic:latest
```

Tag strategy: `main`, `latest` for the default branch, `sha-<short>`, and `1.2` / `1.2.3` for version tags. Pull Requests are built only (amd64) and not pushed.

## Using Docker Compose

The repository root provides `docker-compose.yml`, which references `ghcr.io/axmishell/axmipic:latest` by default and uses **setup mode**. The full contents are as follows (using named volumes, recommended):

```yaml
services:
  axmipic:
    image: ghcr.io/axmishell/axmipic:latest
    restart: unless-stopped
    ports:
      - "8080:8080"
    # When the database runs on the host, it can be reached via host.docker.internal.
    extra_hosts:
      - "host.docker.internal:host-gateway"
    volumes:
      - axmipic-data:/app/data
      - axmipic-config:/app/configs

volumes:
  axmipic-data:
  axmipic-config:
```

Start it:

```bash
docker compose up -d
docker compose logs -f
```

After starting, visit **`http://<host>:8080/install`** manually and fill in the database connection, site address, and administrator account in setup mode to complete initialization. The home page does not redirect to this page, and initialization no longer requires a token.

### 1Panel network orchestration

If you use 1Panel's `1panel-network` and PostgreSQL / MySQL are also containers on the same network, you can use the following complete orchestration. Just enter the database container name as the database host:

```yaml
services:
  axmipic:
    image: ghcr.io/axmishell/axmipic:latest
    restart: unless-stopped
    ports:
      - "127.0.0.1:18105:8080"
    # Keep this when PostgreSQL / MySQL run on the host instead of in a container,
    # and use host.docker.internal as the database host in the setup wizard.
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

Before using it, confirm the network exists:

```bash
docker network ls | grep 1panel-network
```

Port `18105` can be changed as needed; if there is a 1Panel reverse proxy in front, keeping it listening on `127.0.0.1` only is fine.

### Bind-mounting host directories

If you want to keep configuration and data directly in host directories, you can use the following complete orchestration (using the 1Panel network as an example):

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

However, you **cannot mount empty directories directly**. Unlike named volumes, bind mounts do not automatically copy the `config.yaml` bundled in the image, and the container runs as a non-root user (uid 65532). First run the following in the directory containing the compose file:

```bash
mkdir -p ./config ./data
cp configs/config.example.yaml ./config/config.yaml
chown -R 65532:65532 ./config ./data
```

If the repository source is not available on the host, extract the default configuration from the image:

```bash
docker create --name axmipic-tmp ghcr.io/axmishell/axmipic:latest
docker cp axmipic-tmp:/app/configs/config.yaml ./config/config.yaml
docker rm axmipic-tmp
chown -R 65532:65532 ./config ./data
```

Both `./data` and `./config` must be writable by uid 65532, otherwise setup cannot save the database file, uploads, master key, or `config.yaml`. If you cannot adjust host directory permissions, use named volumes instead.

- Setup mode writes the database connection and other settings into the named volume `axmipic-config` (`/app/configs/config.yaml` inside the container) and initializes the target database; the current process immediately takes over the new connection, so you can log in to the admin console without a restart, and subsequent restarts reuse that configuration.
- When using host directory bind mounts, prepare and grant permissions on the configuration as described in "Bind-mounting host directories" above; empty directories cause the configuration file to be missing or setup to fail when writing back the configuration.
- Data (SQLite database, local uploads, automatically generated master key) is stored in the named volume `axmipic-data`; `docker compose down` does not delete the data, while `docker compose down -v` deletes the data volume as well.
- For customization, add `environment:` to the service to override individual settings with `AXMIPIC_*` (for example, for PostgreSQL use `AXMIPIC_DATABASE_DRIVER=postgres`, `AXMIPIC_DATABASE_DSN=host=... port=5432 user=... password=... dbname=... sslmode=disable`), or mount `configs/config.yaml`; you can also set `AXMIPIC_INSTALL_DISABLED=true` + `AXMIPIC_AUTH_BOOTSTRAP_ADMIN=admin:<password>` to skip setup mode.
- The image is distroless and has no built-in healthcheck; you can perform health checks by probing `GET /healthz` externally. This endpoint always returns HTTP 200 to indicate the process is alive, and `data.status` (`ok` / `degraded`) and `data.database.status` (`ok` / `error`) in the response body reflect whether the database connection is available; the home page shows a warning when the database is unhealthy and displays nothing when the connection is fine.

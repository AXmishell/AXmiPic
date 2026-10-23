# Configuration

The configuration file is YAML and supports overrides through environment variables with the `AXMIPIC_` prefix (for example `AXMIPIC_SERVER_PORT=9000`). See `configs/config.example.yaml` for a complete example.

> **Bootstrap items vs. runtime settings**: `config.yaml` only needs to keep **bootstrap items** — `server` (listen/`base_url`), `database`, the initial `storage`, `install`, `logging`, and optionally `auth.encryption_key`/`auth.jwt_secret` (the master key). All other settings (permission toggles, upload/processing/rate-limit/security/SMS/maintenance/site, etc.) are stored in the database and can be changed in the admin console under "System Settings"; on first start the configuration file is used as a fallback to seed them, and afterwards the database takes precedence. **Do not put secrets in plaintext in `config.yaml`**: secrets that must be read back (storage keys, payment keys, SMTP passwords, etc.) are stored encrypted in the database, and the master key should be provided through an environment variable or a 0600 key file.

## Server

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  base_url: "http://localhost:8080"   # used when generating public URLs for locally stored images
  trust_proxy: false                    # set to true only when behind a trusted reverse proxy
  client_ip:
    source: "remote"                    # remote | x-forwarded-for | x-real-ip | cf-connecting-ip | true-client-ip | x-client-ip | forwarded | custom
    header: ""                          # header name when source=custom
    trusted_proxies: []                 # trusted proxy CIDRs; if empty with a proxy source, falls back to remote
    xff_depth: 0                        # 0=first untrusted address from the right
  read_timeout_sec: 30
  write_timeout_sec: 30
  shutdown_timeout_sec: 10
  pprof_enabled: false                   # mounts pprof protected by admin authentication (/api/v1/admin/pprof/)
```

> `X-Forwarded-For` / `X-Real-IP` are only parsed when `trust_proxy` is `true`. Keep it `false` for direct deployments, otherwise clients can forge their source IP to bypass rate limits.

## Frontend hot reload (development)

HMR mode is enabled through the configuration file and is **disabled by default** (the frontend artifacts embedded at compile time are used):

```yaml
web:
  hot_reload:
    enabled: false                    # change to true for local development
    target: "http://127.0.0.1:5173"   # Vite dev server address
```

When enabled, the backend reverse-proxies unmatched frontend requests (pages, modules, and the HMR WebSocket) to `target`, so the browser only needs to access the backend port (default `8080`) and frontend changes hot-reload instantly without rebuilding or refreshing. `/api`, `/i/`, and `/healthz` are still handled directly by the backend and are not forwarded.

Usage:

```bash
cd web && pnpm dev        # start the Vite dev server first
# set web.hot_reload.enabled: true in configs/config.yaml, then start the backend
./bin/axmipic -config configs/config.yaml
```

> For local development only; do not enable it in production. In production, remove this section or keep it `false`, and run `make build` before releasing to re-embed the frontend artifacts. You can also override it with environment variables: `AXMIPIC_WEB_HOT_RELOAD_ENABLED=true`, `AXMIPIC_WEB_HOT_RELOAD_TARGET=http://127.0.0.1:5173`.

**Real client IP**: `server.client_ip` controls the IP used for rate limiting, guest quotas, and logs.
- `source: remote` (default) ignores all forwarding headers and uses the peer address directly, which is the safest option.
- Behind a reverse proxy/Cloudflare, choose the corresponding source (such as `x-forwarded-for`, `cf-connecting-ip`) and you **must** configure `trusted_proxies` (trusted proxy CIDRs). Forwarding headers are only parsed when the direct peer is within a trusted network; otherwise the peer address is used to prevent forgery.
- `x-forwarded-for` / `forwarded` use "the first untrusted address from the right" by default; `xff_depth` can also specify how many trusted proxies to skip on the right.
- Invalid header values automatically fall back to the peer address.

## Database

```yaml
database:
  driver: "sqlite"                      # sqlite | postgres | mysql
  dsn: "./data/axmipic.db"              # for sqlite this is a file path
  # postgres uses a libpq connection string or URL:
  #   host=127.0.0.1 port=5432 user=axmipic password=secret dbname=axmipic sslmode=disable
  #   postgres://axmipic:secret@127.0.0.1:5432/axmipic?sslmode=disable
  # mysql uses the go-sql-driver DSN:
  #   axmipic:secret@tcp(127.0.0.1:3306)/axmipic
  # Connection pool limits (defaults 25 / 5 / 30 minutes). 0 means unlimited (max_idle_conns of 0
  # means no idle connections are kept).
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime_minutes: 30
```

SQLite automatically enables `busy_timeout` and WAL mode.

MySQL 5.7 and later is supported (MariaDB compatible). `parseTime=true`, `charset=utf8mb4`, and `loc=Local` are automatically appended to the connection string (explicitly specified values are not overridden).

The server-side default character set for MySQL 5.7 is `latin1` (`utf8mb4` since 8.0). When creating tables, the application explicitly adds `DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`, so Chinese text is stored correctly even when the database default charset is `latin1`. Creating the database with utf8mb4 is still recommended:

```sql
CREATE DATABASE axmipic CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

If the database contains latin1 tables **created by an older version**, the startup log emits a warning and lists the table names. These must be converted manually (this rewrites the entire table, so run it during off-peak hours):

```sql
ALTER TABLE `images` CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
-- do the same for the other tables listed in the warning
```

For 5.7, keep the default InnoDB large prefix (`innodb_large_prefix=ON`, `innodb_default_row_format=DYNAMIC`) to support long indexes under `utf8mb4` (such as the `varchar(255)` unique index on `images.key`).

## Storage

```yaml
storage:
  driver: "local"                       # local | s3 | qiniu | oss | cos
  local:
    root: "./data/uploads"              # local file storage directory
  s3:
    endpoint: "https://s3.amazonaws.com"
    region: "us-east-1"
    bucket: "axmipic"
    access_key_id: ""
    secret_access_key: ""
    secure: true
    use_path_style: false
    public_base_url: ""                 # optional, CDN or custom public domain
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
  oss:                                  # Alibaba Cloud OSS
    endpoint: ""                        # optional; derived from region when empty
    region: "cn-hangzhou"               # oss-cn-hangzhou is also accepted
    bucket: "axmipic"
    access_key_id: ""
    access_key_secret: ""
    domain: ""                          # optional, custom/CDN domain
    private: false
    use_https: true
    presign_expiry_sec: 900
  cos:                                  # Tencent Cloud COS
    bucket: "axmipic-1250000000"        # name-APPID
    region: "ap-guangzhou"
    secret_id: ""
    secret_key: ""
    domain: ""                          # optional, custom/CDN domain
    private: false
    use_https: true
    presign_expiry_sec: 900
```

Storage configured in the configuration file always serves as the fallback backend. Additional backends can be added dynamically in the admin console under "Storage Configuration" and switched at runtime.

**Storage pools**: under "Storage Configuration → Storage Pools", group multiple backends into a pool and select the current write pool. The pool only determines which backend new objects are written to; existing images are always read from their original backend according to `images.storage_id`. Supported strategies:

- `weighted_rr`: weighted round-robin by member weight (default);
- `priority`: prefer members with a lower `Priority` value, with weighted round-robin within the same priority.

Before enabling a storage pool, the system checks whether all historical images have a `storage_id`; on the first upgrade, old images are automatically assigned to the previous current backend. If pool members are disabled or no members are available, uploads fail instead of silently writing to the fallback storage.

## Upload and processing

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
  cache_mb: 64                          # in-process cache for derived images (MiB), 0 disables it
  max_concurrency: 0                    # concurrent renders, 0 means one per CPU core
  max_decode_pixels: 16000000           # per-image decodable pixel limit (prevents decompression bombs / caps memory)
  max_render_memory_mb: 256             # total memory budget for concurrent renders (MiB)
```

## Runtime and memory

Image decoding is the main source of transient memory. The following options bound the peak and let the GC return memory more aggressively:

```yaml
processing:
  # Lowering max_concurrency, max_decode_pixels, and max_render_memory_mb can significantly
  # reduce peak memory; oversized originals that are rejected return an error instead of
  # dragging down the process.
  max_concurrency: 2
  max_decode_pixels: 16000000
  max_render_memory_mb: 256

runtime:
  # Maps to GOMEMLIMIT: the process soft memory limit (MiB). 0 means automatic: if a container
  # cgroup memory limit is detected, 90% of it is used; otherwise the Go default (no limit) is
  # kept. In containers, explicitly setting 60–75% of the container limit is recommended.
  memory_limit_mb: 0
  # Maps to GOGC: 50 saves memory at the cost of more CPU; 0 keeps the Go default (100).
  gc_percent: 0

plugins:
  # Linear memory limit per WASM plugin (MiB); 0 uses the built-in default of 64.
  memory_limit_mb: 64
  # Idle auto-unload (minutes): plugins that are enabled but have not been called for longer
  # than this are unloaded to free memory and transparently reloaded on the next call.
  # 0 disables it.
  idle_unload_minutes: 0
  # Startup loading mode: lazy (default) only registers without instantiating and loads on first
  # use; eager instantiates all enabled plugins at startup.
  startup: "lazy"
  # Plugins to warm up immediately regardless of startup mode (for example an active SMS channel),
  # avoiding a cold start on the first call.
  preload: []
```

> Plugins are either **built-in** or **external**: built-in plugins ship with the host build (trusted by default, run in-process, no sandbox required), while external plugins must use `runtime: wasm` and run in the wazero sandbox. The legacy `email.*` / `sms.*` configuration is only used for the initial migration; configure them centrally under "Plugin Marketplace" in the admin console afterwards.

> Plugin enabling/pausing and idle unloading are all dynamic and require no restart: pausing immediately releases the plugin's wazero runtime, and idle unloading rebuilds it on demand at the next call. Only "enabled" plugins are instantiated at startup, so paused plugins no longer consume memory at startup. The admin console's "Plugins" page shows each plugin's enabled/loaded status; the `memory_bytes` field of `GET /api/v1/admin/plugins/installed` shows the linear memory usage of loaded WASM plugins.

## Notification channels (built-in plugins)

SMS and email configuration has moved to the "Plugin Marketplace"; `sms` / `email` in the configuration file are only used as migration seeds on first start:

```yaml
plugins:
  # External plugin directory; built-in plugins do not need this directory.
  dir: "./plugins"

# The following are legacy bootstrap items, read only during the initial migration; plugin configuration takes precedence afterwards.
sms:
  enabled: false
email:
  enabled: false
```

Built-in notification plugins:

- `builtin-sms-log` / `builtin-email-log`: log channels for debugging, the current channel by default;
- `builtin-sms-http`: generic HTTP SMS gateway (`provider`/`endpoint`/`method`);
- `builtin-email-smtp`: SMTP email (`host`/`port`/`username`/`password`/`from`/`use_tls`).

Configure the fields in the admin console's "Plugin Marketplace" and use "Set as current" to switch the current SMS/email channel; you can also call
`PUT /api/v1/admin/plugins/active`. Ciphertext fields are stored encrypted with AES-256-GCM and are never echoed back by the API.


> You can also override with the standard environment variables `GOMEMLIMIT` / `GOGC`, which are equivalent. When processing large images in production, prefer building with `-tags libvips` (better than the pure Go processor in both memory and CPU).

With `server.pprof_enabled` enabled, you can retrieve heap/goroutine/profile and other profiling data through the admin endpoint `GET /api/v1/admin/pprof/` (requires an admin token); disable it promptly once troubleshooting is done.

## Authentication and rate limiting

```yaml
auth:
  jwt_secret: ""                        # generated and persisted automatically when empty (see below)
  encryption_key: ""                    # optional: only used to encrypt storage backend secrets; falls back to jwt_secret when empty
  session_ttl_hours: 24
  allow_registration: true
  require_auth: true                    # when true, only logged-in users can upload
  default_quota_mb: 1024                # per-user quota, 0 means unlimited
  bootstrap_admin: ""                   # seeded only when no administrator exists, format "username:password"

limits:
  upload_per_minute: 30
  upload_burst: 5
  guest_per_minute: 6
  guest_burst: 2
  image_per_minute: 600                 # per-IP rate limit for public image reads/processing
  image_burst: 120
  share_per_minute: 30                  # per-IP rate limit for share access (password checks), prevents brute force
  share_burst: 10
```

For production and multi-instance deployments, always set a fixed `auth.jwt_secret` explicitly. When both `jwt_secret` and `encryption_key` are empty, the service generates a master key and persists it to disk (in the SQLite case, `.axmipic-key` in the same directory as the database; otherwise `./data/.axmipic-key`), so storage secrets already encrypted in the database can still be decrypted after a restart; single-instance deployments can rely on this mechanism and work out of the box.

## Guests and setup

```yaml
auth:
  # Allow unauthenticated guests to upload (using the low-privilege Guest role group).
  allow_guest_upload: true
  # Quota (MiB) and per-file limit (MiB) for each guest (identified by a signed cookie).
  guest_quota_mb: 64
  guest_upload_max_mb: 5
  # Total guest upload volume (MiB, 0 means unlimited) per client IP within a 24-hour window.
  guest_ip_quota_mb: 512

install:
  # The instance is considered initialized once this lock file exists; delete it and restart to
  # enter setup mode again.
  lock_file: "./data/install.lock"
  # Path of the configuration file that setup mode rewrites.
  config_path: "./configs/config.yaml"
  # Completely skip the setup check (testing / container orchestration). Frontend routes do not
  # detect the lock file automatically and display the site normally even when uninitialized;
  # users must visit /install manually.
  disabled: false
```

## Admin secure entry

The secure entry is enabled by default in the admin console under "System Settings → Runtime Settings" and stored in the database. The entry format is:

```text
https://<site-address>/admin/entry/<32-byte-random-token>
```

When enabled:

- the regular `/admin/login` returns 404;
- without the entry cookie, the admin login API also returns 404;
- visiting the secure entry sets an entry cookie valid for 10 minutes (HttpOnly) and then redirects to `/admin/login`;
- the entry cookie is cleared after a successful login;
- enabling, disabling, and rotating all require the current password; when TOTP is enabled, a dynamic verification code is also required.

The token is stored encrypted with AES-GCM in the database. If you forget the entry or need to disable it urgently, force an override through the configuration file or environment variables:

```yaml
admin_entry:
  # When true, this configuration takes precedence; the secure entry can no longer be changed in the admin console.
  override: true
  enabled: false
  # Required when enabled=true, 16-128 URL-safe characters.
  token: ""
```

Corresponding environment variables:

```bash
AXMIPIC_ADMIN_ENTRY_OVERRIDE=true
AXMIPIC_ADMIN_ENTRY_ENABLED=false
# To restore/force a specific entry:
# AXMIPIC_ADMIN_ENTRY_ENABLED=true
# AXMIPIC_ADMIN_ENTRY_TOKEN=<token>
```

After restoring, regenerate the entry from the admin console and remove the emergency override configuration.

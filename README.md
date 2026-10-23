# AXmiPic

A lightweight, reliable self-hosted image hosting service. It provides image uploads, on-the-fly processing, multiple storage backends, and an admin console, with a single Go binary backend and an embedded single-page frontend.

> 🌐 English · [简体中文](README-CN.md) · [日本語](README-JA.md)
>
> 📖 Full documentation: **[Docs](https://axmipic.gpcn.cc/)** (English, default) · **[日本語](https://axmipic.gpcn.cc/ja/)** · **[中文文档](https://axmipic.gpcn.cc/zh/)**.

## Features

- **Multiple upload methods**: admin UI upload, batch upload, paste upload, drag-and-drop upload, `multipart` API upload, and presigned direct upload to object storage
- **X link import**: paste an X post URL to resolve the author and images, using the free fxtwitter API or the official X API; choose to publish directly to the plaza or keep images private first, with author and original-post attribution preserved automatically
- **One-click embed code**: copy an image's URL, HTML, BBCode, or Markdown (controlled by role feature toggles)
- **Image and album sharing**: generate share links with an optional access password, expiry, and maximum visit count; the public share page is `/s/{token}`
- **Announcements and standalone pages**: admins can publish announcements (with pinning and severity levels) shown on the dashboard, and maintain standalone pages publicly accessible at `/p/{slug}`
- **Report management**: users report images; admins handle or dismiss reports in the admin console
- **Plans and billing**: plans (price, validity period, quota, role group) and coupons (fixed/percentage, threshold, usage limit, validity), plus checkout, order management, and pluggable payment channels
- **First-party payment integrations**: Alipay Face-to-Face Payment (RSA2 signing/verification) and WeChat Pay v3 (SHA256-RSA signing, AES-GCM callback decryption), along with manual and mock channels
- **Image safety**: a pluggable upload content scanner with a built-in whitelist and dangerous magic-number detection, rejecting executable content disguised as images
- **Notification system**: SMS and email are sent through plugins; the built-in log/HTTP SMS and SMTP email plugins are trusted by default and require no sandbox, and support test sending from the admin console
- **Built-in and external plugins**: built-in plugins are compiled with the host and run in-process; external plugins must run in a WASM sandbox. The admin plugin marketplace installs, configures, tests, and switches the active channel in one place
- **Ticket system**: users submit tickets and reach support; admins reply and close them
- **Random storage keys and deduplication**: stored files use date-partitioned, random, unguessable keys; content `sha256` is recorded separately, so the same owner uploading identical content is deduplicated automatically, while different owners each hold an independent object
- **Original filenames preserved**: the storage layer uses randomly named files, while the database separately records the original filename, stored filename, and hash
- **Multiple image processing drivers**: pure Go (default), libvips (`-tags libvips`), and ImageMagick (the `magick` command), selected by configuration at runtime with fallback when unavailable
- **On-the-fly image processing**: real-time resize, crop, rotate, flip, grayscale, blur, sharpen, text watermark, and transcode via URL query parameters, with ETag caching
- **Multiple storage backends**: local filesystem, S3-compatible object storage (AWS S3 / MinIO / Cloudflare R2), Alibaba Cloud OSS, Tencent Cloud COS, and Qiniu Cloud Kodo
- **Hot storage switching at runtime**: switch the default storage at any time from the admin console; existing images are automatically routed back to their original storage for reads, with no restart required
- **Storage pools**: combine multiple storage backends into a pool and place new objects using weighted round-robin or priority policies; pools only affect writes, while existing images are still read from their records
- **Optional database**: SQLite (default, works out of the box), PostgreSQL, or MySQL 5.7+ (MariaDB compatible)
- **Albums and the Image Plaza**: organize images into albums; both images and albums can be made public, appearing in the cross-user Image Plaza and public album listings, with public user profile pages
- **Role groups and multiple policies**: role groups control a user's storage quota, upload limits, rate limits, image processing capabilities, and feature toggles; policies are reusable and overridable by type, and newly registered users automatically join the default role group
- **Account system**: admins and regular users are managed in separate tables; JWT sessions + long-lived API tokens
- **Separated interfaces and setup mode**: a public landing page + user center (`/user`) separated from the admin console (`/admin`); a built-in low-privilege Guest visitor and a setup mode entered manually at `/install`
- **UI internationalization**: the frontend provides Chinese and English via vue-i18n; Element Plus built-in strings, page titles, and the language preference switch together
- **Quotas and rate limiting**: per-user storage quotas and per-user/guest/IP rate limits
- **Security**: secrets encrypted at rest with AES-256-GCM, bcrypt passwords, and defense-in-depth response headers

## Tech stack

| Layer | Technology |
|-------|------------|
| Backend | Go 1.26, chi router, GORM |
| Database | SQLite (pure Go driver) / PostgreSQL / MySQL 5.7+ (MariaDB compatible) |
| Image processing | Pure Go (`disintegration/imaging`), optional libvips (`-tags libvips`) |
| Object storage | Local / S3-compatible (minio-go) / Alibaba Cloud OSS SDK / Tencent Cloud COS SDK / Qiniu SDK |
| Payments | Official WeChat Pay SDK (`wechatpay-apiv3/wechatpay-go`), Alipay RSA2 (native), YiPay (native) |
| Frontend | Vue 3, Pinia, Vue Router, Element Plus, TypeScript, Vite |

## Quick start

### Requirements

- Go 1.26 or later
- Node.js 22+ and pnpm 9 (required to build the frontend; alternatively use a Docker/CI binary with the frontend already embedded)

Frontend assets are **not committed to the repository**; they are generated into `internal/webui/dist` during the build. `make build` builds the frontend and then compiles the backend. To build only the backend (with no Node environment), prepare an `internal/webui/dist` directory first (for example from a CI/release artifact) and run `make build-go`.

### Build and run

```bash
# Full build (frontend + backend), output at bin/axmipic
make build

# Build backend only (reuse existing internal/webui/dist; the web UI returns 503 without it)
# make build-go

# Build frontend only
# make web

# Run with the example configuration
cp configs/config.example.yaml configs/config.yaml
./bin/axmipic -config configs/config.yaml
```

Or run directly:

```bash
make run            # build the frontend, then go run ./cmd/axmipic -config configs/config.example.yaml
```

It listens on `0.0.0.0:8080` by default; open `http://localhost:8080` to access the console.

For local frontend development, you can enable HMR (disabled by default): run `pnpm dev` in `web/`, set `web.hot_reload.enabled: true` in the configuration file (`target` defaults to `http://127.0.0.1:5173`), then open `http://localhost:8080` to get frontend hot reload without rebuilding. See [Configuration](https://axmipic.gpcn.cc/guide/configuration#frontend-hot-reload-development) for details.

### Create an administrator

There are two ways:

1. **Seed from configuration (recommended)**: set `auth.bootstrap_admin: "username:password"` in `configs/config.yaml`. It only takes effect when the admins table is empty:

   ```yaml
   auth:
     bootstrap_admin: "admin:your-strong-password"
   ```

2. **Create in the admin console**: sign in with an existing administrator and create one under **Users → Administrators**.

> For the first deployment, configure `auth.bootstrap_admin` before starting to avoid having no administrator available. The registration entry only creates regular users.

## Documentation

The full documentation has moved to the online docs site (built with VitePress, source in [`docs/`](docs/)):

| Document | Contents |
|----------|----------|
| [Getting started](https://axmipic.gpcn.cc/guide/getting-started) | Requirements, build and run, creating an administrator |
| [Configuration](https://axmipic.gpcn.cc/guide/configuration) | Server, database, storage, upload, authentication, and rate limiting |
| [Image processing](https://axmipic.gpcn.cc/guide/image-processing) | URL processing parameters, libvips, and processing drivers |
| [UI and routes](https://axmipic.gpcn.cc/guide/ui-and-routes) | User center / admin routes, Guest visitors, setup mode |
| [Docker deployment](https://axmipic.gpcn.cc/guide/deployment) | Image builds, GHCR, Docker Compose |
| [API reference](https://axmipic.gpcn.cc/api/) | All `/api/v1` endpoints |
| [Development guide](https://axmipic.gpcn.cc/dev/) | Local development, SDKs, CI, directory structure |

To preview the docs locally:

```bash
cd docs
pnpm install
pnpm dev        # http://localhost:5173/
```

## Run with Docker

```bash
docker run -d --name axmipic \
  -p 8080:8080 \
  -v axmipic-data:/app/data \
  -e AXMIPIC_SERVER_BASE_URL=http://localhost:8080 \
  -e AXMIPIC_AUTH_BOOTSTRAP_ADMIN="admin:your-strong-password" \
  ghcr.io/axmishell/axmipic:latest
```

For full instructions (image builds, GHCR, Docker Compose, setup mode), see [Docker deployment](https://axmipic.gpcn.cc/guide/deployment).

## License

This project is released under the [MIT License](LICENSE).

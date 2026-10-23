# Directory structure

```
cmd/axmipic/          program entry point
internal/
  api/                HTTP routes and handlers
  auth/               principal identity, passwords, JWT, tokens, rate limiting
  config/             configuration loading and validation
  imaging/            image processing (pure Go / libvips)
  plugin/             runtime plugin framework (WASM sandbox, host capabilities, registry)
  secret/             AES-256-GCM encryption
  server/             HTTP server lifecycle
  service/            business logic (accounts, uploads, processing, storage, administration)
  storage/            storage backends and manager (local / S3 / Qiniu)
  store/              data persistence (GORM)
  webui/              embedded frontend assets
configs/              example configuration
web/                  frontend source (Vue 3)
```

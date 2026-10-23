# Development

```bash
make build    # Full build: frontend + backend binary
make build-go # Backend only (reuses the existing internal/webui/dist)
make web      # Frontend only, built into internal/webui/dist
make run      # Build the frontend first, then run
make vet     # go vet
make test    # go test ./...
make tidy    # go mod tidy
make test-sdk # Run the Go SDK tests
make sdk     # Build and test all SDKs
make clean   # Clean bin/ and data/
```

## Frontend development

```bash
cd web
pnpm install
pnpm dev        # Vite dev server (when hitting :5173 directly, /api and /i are proxied to :8080)
pnpm typecheck  # vue-tsc type checking
```

By default the backend serves the frontend artifacts embedded at compile time. To get **HMR (hot module replacement)** during development, enable hot-reload mode in the configuration file (it is off by default) and start Vite first:

```yaml
# configs/config.yaml
web:
  hot_reload:
    enabled: true
    target: "http://127.0.0.1:5173"
```

```bash
cd web && pnpm dev              # Terminal 1: Vite
./bin/axmipic -config configs/config.yaml   # Terminal 2: backend (defaults to :8080)
```

Once enabled, the backend reverse-proxies pages, modules, and the HMR WebSocket to Vite, so you can visit
`http://localhost:8080` and see changes live without rebuilding. `/api`, `/i/`, and
`/healthz` are always handled by the backend. This mode is for local development only; keep it disabled in
production (or remove the configuration entirely) and run `make build` to embed the latest frontend artifacts.

Build the frontend with `make web` (it builds into `../internal/webui/dist` and restores the placeholder file
`.gitkeep` for Go embedding); running `pnpm build` directly clears `dist`, which deletes the placeholder file.
After changing the frontend, re-run `make build` (or `make web && make build-go`) so the embedded assets are updated.

UI copy is managed with vue-i18n, with Chinese and English support. See [Frontend internationalization (i18n)](/dev/i18n).

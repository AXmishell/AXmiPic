# Getting Started

## Requirements

- Go 1.26 or later
- Node.js 22+ and pnpm 9 (needed to build the frontend; `make build` builds it automatically)

Frontend artifacts are not committed to the repository; they are generated into `internal/webui/dist` at build time. To build only the backend (no Node), reuse existing frontend artifacts and run `make build-go`.

## Build and run

```bash
# Full build (frontend + backend)
make build          # output is at bin/axmipic

# Backend only (reuses the existing internal/webui/dist)
# make build-go

# Frontend only
# make web

# Run with the example configuration
cp configs/config.example.yaml configs/config.yaml
./bin/axmipic -config configs/config.yaml
```

Or run directly:

```bash
make run            # builds the frontend first, then runs
```

It listens on `0.0.0.0:8080` by default; open `http://localhost:8080` to access the site. On first deployment, visit `http://localhost:8080/install` manually to enter setup mode and initialize the application and database; the home page does not redirect automatically.

## Creating an administrator

There are two ways:

1. **Configuration file seeding** (recommended): set `auth.bootstrap_admin: "username:password"` in `configs/config.yaml`. It only takes effect when the admins table is empty:

   ```yaml
   auth:
     bootstrap_admin: "admin:your-strong-password"
   ```

2. **Create in the admin console**: after logging in as an existing administrator, create a new administrator on the "User Management → Administrators" tab.

> On first deployment, configure `auth.bootstrap_admin` before starting to avoid having no administrator available. The registration entry point only creates regular users.

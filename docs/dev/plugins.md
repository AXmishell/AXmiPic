# Plugin system

AXmiPic plugins come in two kinds: **built-in plugins** and **external plugins**:

| Type | How it runs | Trust level | Use cases |
|------|----------|----------|----------|
| Built-in (`builtin`) | Compiled with the host, in-process Go `Provider` | Trusted by default, **no sandbox**, resident in memory | Low-resource, stable basic channels (log SMS, HTTP SMS, SMTP email) |
| External (`external`) | Loaded from `plugins.dir`, wazero WASM sandbox | Untrusted, **sandbox required**, lazy-loaded by default | Third-party SMS providers, and future payment/storage channels |

Built-in plugins do not read `plugin.yaml`, do not consume WASM runtime memory, and cannot be uninstalled, replaced, or paused;
external plugins may only use `runtime: wasm`, and a manifest declaring `process` or any other runtime is rejected outright.

Both kinds share the same `category + descriptor` contract and the same configuration/enable/invoke interfaces.

## Directory structure

```
plugins/                   plugin root directory (default; configurable via plugins.dir)
  smsbao/
    plugin.yaml            manifest
    plugin.wasm            build artifact
internal/plugin/           host framework (loading, registration, sandbox, host capabilities)
sdk/plugin-go/             Go plugin SDK
  examples/smsbao/         example plugin (SMSBao, wasm)
  examples/aliyun-sms/     example plugin (Alibaba Cloud SMS, wasm)
  examples/tencent-sms/    example plugin (Tencent Cloud SMS, wasm)
```

Each `plugins/<name>/` subdirectory is a single plugin, described by `plugin.yaml`.

## Manifest `plugin.yaml`

```yaml
name: smsbao              # Plugin name (unique)
version: 0.1.0
category: notify.sms      # Domain namespace: notify.sms / notify.email / payment / ...
runtime: wasm             # Currently only wasm is supported
entry: plugin.wasm        # Entry file, relative to the plugin directory
abi: 1                    # Must match the host version
capabilities:             # Capability declarations; anything undeclared is unavailable
  http:
    hosts: ["api.smsbao.com", "*.example.com"]
# checksum: sha256:<hex>  # Optional; verifies the integrity of the entry file
```

## Writing a plugin (Go SDK)

A plugin implements the `Provider` interface and registers itself in `init`:

```go
package main

import plugin "github.com/AXmishell/axmipic/sdk/plugin-go"

type myPlugin struct{ user string }

func (p *myPlugin) Describe() plugin.Descriptor {
    return plugin.Descriptor{Title: "Example", Fields: []plugin.Field{
        {Key: "user", Label: "Account", Required: true},
        {Key: "secret", Label: "Secret", Type: plugin.FieldPassword, Secret: true},
    }}
}

func (p *myPlugin) Configure(cfg map[string]string) error { p.user = cfg["user"]; return nil }

func (p *myPlugin) Invoke(op string, in []byte) ([]byte, error) {
    // Makes an allowlist-constrained HTTP request through the host
    resp, err := plugin.HTTPDo(plugin.HTTPRequest{Method: "GET", URL: "https://api.example.com/send"})
    if err != nil { return nil, err }
    _ = resp
    return []byte(`{"ok":true}`), nil
}

func init() { plugin.Register(&myPlugin{}) }

func main() {}
```

Build it as a WASM reactor (it must be `package main` and contain an empty `func main(){}`):

```bash
GOOS=wasip1 GOARCH=wasm CGO_ENABLED=0 \
  go build -buildmode=c-shared -o plugin.wasm .
```

> `-buildmode=c-shared` exports `_initialize`, which the host uses to initialize the Go runtime before calling
> the ABI functions. A plain `go build` exports `_start` and exits immediately, so it cannot be loaded as a plugin.

## ABI

A plugin must export:

| Export | Description |
|------|------|
| `alloc(size) -> ptr` | Allocate guest memory |
| `dealloc(ptr, size)` | Free guest memory |
| `describe() -> i64` | Returns `(ptr<<32\|len)` of the self-description JSON |
| `configure(ptr, len) -> i64` | Injects configuration, returns a result envelope |
| `invoke(opPtr, opLen, inPtr, inLen) -> i64` | Executes an operation, returns a result envelope |

The host provides the plugin with an import module named `axmipic`:

| Import | Description |
|------|------|
| `http_request(ptr, len) -> i64` | The only network egress; enforces the host allowlist, method, size, and timeout |
| `log(level, ptr, len)` | Structured logging |
| `now_millis() -> i64` | Current Unix timestamp in milliseconds |

Results are always encoded in the envelope `{"result":<base64>, "error":"..."}`, with `ptr/len` packed
into an `i64` as `(ptr<<32|len)`. The SDK wraps these details, so plugin authors never have to handle them manually.

## Configuration and loading

```yaml
plugins:
  enabled: true
  dir: "./plugins"
  http_timeout_sec: 10
  max_http_body_kb: 1024
  trusted_keys: []          # Ed25519 trusted public keys (base64); once set, installation must pass signature verification
  require_signature: false  # When true, anything without a valid signature is rejected
  index_url: ""             # Plugin marketplace index (JSON) URL
  max_archive_mb: 64        # Maximum size after archive extraction
  memory_limit_mb: 64       # Linear memory limit per WASM plugin (MiB); 0 = built-in default of 64
  idle_unload_minutes: 0    # Idle auto-unload duration (minutes); 0 = disabled
  startup: "lazy"           # lazy (default) only registers without instantiating; eager instantiates at startup
  preload: []               # Plugins that are always warmed up (e.g. the active SMS channel)
```

At startup the host first registers the built-in plugins (always enabled + loaded), then scans `plugins.dir` to load
external plugins, validating each manifest and checksum one by one. External plugins use **lazy** mode by default: they
are only registered, not instantiated, and are actually loaded on first use; a failure in a single plugin does not prevent
other plugins or the service from starting.

## Plugin lifecycle and memory management

The host separates a plugin's "**logical enablement**" from its "**physical loading**", which yields three states
(exposed as the `state` field):

| `state` | Logically enabled | Physically loaded | Meaning |
|---------|:---:|:---:|------|
| `disabled` | ❌ | ❌ | Paused (disabled in the admin console); not loaded, occupies no memory |
| `standby` | ✅ | ❌ | Enabled but **not yet activated**: not instantiated; loaded on first call |
| `active` | ✅ | ✅ | Running: instantiated and holding memory |

Once registered, built-in plugins are always `enabled + loaded` and do not participate in pausing or idle unloading;
the enable/pause and on-demand activation rules below apply to external plugins.

- Persistence stores only the **intent**: `plugin.state.<name>` in `settings` ∈ `enabled` / `disabled`.
- `active` is purely a runtime state; it returns to `standby` after a process restart and is never persisted.
- Therefore "enabled" ≠ "occupies memory": **a plugin is active only when it is actually used (or preloaded)**.

Full lifecycle:

1. **Startup (lazy, default)**: while scanning the directory only `plugin.yaml` is read (nothing is instantiated); all plugins
   are registered as `standby`/`disabled`, and no wazero runtime is created at startup. If `startup: "eager"` is configured,
   all `enabled` plugins are instantiated at startup; in either mode, plugins in the `preload`
   list are activated immediately.
2. **On-demand activation**: on `Invoke`, `Configure`, construction of a notification channel (`Sender`), `Test`, or
   `PUT .../reload`, a plugin that is in `standby` is instantiated in place and becomes `active`. The first call incurs a
   one-time cold-start cost (WASM decoding + Go runtime initialization).
3. **Enable/pause**: `PUT .../enabled {"enabled":true}` only puts the plugin into `standby` (**does not load it**);
   `{"enabled":false}` puts it into `disabled` and, if it is loaded, immediately calls `Close` to release memory. Files and configuration
   are kept, and the state is persisted, so it survives restarts.
4. **Idle auto-unload**: when `plugins.idle_unload_minutes > 0`, disk plugins that are `active` but have not been
   called for longer than that period are unloaded (back to `standby`, **remaining enabled**); the next call transparently
   reloads them and replays the saved configuration. Built-in plugins are resident and are never idle-unloaded.
5. **Descriptor cache**: after unloading/pausing, a plugin's `descriptor` (fields, title) is cached in `settings`
   under `plugin.desc.<name>`, so admin forms and the channel list can still display it correctly, and fields are not lost
   just because the plugin is not loaded.
6. **Shutdown**: all loaded instances are closed when the process exits.

Concurrency safety: `Close` and in-flight `Invoke`/`Configure` calls are serialized with a mutex; the first load of the same
plugin is also double-checked under the lock, so "pausing while a call is in flight" or concurrent first calls never
instantiate the plugin twice or close the wazero runtime concurrently. If activation fails, the plugin stays in `standby`
and records `last_error`; the enable intent is never changed by mistake.

Memory and state visibility:

- `GET /api/v1/admin/plugins/installed` returns each plugin's `state` (disabled/standby/active),
  `builtin`, `sandboxed`, `enabled`, `loaded`, `memory_bytes` (linear memory in bytes for loaded WASM plugins;
  0 for built-in plugins), `last_used` (Unix seconds; 0 means it has not been called since loading), and `last_error`.
- `GET /api/v1/admin/plugins?category=` returns only **enabled** (`standby`/`active`) plugins;
  paused plugins do not appear in the channel picker, but their descriptor and configuration can still be viewed/edited via `/installed`.
- Together with the admin console's "Process Info" endpoint and the optional pprof endpoint, overall memory usage can be observed.

> Practical advice: with the default `lazy` mode, plugins that are never called consume no memory at all. If you only use one SMS
> channel, **pausing** the others prevents them from being loaded at all; adding the active channel to `preload` or setting `idle_unload_minutes`
> (e.g. `30`–`60`) lets you trade off between "instant response" and "memory savings" as needed.

## Admin API

| Method | Path | Description |
|------|------|------|
| GET | `/api/v1/admin/plugins?category=` | List self-descriptions of enabled plugins (optionally by category) |
| GET | `/api/v1/admin/plugins/installed` | List all installed plugins and their **state**: `state`(disabled/standby/active), `enabled`, `loaded`, `memory_bytes`, `last_used`, `last_error`, `configured` |
| GET | `/api/v1/admin/plugins/active?category=` | Read the plugin currently used for a notification category |
| PUT | `/api/v1/admin/plugins/active` | Set the current channel: `{"category":"notify.sms","name":"builtin-sms-http"}` |
| GET | `/api/v1/admin/plugins/registry` | Fetch the plugin marketplace index |
| POST | `/api/v1/admin/plugins/install` | Install a plugin (JSON by URL/index name, or a raw archive body) |
| GET | `/api/v1/admin/plugins/{name}` | Read the plugin configuration; secret fields report only whether they are set |
| PUT | `/api/v1/admin/plugins/{name}/config` | Save the configuration and apply it immediately (secrets are stored encrypted) |
| PUT | `/api/v1/admin/plugins/{name}/enabled` | Enable/pause the plugin (`{"enabled":bool}`, state persisted) |
| POST | `/api/v1/admin/plugins/{name}/test` | Send a test notification with the current configuration |
| POST | `/api/v1/admin/plugins/{name}/reload` | Reload the plugin (keeping its enabled state) |
| DELETE | `/api/v1/admin/plugins/{name}` | Uninstall and delete an external plugin (built-in plugins cannot be deleted) |

The "Plugin Marketplace" in the admin console is the **only entry point** for plugin configuration: it renders a dynamic form from the plugin's `fields`, showing regular fields directly
and rendering `secret` fields as password inputs that are never echoed back; saving applies the configuration immediately.
Fields can declare display conditions with `visible_when: {key: provider, values: [official]}`, and the console uses them to
dynamically show/hide the related fields.
Built-in plugins are configured here too, but their configuration is editable only — they cannot be uninstalled, paused, or reloaded.

## Notification channels (built-in + external)

SMS and email are both sent through plugins. The following built-in plugins are registered at startup:

| Plugin name | Category | Description | Key fields |
|--------|------|------|----------|
| `builtin-sms-log` | `notify.sms` | Log SMS, for debugging | none |
| `builtin-sms-http` | `notify.sms` | Generic HTTP SMS gateway | `provider`, `endpoint`, `method` |
| `builtin-email-log` | `notify.email` | Log email, for debugging | none |
| `builtin-email-smtp` | `notify.email` | SMTP email (STARTTLS / implicit TLS) | `host`, `port`, `username`, `password` (encrypted), `from`, `use_tls` |

External SMS plugins (such as `smsbao`) share the same install, configure, test, and invoke flow as built-in plugins. Each
notification category records the **current channel** in `plugin.active.<category>`:

- `PUT /api/v1/admin/plugins/active` sets the current channel and takes effect immediately after saving;
- if it is not set explicitly, the system falls back to `builtin-sms-log` / `builtin-email-log`;
- if the current channel is paused or uninstalled, the send path automatically falls back to the built-in log channel and logs a warning;
- the host configures the instance with the saved plugin configuration via `Configure`, and bridges `notify.Sender` to the plugin's
  `invoke("send", ...)`, with a neutral message payload (`to`/`subject`/`body`/`template`/
  `params`/`sign_name`) that the plugin maps to the specific provider.

> **Migrating from older versions**: on first startup, `settings.smtp` and the legacy `settings.sms` are automatically migrated into
> plugin configuration for `builtin-email-smtp` / `builtin-sms-http`, and the current channel is set; the migration runs only
> once (`plugin.migrated.notify_v1`). The old notification settings page has been removed; use the "Plugin Marketplace" instead.

## X link import (built-in resolver plugin)

`builtin-x-resolver` belongs to the `import.x` category and is a built-in plugin shipped with the host. It is responsible only for
resolving X/Twitter status links into the author and image URLs; it **never accesses the database or storage directly**.
Image download, upload, deduplication, quota, and gallery moderation are handled by the host's X import service.

```yaml
# Built-in plugins do not need plugin.yaml; the equivalent description is as follows
name: builtin-x-resolver
category: import.x
runtime: builtin
abi: 1
```

Configuration fields (Plugin Marketplace → Configuration):

| Field | Description |
|------|------|
| `provider` | `fxtwitter` (free, default) or `official` (official X API v2) |
| `bearer_token` | Bearer token for the official API, stored encrypted |
| `timeout_sec` | Resolution request timeout, 15 seconds by default |
| `image_variant` | `orig` / `large` / `medium` |
| `proxy_url` | Optional proxy URL; supports `http://`, `https://`, `socks5://`, `socks5h://`; when empty, the system environment proxy is used |
| `proxy_username` | Proxy authentication username, optional |
| `proxy_password` | Proxy authentication password, stored encrypted, optional |

Input and output of the `resolve` plugin operation:

```json
// Input
{ "url": "https://x.com/user/status/123" }

// Output
{
  "platform": "x",
  "post_id": "123",
  "source_url": "https://x.com/user/status/123",
  "author": { "name": "Display Name", "handle": "user" },
  "media": [
    { "id": "456", "type": "photo", "url": "https://pbs.twimg.com/...", "width": 928, "height": 1232 }
  ]
}
```

The host only accepts image URLs from `https://pbs.twimg.com` / `*.twimg.com`, and only imports the `photo`
type; video and GIF are not supported yet.

`fxtwitter` is a third-party public endpoint whose availability is up to its maintainers; deployments with high stability
requirements should configure a Bearer token for the official X API v2.

## Online installation and signing

Plugin archives are zips (with `plugin.yaml` and the entry file at the root). The publishing tool `pluginpack`
is located in `sdk/plugin-go/cmd/pluginpack`:

```bash
cd sdk/plugin-go
# Package and sign (the private key is a base64 32-byte seed or a 64-byte Ed25519 private key)
go run ./cmd/pluginpack -dir ../../plugins/smsbao -out smsbao-0.1.0.zip -key "$PRIVATE_KEY"
# Prints sha256=... and signature=...
```

Installation methods:

- **Local upload**: `POST /api/v1/admin/plugins/install` with the raw zip bytes as the body, and optional headers
  `X-Plugin-Sha256` and `X-Plugin-Signature`.
- **By URL / index name**: JSON body `{"url":"...","sha256":"...","signature":"..."}`,
  or `{"name":"smsbao"}` resolved from the marketplace via `index_url`.

Enable/pause: `PUT /api/v1/admin/plugins/{name}/enabled` with body `{"enabled":true|false}`.
In `lazy` mode, `{"enabled":true}` only puts the plugin into `standby` (**it is not loaded immediately**) and it is activated
on first use; `{"enabled":false}` unregisters it and (for disk plugins) closes its instance to **release memory immediately**,
while keeping files and configuration. State is persisted and survives restarts. A paused plugin cannot be set as the current notification
channel, but its descriptor and configuration remain viewable/editable (from the `plugin.desc.<name>` cache). For idle
auto-unload of enabled plugins, see "Plugin lifecycle and memory management" above.

The host validates the size, sha256, and Ed25519 signature (enforced when `trusted_keys` or
`require_signature` is configured), then safely extracts the archive (rejecting path traversal, symlinks, and oversized content),
validates the manifest, and hot-loads the plugin.

JSON structure of the marketplace index at `index_url`:

```json
{
  "plugins": [
    {
      "name": "smsbao",
      "version": "0.1.0",
      "category": "notify.sms",
      "runtime": "wasm",
      "description": "SMSBao",
      "url": "https://example.com/smsbao-0.1.0.zip",
      "sha256": "…",
      "signature": "…"
    }
  ]
}
```

## Security model

- WASM sandbox: limits linear memory (64 MiB by default, configurable via `plugins.memory_limit_mb`) and supports interruption via per-context timeouts.
- Capability allowlist: hosts outside `capabilities.http.hosts` are always rejected; if undeclared, all network access is disabled.
- External plugins may only use the WASM runtime and always run in the wazero sandbox.
- Built-in plugins are compiled and shipped with the host, trusted by default, and run in-process; external plugins cannot forge a built-in identity
  (both the `builtin-` name prefix and `runtime: builtin` are rejected).
- Secret fields are encrypted by the host with AES-256-GCM before being written to the `settings` table; the AAD binds the plugin name and field name,
  and API responses never contain plaintext.
- The optional `checksum` guards against tampering with the entry file.

## Build and test

```bash
make plugin-example   # Build all example WASM plugins into plugins/
make test-plugin      # Run plugin framework tests (including WASM end-to-end)
```

## Roadmap

1. ~~**M2**: wire plugins into `notify`, admin dynamic forms and credential encryption, test sending.~~ ✅
2. ~~**M3**: Alibaba Cloud / Tencent Cloud SMS plugins.~~ ✅ (see `sdk/plugin-go/examples/`)
3. ~~**M5**: online installation, Ed25519 signature verification, and plugin marketplace index.~~ ✅
4. ~~**Plugin lifecycle**: on-demand activation (lazy by default, loaded on first use), preload warm-up, idle auto-unload, and three-state visibility.~~ ✅
5. ~~**Built-in/external separation**: built-in notification plugins (SMS/email) require no sandbox; external plugins are forced into the WASM sandbox.~~ ✅

## Bundled example plugins

| Plugin | Directory | Runtime | Description |
|------|------|--------|------|
| SMSBao | `examples/smsbao` | wasm | Generic HTTP + MD5 signature |
| Alibaba Cloud SMS | `examples/aliyun-sms` | wasm | RPC + HMAC-SHA1 signature, template parameters taken from `params` |
| Tencent Cloud SMS | `examples/tencent-sms` | wasm | TC3-HMAC-SHA256 signature, JSON POST |

All cloud provider plugins perform signing inside the plugin using the Go standard library, so the host needs no provider-specific dependencies. Running
`make plugin-example` builds all examples into `plugins/`, after which they can be configured in the admin console's "Plugin Marketplace" and set as the current SMS channel.

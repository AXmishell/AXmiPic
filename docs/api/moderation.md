# Image Plaza AI Moderation

Before an uploaded image is "opened to the image plaza" (made public), a vision model can be called to perform content moderation. Moderation **applies only to the image plaza**: images that fail moderation are not made public and remain in the user's own private images.

There are two ways to configure it, and **database settings take precedence**:

1. **Online configuration in the admin console (recommended; takes effect immediately without a restart)**: the "System Settings → AI Moderation" page, or `GET/PUT /api/v1/admin/moderation`. The key is encrypted with the master key and stored in the database `settings` table; the API only reports whether it is set (`api_key_set`).
2. **Configuration file / environment variables (fallback for first startup)**: the `moderation.*` section or `AXMIPIC_MODERATION_*`. When there is no database record, it is initialized from this and written to the database.

| Setting | Description |
|---------|-------------|
| `moderation.enabled` | Whether image plaza AI moderation is enabled |
| `moderation.base_url` | Root URL of a standard OpenAI-compatible API, e.g. `https://api.openai.com/v1` |
| `moderation.api_key` | API key (Bearer); leave empty when saving online to keep the existing key |
| `moderation.model` | Vision model name, e.g. `gpt-4o-mini` |
| `moderation.timeout_sec` | Timeout for a single moderation request (seconds; default 30) |
| `moderation.prompt` | Override the default moderation prompt (by default the model must answer only `SAFE` or `UNSAFE`) |
| `moderation.max_image_mb` | Maximum size of an image submitted for moderation (MiB; default 10); larger images are treated as failed |

Behavior and fallbacks:

- When images are made public via `POST /api/v1/images/batch`, each is moderated individually (concurrency limit 4). Those that pass become public; those that fail are forced to stay private. The response includes `published` and `blocked` (the IDs of blocked images), which the frontend uses to inform the user.
- **Fail closed**: if the model returns empty content or an unrecognizable result, or the API call fails (network/timeout/non-200), moderation is always treated as failed and the image stays private.
- Images that are already public are not moderated again; setting an image to private does not trigger moderation.
- This moderation is a gate "before going public" and does not change the upload itself—images are always stored privately first.
- After saving in the admin console, the running moderator is rebuilt immediately; no restart is needed.

Example:

```yaml
moderation:
  enabled: true
  base_url: "https://api.openai.com/v1"
  api_key: "sk-..."
  model: "gpt-4o-mini"
  timeout_sec: 30
  max_image_mb: 10
```

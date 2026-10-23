# Image Security, Cloud Processing, SMS, and Email

These capabilities are all implemented through pluggable interfaces; when no provider is configured, they fall back to safe default implementations.

| Setting | Description |
|---------|-------------|
| `security.scanner` | Upload content scanner: `none` (allow) or `builtin` (allowlist + dangerous magic-number detection, rejecting executable content disguised as images) |
| `security.cloud_processor` | Cloud processor: `local` (uses the built-in imaging pipeline); implement `security.CloudProcessor` to integrate an external cloud service |
| `sms.*` | Generic HTTP SMS gateway: `enabled`, `provider`, `endpoint`, `method`; when not enabled, messages are written to the log |
| `email.*` | SMTP email: `host`, `port`, `username`, `password`, `from`, `use_tls`; when not enabled, messages are written to the log |

Admin endpoints:

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/security` | Currently enabled scanner |
| GET | `/admin/runtime` | Instance runtime environment information (site address, database/storage/processor, quotas and rate limits, etc.; no secrets) |
| GET | `/admin/process` | Live process runtime metrics (goroutines, heap memory, heap object count, GC, uptime, etc.) |
| GET | `/admin/notify/channels` | Configured SMS and email channels |
| POST | `/admin/notify/test` | Send a test notification (`{"channel":"sms","to":"…","body":"…"}`) |
| GET | `/admin/notify/logs` | Notification send logs (`?channel=email\|sms&page=&page_size=`; newest first) |
| GET | `/admin/notify/smtp` | Read SMTP email channel settings (the password only reports whether it is set) |
| PUT | `/admin/notify/smtp` | Save SMTP settings with immediate effect (an empty `password` means keep the existing password) |
| GET/PUT | `/admin/auth` | Read/switch permission toggles online (`allow_registration`, `require_auth`, `allow_guest_upload`); saves take effect immediately without a restart |
| GET/PUT | `/admin/moderation` | Read/save image plaza AI moderation settings online (secrets masked); saves take effect immediately without a restart |
| GET | `/admin/payment` | Read payment settings (secrets only report whether they are set) |
| PUT | `/admin/payment` | Save payment settings with immediate effect (an empty secret means keep the existing value) |

The scanner runs on both entry points: `multipart` uploads and presigned direct-upload confirmation. When dangerous content is detected it returns HTTP 422 and refuses to store the object. The fallback implementations of notification channels write messages to the server log, which helps with development and debugging. "System Settings → Notification Settings" in the admin console provides SMTP configuration, channel inspection, and test sending; the SMTP password is encrypted with the master key and stored in the `settings` table in the database, and after saving, the email channel switches immediately without a restart. Every send (including verification codes, notifications, and admin test messages) is recorded in the `notify_logs` table and can be viewed by email/SMS on the "Email Logs" page in the admin sidebar; logs are retained for 30 days and then cleaned up automatically.

# Role Groups and Policies

A role group binds a set of typed policies together and assigns them to regular users, controlling resources and features by role. Within a role group, each type can be bound to at most one policy; binding the same type again replaces the old policy. Built-in policy types:

| Type | Purpose | Settings fields (JSON) |
|------|---------|------------------------|
| `quota` | Storage quota | `quota_mb` (0 means unlimited) |
| `upload` | Single-file size and media types | `max_size_mb`, `allowed_mime_types` |
| `rate` | Upload and image-read rate limits | `upload_per_minute`, `upload_burst`, `image_per_minute`, `image_burst` |
| `processing` | On-the-fly image processing capabilities | `enabled`, `max_width`, `max_height`, `default_quality`, `allowed_formats` |
| `feature` | Set of feature toggles | `features` (such as `plaza`, `albums`, `api_tokens`, `batch_upload`, `paste_upload`, `drag_upload`, `embed_code`, `share`, `share_password`) |

Policy fields are optional overrides: any field that is absent inherits the default value from the configuration file. On first startup, a "default role group" and one policy of each type are seeded from the `auth`/`upload`/`processing`/`limits` configuration; existing users are automatically assigned to the default group.

Upload and image-read rate limits are resolved and enforced on every request according to the `rate` policy of the caller's role group (guests and anonymous requests use the Guest group, falling back to the default group); changes made by administrators take effect within about 30 seconds. The `limits.*` configuration only seeds policy defaults.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/auth/policies` | The effective policy set for the current account |
| GET/POST | `/admin/role-groups` | Role group list / create |
| GET/PUT/DELETE | `/admin/role-groups/{id}` | Role group details / update / delete |
| POST | `/admin/role-groups/{id}/policies` | Bind a policy (body is `{"policy_id":"…"}`; replaces the same type) |
| DELETE | `/admin/role-groups/{id}/policies/{policyID}` | Unbind |
| GET/POST | `/admin/policies` | Policy list (optionally with `?type=`) / create |
| GET/PUT/DELETE | `/admin/policies/{id}` | Policy details / update / delete |

Via `PATCH /admin/customers/{id}`, the request body may include `"role_group_id"` (an empty string means fall back to the default group) to assign a role group to a regular user; on assignment, the user's storage quota is synchronized according to the role group's `quota` policy. After a quota policy is modified, the quotas of members of all role groups that reference it are re-synchronized as well.

# Admin Endpoints (Administrator Required)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/stats` | Instance statistics |
| GET | `/admin/customers` | Regular user list |
| GET/POST | `/admin/admins` | Administrator list / create administrator |
| PATCH/DELETE | `/admin/customers/{id}` | Enable/disable or delete a regular user |
| PATCH/DELETE | `/admin/admins/{id}` | Enable/disable or delete an administrator |
| GET/POST | `/admin/storage` | Storage backend list / create |
| GET/PUT/DELETE | `/admin/storage/{id}` | Storage backend details / update / delete |
| POST | `/admin/storage/{id}/activate` | Switch the default storage backend (hot switch) |
| GET/POST | `/admin/storage/pools` | Storage pool list / create |
| GET/PUT/DELETE | `/admin/storage/pools/{id}` | Storage pool details / update / delete |
| POST | `/admin/storage/pools/{id}/activate` | Make a storage pool the current write target |

Storage backend secrets (S3/Qiniu Secret) are stored encrypted in the database; the API only returns whether they are set and never returns plaintext.

A storage pool only determines **which backend new objects are written to**; existing images are still read from their original backend according to `images.storage_id`. `policy` supports `weighted_rr` (weighted round-robin, the default) and `priority` (lower values take precedence). Before enabling a storage pool, the system checks whether all historical images have a recorded `storage_id`; if some do not, it refuses and requires backfilling first, avoiding misrouted reads of old images.

## Customer List Pagination and Filtering

Without any query parameters, `GET /admin/customers` keeps its legacy behavior and returns a full array of users (the Go/TypeScript SDKs depend on this shape). When any of the following parameters is provided, it returns a paginated object:

| Parameter | Description |
|-----------|-------------|
| `page` | Page number, starting at 1 (default 1) |
| `page_size` | Number of items per page (default 20, maximum 200) |
| `keyword` | Fuzzy match by username or email |
| `status` | `enabled` / `disabled`; empty means all |
| `role_group_id` | Filter by role group |
| `sort` | `newest` (default) / `oldest` / `username` / `usage` |

Paginated response:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [ /* User */ ],
    "total": 100,
    "page": 1,
    "page_size": 20
  }
}
```

# API

All endpoints are prefixed with `/api/v1`, and requests use the `Authorization: Bearer <token>` header (a session JWT or an API token). Responses share a uniform shape:

```json
{ "code": 0, "message": "ok", "data": {} }
```

A `code` of 0 indicates success; on error, `code` is the corresponding HTTP status code and `data` is `null`.

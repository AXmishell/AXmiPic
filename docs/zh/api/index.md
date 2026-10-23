# API

所有接口以 `/api/v1` 为前缀，请求头使用 `Authorization: Bearer <令牌>`（会话 JWT 或 API 令牌）。响应统一为：

```json
{ "code": 0, "message": "ok", "data": {} }
```

`code` 为 0 表示成功；错误时 `code` 为对应 HTTP 状态码，`data` 为 `null`。

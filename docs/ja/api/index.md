# API

すべてのエンドポイントには `/api/v1` が付与され、リクエストでは `Authorization: Bearer <token>` ヘッダー(セッション JWT または API トークン)を使用します。レスポンスは共通の形式になります:

```json
{ "code": 0, "message": "ok", "data": {} }
```

`code` が 0 の場合は成功を示します。エラー時は `code` に対応する HTTP ステータスコードが入り、`data` は `null` になります。

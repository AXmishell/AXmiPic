# アップロード

| メソッド | パス | 説明 |
|--------|------|-------------|
| POST | `/upload` | `multipart` フォームによるアップロード。フィールド名は `file` です |
| POST | `/upload/presign` | オブジェクトストレージへの署名付き直接アップロードを要求します |
| POST | `/upload/confirm` | 直接アップロードが完了したことを確認し、記録します |

`/upload` のレスポンス例:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "id": "9fa5bcf9-…",
    "key": "2026/10/05/6f1c3b8e-…-a2.png",
    "url": "http://localhost:8080/i/2026/10/05/6f1c3b8e-…-a2.png",
    "original_name": "new-image.png",
    "filename": "6f1c3b8e-…-a2.png",
    "hash": "163053ece784c464ae8fe55e2532d2804a3762eeac80e5b6cbef4f7e2c16bce4",
    "permission": "private",
    "size": 73,
    "mime_type": "image/png",
    "width": 5,
    "height": 7,
    "created_at": "2026-10-04T18:57:00+08:00"
  }
}
```

curl によるアップロード:

```bash
curl -X POST http://localhost:8080/api/v1/upload \
  -H "Authorization: Bearer <API_TOKEN>" \
  -F "file=@./photo.png"
```

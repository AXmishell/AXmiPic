# Upload

| Method | Path | Description |
|--------|------|-------------|
| POST | `/upload` | `multipart` form upload, with the field named `file` |
| POST | `/upload/presign` | Request a presigned direct upload to object storage |
| POST | `/upload/confirm` | Confirm that a direct upload has completed and record it |

Sample `/upload` response:

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

Uploading with curl:

```bash
curl -X POST http://localhost:8080/api/v1/upload \
  -H "Authorization: Bearer <API_TOKEN>" \
  -F "file=@./photo.png"
```

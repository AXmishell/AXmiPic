# 上传

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/upload` | `multipart` 表单上传，字段名 `file` |
| POST | `/upload/presign` | 申请对象存储预签名直传 |
| POST | `/upload/confirm` | 确认直传完成并入库 |

`/upload` 返回示例：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "id": "9fa5bcf9-…",
    "key": "2026/10/05/6f1c3b8e-…-a2.png",
    "url": "http://localhost:8080/i/2026/10/05/6f1c3b8e-…-a2.png",
    "original_name": "新图.png",
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

使用 curl 上传：

```bash
curl -X POST http://localhost:8080/api/v1/upload \
  -H "Authorization: Bearer <API_TOKEN>" \
  -F "file=@./photo.png"
```

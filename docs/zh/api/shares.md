# 分享

分享链接可指向单张图片或整个相册，支持可选的访问密码、有效期（小时）与最大访问次数。

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/shares` | 创建分享（体见下） |
| GET | `/shares` | 当前账号创建的分享 |
| DELETE | `/shares/{id}` | 撤销分享 |
| GET | `/shares/{token}` | 分享公开信息（无需登录，用于判断是否需要密码） |
| POST | `/shares/{token}/access` | 校验密码并返回内容（无需登录） |

创建请求体示例：

```json
{
  "target_type": "image",
  "target_id": "<image_id 或 album_id>",
  "password": "可选",
  "expires_in_hours": 24,
  "max_views": 100
}
```

`target_type` 取值为 `image` 或 `album`。`password` 为空表示无需密码；`expires_in_hours` 与 `max_views` 为 0 或省略表示不限制。访问受密码保护的分享时，`POST /shares/{token}/access` 需在请求体携带 `{"password":"…"}`。加密分享通过角色组的 `share` 与 `share_password` 功能开关控制。分享的公开页面为 `/s/{token}`。

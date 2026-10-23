# Shares

A share link can point to a single image or an entire album, with an optional access password, expiry (in hours), and maximum view count.

| Method | Path | Description |
|--------|------|-------------|
| POST | `/shares` | Create a share (body below) |
| GET | `/shares` | Shares created by the current account |
| DELETE | `/shares/{id}` | Revoke a share |
| GET | `/shares/{token}` | Public share information (no login required; used to determine whether a password is needed) |
| POST | `/shares/{token}/access` | Verify the password and return the content (no login required) |

Sample create request body:

```json
{
  "target_type": "image",
  "target_id": "<image_id or album_id>",
  "password": "optional",
  "expires_in_hours": 24,
  "max_views": 100
}
```

`target_type` is `image` or `album`. An empty `password` means no password is required; `expires_in_hours` and `max_views` being 0 or omitted means no limit. When accessing a password-protected share, `POST /shares/{token}/access` must include `{"password":"…"}` in the request body. Password-protected shares are controlled by the role group's `share` and `share_password` feature toggles. The public page for a share is `/s/{token}`.

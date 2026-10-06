# 相册与图片广场

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/albums` | 当前账号的相册列表（含图片数量、所有者与可见性） |
| POST | `/albums` | 新建相册（可带 `permission`: `private`/`public`） |
| GET | `/albums/{id}` | 相册详情（公开相册无需登录） |
| PATCH | `/albums/{id}` | 修改相册名称/简介/可见性（仅所有者） |
| DELETE | `/albums/{id}` | 删除相册（图片保留，仅移出相册） |
| GET | `/albums/{id}/images` | 相册中的图片（公开相册无需登录） |
| GET | `/plaza` | 公开图片广场（跨用户，返回 `permission=public` 的图片，支持 `user_id` 按上传者过滤） |
| GET | `/plaza/albums` | 公开相册列表（支持 `user_id` 过滤） |
| GET | `/users/{id}` | 用户公开资料：用户名、公开图片数与公开相册 |

图片的 `permission` 取值为 `private`（默认，仅本人可见）或 `public`（可出现在图片广场，并在公开列表中附带 `owner_username`）。相册的 `permission` 同样为 `private`（默认）或 `public`：公开相册的详情与图片对未登录访客开放，并出现在 `/plaza/albums`。广场、公开相册与用户资料均为只读接口，无需登录即可访问。

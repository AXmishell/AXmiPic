# 站点内容（公告 / 举报 / 独立页面）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/announcements` | 已发布公告（无需登录） |
| GET | `/pages/{slug}` | 已发布独立页面（无需登录） |
| POST | `/reports` | 提交举报（`{"image_id":"…","reason":"…","detail":"…"}`，`image_id` 可选） |
| GET/POST | `/admin/announcements` | 公告列表 / 新建 |
| PUT/DELETE | `/admin/announcements/{id}` | 修改 / 删除公告 |
| GET | `/admin/reports` | 举报列表（可带 `?status=pending\|resolved\|rejected`） |
| PATCH | `/admin/reports/{id}` | 处理举报（`{"status":"resolved","note":"…"}`） |
| GET/POST | `/admin/pages` | 独立页面列表 / 新建 |
| PUT/DELETE | `/admin/pages/{id}` | 修改 / 删除页面 |

公告的 `level` 取值为 `info`/`success`/`warning`/`danger`，可置顶（`pinned`）与设为草稿（`published=false`）。独立页面的 `slug` 仅允许小写字母、数字与连字符，公开地址为 `/p/{slug}`，前端以轻量 Markdown 渲染内容。

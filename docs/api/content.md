# Site Content (Announcements / Reports / Standalone Pages)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/announcements` | Published announcements (no login required) |
| GET | `/pages/{slug}` | Published standalone pages (no login required) |
| POST | `/reports` | Submit a report (`{"image_id":"…","reason":"…","detail":"…"}`; `image_id` is optional) |
| GET/POST | `/admin/announcements` | Announcement list / create |
| PUT/DELETE | `/admin/announcements/{id}` | Update / delete an announcement |
| GET | `/admin/reports` | Report list (optionally with `?status=pending\|resolved\|rejected`) |
| PATCH | `/admin/reports/{id}` | Handle a report (`{"status":"resolved","note":"…"}`) |
| GET/POST | `/admin/pages` | Standalone page list / create |
| PUT/DELETE | `/admin/pages/{id}` | Update / delete a page |

An announcement's `level` is `info`/`success`/`warning`/`danger`, and it can be pinned (`pinned`) or saved as a draft (`published=false`). A standalone page's `slug` allows only lowercase letters, digits, and hyphens; its public address is `/p/{slug}`, and the frontend renders the content as lightweight Markdown.

`GET /admin/reports` supports `page`, `page_size` (default 20, maximum 200), and `status`. When pagination parameters are passed it returns `{items,total,page,page_size}`; otherwise it keeps the legacy array response.

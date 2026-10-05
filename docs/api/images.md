# 图片与令牌

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/images` | 图片列表（分页与过滤，见下） |
| GET | `/images/{id}` | 单张图片信息 |
| PATCH | `/images/{id}` | 重命名图片（仅修改展示用原文件名） |
| DELETE | `/images/{id}` | 删除图片 |
| POST | `/images/batch` | 批量设置可见性 / 所属相册 |
| POST | `/tokens` | 创建 API 令牌（明文仅返回一次） |
| GET | `/tokens` | 令牌列表 |
| DELETE | `/tokens/{id}` | 吊销令牌 |

`GET /images` 支持查询参数 `page`、`page_size`、`order`（`newest`/`earliest`/`largest`/`smallest`）、`keyword`（按文件名搜索）、`permission`（`public`/`private`）与 `album_id`（按相册过滤）。

`POST /images/batch` 请求体示例，`permission` 与 `album_id`/`clear_album` 至少提供其一：

```json
{ "ids": ["<id1>", "<id2>"], "permission": "public", "album_id": "<album_id>" }
```

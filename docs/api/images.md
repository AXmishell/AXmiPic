# 图片与令牌

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/images` | 图片列表（分页与过滤，见下） |
| GET | `/images/{id}` | 单张图片信息 |
| PATCH | `/images/{id}` | 重命名图片；管理员可同时修改作者与来源 |
| DELETE | `/images/{id}` | 删除图片 |
| POST | `/images/batch` | 批量设置可见性 / 所属相册 |
| POST | `/import/x/preview` | 解析 X 帖子链接，返回作者与候选图片 |
| POST | `/import/x` | 下载并导入 X 帖子中的图片 |
| POST | `/tokens` | 创建 API 令牌（明文仅返回一次） |
| GET | `/tokens` | 令牌列表 |
| DELETE | `/tokens/{id}` | 吊销令牌 |

`GET /images` 支持查询参数 `page`、`page_size`、`order`（`newest`/`earliest`/`largest`/`smallest`）、`keyword`（按文件名搜索）、`permission`（`public`/`private`）与 `album_id`（按相册过滤）。

`POST /images/batch` 请求体示例，`permission` 与 `album_id`/`clear_album` 至少提供其一：

```json
{ "ids": ["<id1>", "<id2>"], "permission": "public", "album_id": "<album_id>" }
```

## 作者与来源

图片列表与详情会返回以下可选字段（均可为空）：

```json
{
  "author_name": "げっか ☾ᶻ 𝗓 𐰁 🐾",
  "author_handle": "mooNli9ht_4tune",
  "author_platform": "x",
  "author_url": "https://x.com/mooNli9ht_4tune",
  "source_url": "https://x.com/mooNli9ht_4tune/status/2112521500301607286",
  "source_post_id": "2112521500301607286"
}
```

`author_url` 由服务端根据 `author_platform` 与 `author_handle` 派生，不落库。

`PATCH /images/{id}` 支持部分更新，省略字段表示不修改，传空字符串表示清空：

```json
{
  "name": "新名称.png",
  "author_name": "げっか ☾ᶻ 𝗓 𐰁 🐾",
  "author_handle": "mooNli9ht_4tune",
  "source_url": "https://x.com/mooNli9ht_4tune/status/2112521500301607286"
}
```

- `name`：图片所有者或管理员可改。
- `author_name` / `author_handle` / `source_url`：**仅管理员可改**。
- X 来源链接会被规范化为 `https://x.com/{user}/status/{id}`，并自动补全 `source_post_id`。

## X 导入

解析（不下载）：

```http
POST /api/v1/import/x/preview
{ "url": "https://x.com/mooNli9ht_4tune/status/2112521500301607286" }
```

导入（默认公开到广场）：

```http
POST /api/v1/import/x
{
  "url": "https://x.com/mooNli9ht_4tune/status/2112521500301607286",
  "media_ids": ["2112521494509297664"],
  "permission": "public"
}
```

- `media_ids` 为空时导入帖子内全部图片，最多 4 张。
- `permission` 为 `public`（默认）或 `private`；私有导入后可在图片管理中再公开。
- 导入会复用上传配额、安全扫描、内容哈希去重与广场审核。
- 返回 `items[].status`：`imported` / `existing` / `blocked` / `failed`。
- 解析接口由内置 `builtin-x-resolver` 插件提供；管理员可在「插件市场」配置使用
  `fxtwitter` 免费接口或 X 官方 API v2（Bearer Token）。

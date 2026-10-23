# Images and Tokens

| Method | Path | Description |
|--------|------|-------------|
| GET | `/images` | Image list (pagination and filtering; see below) |
| GET | `/images/{id}` | Information for a single image |
| PATCH | `/images/{id}` | Rename an image; administrators can also change the author and source |
| DELETE | `/images/{id}` | Delete an image |
| POST | `/images/batch` | Batch-set visibility / owning album |
| POST | `/import/x/preview` | Parse an X post link and return the author and candidate images |
| POST | `/import/x` | Download and import images from an X post |
| POST | `/tokens` | Create an API token (the plaintext is returned only once) |
| GET | `/tokens` | Token list |
| DELETE | `/tokens/{id}` | Revoke a token |

`GET /images` supports the query parameters `page`, `page_size`, `order` (`newest`/`earliest`/`largest`/`smallest`), `keyword` (search by filename), `permission` (`public`/`private`), and `album_id` (filter by album).

Sample `POST /images/batch` request body; provide at least one of `permission` and `album_id`/`clear_album`:

```json
{ "ids": ["<id1>", "<id2>"], "permission": "public", "album_id": "<album_id>" }
```

## Author and Source

Image lists and details return the following optional fields (all may be empty):

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

`author_url` is derived by the server from `author_platform` and `author_handle`; it is not persisted.

`PATCH /images/{id}` supports partial updates: omitting a field leaves it unchanged, while passing an empty string clears it:

```json
{
  "name": "new-name.png",
  "author_name": "げっか ☾ᶻ 𝗓 𐰁 🐾",
  "author_handle": "mooNli9ht_4tune",
  "source_url": "https://x.com/mooNli9ht_4tune/status/2112521500301607286"
}
```

- `name`: can be changed by the image owner or an administrator.
- `author_name` / `author_handle` / `source_url`: **administrators only**.
- X source links are normalized to `https://x.com/{user}/status/{id}`, and `source_post_id` is filled in automatically.

## X Import

Preview (no download):

```http
POST /api/v1/import/x/preview
{ "url": "https://x.com/mooNli9ht_4tune/status/2112521500301607286" }
```

Import (public in the plaza by default):

```http
POST /api/v1/import/x
{
  "url": "https://x.com/mooNli9ht_4tune/status/2112521500301607286",
  "media_ids": ["2112521494509297664"],
  "permission": "public"
}
```

- When `media_ids` is empty, all images in the post are imported, up to 4.
- `permission` is `public` (default) or `private`; after a private import, images can be made public later in image management.
- Imports reuse the upload quota, security scanning, content-hash deduplication, and plaza moderation.
- Returns `items[].status`: `imported` / `existing` / `blocked` / `failed`.
- The preview endpoint is provided by the built-in `builtin-x-resolver` plugin; administrators can configure it in the
  "Plugin Marketplace" to use the free `fxtwitter` API or the official X API v2 (Bearer Token).

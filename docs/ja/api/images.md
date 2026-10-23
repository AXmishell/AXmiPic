# 画像とトークン

| メソッド | パス | 説明 |
|--------|------|-------------|
| GET | `/images` | 画像一覧(ページネーションとフィルタリング。後述) |
| GET | `/images/{id}` | 単一画像の情報 |
| PATCH | `/images/{id}` | 画像の名前を変更します。管理者は作者とソースも変更できます |
| DELETE | `/images/{id}` | 画像を削除します |
| POST | `/images/batch` | 公開範囲 / 所属アルバムを一括設定します |
| POST | `/import/x/preview` | X の投稿リンクを解析し、作者と候補画像を返します |
| POST | `/import/x` | X の投稿から画像をダウンロードしてインポートします |
| POST | `/tokens` | API トークンを作成します(平文は一度だけ返されます) |
| GET | `/tokens` | トークン一覧 |
| DELETE | `/tokens/{id}` | トークンを失効させます |

`GET /images` はクエリパラメータ `page`、`page_size`、`order`(`newest`/`earliest`/`largest`/`smallest`)、`keyword`(ファイル名で検索)、`permission`(`public`/`private`)、`album_id`(アルバムで絞り込み)に対応しています。

`POST /images/batch` のリクエストボディ例。`permission` と `album_id`/`clear_album` のうち少なくとも 1 つを指定します:

```json
{ "ids": ["<id1>", "<id2>"], "permission": "public", "album_id": "<album_id>" }
```

## 作者とソース

画像一覧と詳細では、次のオプションフィールドが返されます(いずれも空の場合があります):

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

`author_url` はサーバーが `author_platform` と `author_handle` から導出するもので、永続化されません。

`PATCH /images/{id}` は部分更新に対応しています。フィールドを省略すると変更されず、空文字列を渡すとクリアされます:

```json
{
  "name": "new-name.png",
  "author_name": "げっか ☾ᶻ 𝗓 𐰁 🐾",
  "author_handle": "mooNli9ht_4tune",
  "source_url": "https://x.com/mooNli9ht_4tune/status/2112521500301607286"
}
```

- `name`: 画像の所有者または管理者が変更できます。
- `author_name` / `author_handle` / `source_url`: **管理者のみ**。
- X のソースリンクは `https://x.com/{user}/status/{id}` に正規化され、`source_post_id` が自動的に設定されます。

## X インポート

プレビュー(ダウンロードなし):

```http
POST /api/v1/import/x/preview
{ "url": "https://x.com/mooNli9ht_4tune/status/2112521500301607286" }
```

インポート(デフォルトでは画像広場に公開):

```http
POST /api/v1/import/x
{
  "url": "https://x.com/mooNli9ht_4tune/status/2112521500301607286",
  "media_ids": ["2112521494509297664"],
  "permission": "public"
}
```

- `media_ids` が空の場合は、投稿内のすべての画像を最大 4 枚までインポートします。
- `permission` は `public`(デフォルト)または `private` です。非公開でインポートした後でも、画像管理から後で公開に変更できます。
- インポートはアップロードクォータ、セキュリティスキャン、コンテンツハッシュによる重複排除、画像広場のモデレーションを再利用します。
- `items[].status` を返します: `imported` / `existing` / `blocked` / `failed`。
- プレビューエンドポイントは組み込みの `builtin-x-resolver` プラグインが提供します。管理者は「プラグインマーケットプレイス」で、
  無料の `fxtwitter` API または公式 X API v2(Bearer Token)を使用するように設定できます。

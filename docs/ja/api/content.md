# サイトコンテンツ(お知らせ / 通報 / 単独ページ)

| メソッド | パス | 説明 |
|--------|------|-------------|
| GET | `/announcements` | 公開中のお知らせ(ログイン不要) |
| GET | `/pages/{slug}` | 公開中の単独ページ(ログイン不要) |
| POST | `/reports` | 通報を送信します(`{"image_id":"…","reason":"…","detail":"…"}`。`image_id` は任意) |
| GET/POST | `/admin/announcements` | お知らせ一覧 / 作成 |
| PUT/DELETE | `/admin/announcements/{id}` | お知らせの更新 / 削除 |
| GET | `/admin/reports` | 通報一覧(任意で `?status=pending\|resolved\|rejected` を指定) |
| PATCH | `/admin/reports/{id}` | 通報を処理します(`{"status":"resolved","note":"…"}`) |
| GET/POST | `/admin/pages` | 単独ページ一覧 / 作成 |
| PUT/DELETE | `/admin/pages/{id}` | ページの更新 / 削除 |

お知らせの `level` は `info`/`success`/`warning`/`danger` で、ピン留め(`pinned`)や下書きとしての保存(`published=false`)が可能です。単独ページの `slug` は小文字、数字、ハイフンのみ使用でき、公開アドレスは `/p/{slug}` です。フロントエンドはコンテンツを軽量な Markdown としてレンダリングします。

`GET /admin/reports` は `page`、`page_size`(デフォルト 20、最大 200)、`status` に対応しています。ページネーションパラメータを渡した場合は `{items,total,page,page_size}` を返し、それ以外の場合は従来の配列レスポンスを維持します。

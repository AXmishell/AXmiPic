# 共有

共有リンクは単一の画像またはアルバム全体を指すことができ、任意でアクセスパスワード、有効期限(時間単位)、最大閲覧回数を設定できます。

| メソッド | パス | 説明 |
|--------|------|-------------|
| POST | `/shares` | 共有を作成します(ボディは後述) |
| GET | `/shares` | 現在のアカウントが作成した共有 |
| DELETE | `/shares/{id}` | 共有を失効させます |
| GET | `/shares/{token}` | 公開共有情報(ログイン不要。パスワードが必要かどうかの判定に使用) |
| POST | `/shares/{token}/access` | パスワードを検証してコンテンツを返します(ログイン不要) |

作成リクエストのボディ例:

```json
{
  "target_type": "image",
  "target_id": "<image_id or album_id>",
  "password": "optional",
  "expires_in_hours": 24,
  "max_views": 100
}
```

`target_type` は `image` または `album` です。`password` が空の場合はパスワード不要を意味し、`expires_in_hours` と `max_views` が 0 または省略の場合は制限なしを意味します。パスワードで保護された共有にアクセスする場合、`POST /shares/{token}/access` のリクエストボディに `{"password":"…"}` を含める必要があります。パスワード保護付きの共有は、ロールグループの `share` と `share_password` の機能トグルで制御されます。共有の公開ページは `/s/{token}` です。

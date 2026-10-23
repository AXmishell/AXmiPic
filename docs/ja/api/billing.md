# プラン、クーポン、注文、チケット

| メソッド | パス | 説明 |
|--------|------|-------------|
| GET | `/plans` | 有効なプラン(ログイン不要) |
| GET | `/admin/plans` | すべてのプラン |
| POST/PUT/DELETE | `/admin/plans[/{id}]` | プランの作成 / 更新 / 削除 |
| GET | `/admin/coupons` | クーポン一覧 |
| POST/PUT/DELETE | `/admin/coupons[/{id}]` | クーポンの作成 / 更新 / 削除 |
| POST | `/coupons/validate` | クーポンを検証します(`{"code":"…","amount_cents":1000}`) |
| GET/POST | `/orders` | 自分の注文 / 注文の作成(管理者はすべての注文を表示) |
| POST | `/orders/{id}/pay` | 手動/モック注文の支払いを完了します |
| GET/POST | `/tickets` | 自分のチケット / チケットの作成(管理者はすべて表示) |
| GET | `/tickets/{id}` | チケットの詳細(メッセージを含む) |
| POST | `/tickets/{id}/reply` | チケットに返信します |
| PATCH | `/admin/tickets/{id}` | チケットのステータスを更新します |

プランのフィールド: `price_cents`(セント単位の価格)、`duration_days`(永続の場合は 0)、`quota_mb`(無制限の場合は 0)、`role_group_id`(購入後に適用されるロールグループ。空の場合あり)、`active`、`sort_order`。クーポンの `type` は `fixed`(セント単位の金額)または `percent`(割合)で、`min_amount_cents` のしきい値、合計 `max_uses`、ユーザーごとの `per_user_limit`、`expires_at` に対応しています。注文のリクエストボディは `{"plan_id":"…","coupon_code":"…","provider":"manual"}` です。支払いが成功すると(無料注文を含む)、プランのクォータとロールグループが自動的に適用されます。

## ページネーション

`GET /orders` と `GET /tickets` はサーバーサイドのページネーションに対応しています:

| パラメータ | 説明 |
|-----------|-------------|
| `page` | ページ番号。1 から始まり、デフォルトは 1 |
| `page_size` | 1 ページあたりの件数。デフォルトは 20、最大 200 |
| `status` | 注文: `pending` / `paid` / `cancelled`、チケット: `open` / `answered` / `closed` |

`page` または `page_size` を渡すとページネーションオブジェクトを返します。**ページネーションパラメータがない場合は、既存の SDK との互換性のため従来の配列を返します**:

```json
{ "items": [], "total": 0, "page": 1, "page_size": 20 }
```

管理者がこれら 2 つのエンドポイントを呼び出すとサイト全体のデータを受け取りますが、一般ユーザーは自分の注文/チケットのみを参照できます。

`duration_days` が 0 より大きい場合、購入は現在の有効期限(まだ期限切れでない場合)または現在時刻から有効期間を延長します。ユーザーは `plan_expires_at`(`GET /auth/me` が返します)で確認できます。有効期限が切れると、サービスは自動的に(遅くとも 1 時間以内に)ユーザーをデフォルトのロールグループとデフォルトのクォータに戻します。`duration_days` が 0 の場合は永続を意味し、有効期限をクリアします。

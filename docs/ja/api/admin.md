# 管理エンドポイント(管理者権限が必要)

| メソッド | パス | 説明 |
|--------|------|-------------|
| GET | `/admin/stats` | インスタンスの統計情報 |
| GET | `/admin/customers` | 一般ユーザー一覧 |
| GET/POST | `/admin/admins` | 管理者一覧 / 管理者の作成 |
| PATCH/DELETE | `/admin/customers/{id}` | 一般ユーザーの有効化/無効化または削除 |
| PATCH/DELETE | `/admin/admins/{id}` | 管理者の有効化/無効化または削除 |
| GET/POST | `/admin/storage` | ストレージバックエンド一覧 / 作成 |
| GET/PUT/DELETE | `/admin/storage/{id}` | ストレージバックエンドの詳細 / 更新 / 削除 |
| POST | `/admin/storage/{id}/activate` | デフォルトのストレージバックエンドを切り替えます(ホットスイッチ) |
| GET/POST | `/admin/storage/pools` | ストレージプール一覧 / 作成 |
| GET/PUT/DELETE | `/admin/storage/pools/{id}` | ストレージプールの詳細 / 更新 / 削除 |
| POST | `/admin/storage/pools/{id}/activate` | ストレージプールを現在の書き込み先にします |

ストレージバックエンドのシークレット(S3/Qiniu Secret)は暗号化されてデータベースに保存されます。API は設定済みかどうかのみを返し、平文を返すことはありません。

ストレージプールは**新しいオブジェクトをどのバックエンドに書き込むか**を決めるだけです。既存の画像は `images.storage_id` に従って元のバックエンドから読み取られます。`policy` は `weighted_rr`(重み付きラウンドロビン、デフォルト)と `priority`(値が小さいほど優先)に対応しています。ストレージプールを有効にする前に、システムはすべての過去の画像に `storage_id` が記録されているかを確認します。記録されていないものがある場合は有効化を拒否し、先にバックフィルを要求します。これにより、古い画像の読み取りが誤った経路に振り分けられるのを防ぎます。

## 顧客一覧のページネーションとフィルタリング

クエリパラメータを一切指定しない場合、`GET /admin/customers` は従来の動作を維持し、ユーザーの完全な配列を返します(Go/TypeScript SDK はこの形式に依存しています)。次のパラメータのいずれかを指定すると、ページネーションオブジェクトを返します:

| パラメータ | 説明 |
|-----------|-------------|
| `page` | ページ番号。1 から始まります(デフォルト 1) |
| `page_size` | 1 ページあたりの件数(デフォルト 20、最大 200) |
| `keyword` | ユーザー名またはメールアドレスによるあいまい一致 |
| `status` | `enabled` / `disabled`。空の場合はすべて |
| `role_group_id` | ロールグループで絞り込み |
| `sort` | `newest`(デフォルト) / `oldest` / `username` / `usage` |

ページネーションのレスポンス:

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [ /* User */ ],
    "total": 100,
    "page": 1,
    "page_size": 20
  }
}
```

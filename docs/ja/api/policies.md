# ロールグループとポリシー

ロールグループは型付きポリシーのセットをまとめて一般ユーザーに割り当て、ロールごとにリソースと機能を制御します。ロールグループ内では、各型に最大 1 つのポリシーをバインドできます。同じ型を再度バインドすると、古いポリシーが置き換えられます。組み込みのポリシータイプ:

| 型 | 用途 | 設定フィールド(JSON) |
|------|---------|------------------------|
| `quota` | ストレージクォータ | `quota_mb`(0 は無制限) |
| `upload` | 単一ファイルのサイズとメディアタイプ | `max_size_mb`、`allowed_mime_types` |
| `rate` | アップロードと画像読み取りのレート制限 | `upload_per_minute`、`upload_burst`、`image_per_minute`、`image_burst` |
| `processing` | オンザフライの画像処理機能 | `enabled`、`max_width`、`max_height`、`default_quality`、`allowed_formats` |
| `feature` | 機能トグルのセット | `features`(`plaza`、`albums`、`api_tokens`、`batch_upload`、`paste_upload`、`drag_upload`、`embed_code`、`share`、`share_password` など) |

ポリシーのフィールドは任意の上書きです。存在しないフィールドは、設定ファイルのデフォルト値を継承します。初回起動時に、「デフォルトロールグループ」と各型 1 つずつのポリシーが `auth`/`upload`/`processing`/`limits` 設定からシードされます。既存のユーザーは自動的にデフォルトグループに割り当てられます。

アップロードと画像読み取りのレート制限は、呼び出し元のロールグループの `rate` ポリシーに従ってリクエストごとに解決・適用されます(ゲストと匿名リクエストは Guest グループを使用し、デフォルトグループにフォールバックします)。管理者による変更は約 30 秒以内に反映されます。`limits.*` 設定はポリシーのデフォルト値をシードするだけです。

| メソッド | パス | 説明 |
|--------|------|-------------|
| GET | `/auth/policies` | 現在のアカウントに適用される実効ポリシーセット |
| GET/POST | `/admin/role-groups` | ロールグループ一覧 / 作成 |
| GET/PUT/DELETE | `/admin/role-groups/{id}` | ロールグループの詳細 / 更新 / 削除 |
| POST | `/admin/role-groups/{id}/policies` | ポリシーをバインドします(ボディは `{"policy_id":"…"}`。同じ型は置き換えられます) |
| DELETE | `/admin/role-groups/{id}/policies/{policyID}` | バインドを解除します |
| GET/POST | `/admin/policies` | ポリシー一覧(任意で `?type=` を指定) / 作成 |
| GET/PUT/DELETE | `/admin/policies/{id}` | ポリシーの詳細 / 更新 / 削除 |

`PATCH /admin/customers/{id}` では、リクエストボディに `"role_group_id"`(空文字列はデフォルトグループへのフォールバックを意味します)を含めることで、一般ユーザーにロールグループを割り当てられます。割り当て時には、ロールグループの `quota` ポリシーに従ってユーザーのストレージクォータが同期されます。クォータポリシーを変更すると、それを参照しているすべてのロールグループのメンバーのクォータも再同期されます。

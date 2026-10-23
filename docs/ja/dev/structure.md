# ディレクトリ構造

```
cmd/axmipic/          プログラムのエントリーポイント
internal/
  api/                HTTP ルートとハンドラー
  auth/               プリンシパル ID、パスワード、JWT、トークン、レート制限
  config/             設定の読み込みと検証
  imaging/            画像処理（純粋な Go / libvips）
  plugin/             実行時プラグインフレームワーク（WASM サンドボックス、ホストケイパビリティ、レジストリ）
  secret/             AES-256-GCM 暗号化
  server/             HTTP サーバーのライフサイクル
  service/            ビジネスロジック（アカウント、アップロード、処理、ストレージ、管理）
  storage/            ストレージバックエンドとマネージャー（local / S3 / Qiniu）
  store/              データ永続化（GORM）
  webui/              埋め込みフロントエンドアセット
configs/              設定例
web/                  フロントエンドのソース（Vue 3）
```

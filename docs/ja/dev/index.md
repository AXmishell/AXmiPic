# 開発

```bash
make build    # フルビルド: フロントエンド + バックエンドバイナリ
make build-go # バックエンドのみ（既存の internal/webui/dist を再利用）
make web      # フロントエンドのみを internal/webui/dist にビルド
make run      # フロントエンドを先にビルドしてから実行
make vet     # go vet
make test    # go test ./...
make tidy    # go mod tidy
make test-sdk # Go SDK のテストを実行
make sdk     # すべての SDK をビルドしてテスト
make clean   # bin/ と data/ をクリーン
```

## フロントエンド開発

```bash
cd web
pnpm install
pnpm dev        # Vite 開発サーバー（:5173 に直接アクセスする場合、/api と /i は :8080 にプロキシされます）
pnpm typecheck  # vue-tsc による型チェック
```

デフォルトでは、バックエンドはコンパイル時に埋め込まれたフロントエンドのアーティファクトを配信します。開発中に **HMR（ホットモジュールリプレースメント）** を利用するには、設定ファイルでホットリロードモードを有効化し（デフォルトでは無効）、Vite を先に起動します:

```yaml
# configs/config.yaml
web:
  hot_reload:
    enabled: true
    target: "http://127.0.0.1:5173"
```

```bash
cd web && pnpm dev              # ターミナル 1: Vite
./bin/axmipic -config configs/config.yaml   # ターミナル 2: バックエンド（デフォルトは :8080）
```

有効化すると、バックエンドはページ、モジュール、HMR WebSocket を Vite にリバースプロキシするため、
`http://localhost:8080` にアクセスすれば再ビルドなしで変更をライブで確認できます。`/api`、`/i/`、
`/healthz` は常にバックエンドが処理します。このモードはローカル開発専用です。本番環境では無効のままにする
（または設定自体を削除する）ようにし、`make build` を実行して最新のフロントエンドアーティファクトを埋め込んでください。

フロントエンドは `make web` でビルドします（`../internal/webui/dist` にビルドされ、Go の埋め込み用にプレースホルダーファイル
`.gitkeep` が復元されます）。`pnpm build` を直接実行すると `dist` がクリアされ、このプレースホルダーファイルが削除されます。
フロントエンドを変更した後は、埋め込みアセットが更新されるように `make build`（または `make web && make build-go`）を再実行してください。

UI の文言は vue-i18n で管理されており、中国語、英語、日本語に対応しています。詳細は[フロントエンドの国際化（i18n）](/ja/dev/i18n)を参照してください。

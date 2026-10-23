# はじめに

## 動作要件

- Go 1.26 以降
- Node.js 22+ と pnpm 9（フロントエンドのビルドに必要です。`make build` を実行すると自動的にビルドされます）

フロントエンドの成果物はリポジトリにコミットされていません。ビルド時に `internal/webui/dist` へ生成されます。Node を使わずバックエンドのみをビルドする場合は、既存のフロントエンド成果物を再利用して `make build-go` を実行してください。

## ビルドと実行

```bash
# フルビルド（フロントエンド + バックエンド）
make build          # 出力は bin/axmipic

# バックエンドのみ（既存の internal/webui/dist を再利用）
# make build-go

# フロントエンドのみ
# make web

# サンプル設定で実行
cp configs/config.example.yaml configs/config.yaml
./bin/axmipic -config configs/config.yaml
```

または直接実行します:

```bash
make run            # まずフロントエンドをビルドしてから実行
```

デフォルトでは `0.0.0.0:8080` で待ち受け、`http://localhost:8080` を開くとサイトにアクセスできます。初回デプロイ時は `http://localhost:8080/install` を手動で開いてセットアップモードに入り、アプリケーションとデータベースを初期化してください。ホームページは自動的にリダイレクトされません。

## 管理者の作成

方法は 2 つあります:

1. **設定ファイルによるシード**（推奨）: `configs/config.yaml` で `auth.bootstrap_admin: "username:password"` を設定します。管理者テーブルが空のときにのみ有効になります:

   ```yaml
   auth:
     bootstrap_admin: "admin:your-strong-password"
   ```

2. **管理者コンソールで作成**: 既存の管理者としてログインした後、「ユーザー管理 → 管理者」タブで新しい管理者を作成します。

> 初回デプロイ時は、管理者が存在しない状態を避けるために、起動前に `auth.bootstrap_admin` を設定してください。登録エントリーポイントは一般ユーザーのみを作成します。

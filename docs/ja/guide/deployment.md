# Docker デプロイ

リポジトリにはマルチステージの `Dockerfile` が用意されています。フロントエンドは Node でビルドされ、バックエンドは純粋な静的バイナリ（CGO 無効）にクロスコンパイルされ、非 root の distroless イメージで動作し、すべてのデータは `/app/data` 配下に配置されます。

## ビルドと実行

```bash
docker build -t axmipic .

docker run -d --name axmipic \
  -p 8080:8080 \
  -v axmipic-data:/app/data \
  -e AXMIPIC_SERVER_BASE_URL=http://localhost:8080 \
  -e AXMIPIC_AUTH_BOOTSTRAP_ADMIN="admin:your-strong-password" \
  axmipic
```

- コンテナ内ではデフォルトで `/app/configs/config.yaml` が読み込まれます（`configs/config.example.yaml` から生成されます）。`-v /path/config.yaml:/app/configs/config.yaml:ro` でカスタム設定をマウントするか、`AXMIPIC_*` 環境変数で個々の設定を上書きできます。
- `/app/data` には SQLite データベース、ローカルアップロード、自動生成されたマスターキーが保存されます。必ず永続ボリュームをマウントしてください。そうしないと再起動後に暗号化キーが変わってしまいます。
- イメージは非 root ユーザー（uid 65532）で動作します。`AXMIPIC_SERVER_BASE_URL` で `base_url` を実際の公開アドレスに設定してください。

## GHCR イメージの使用

CI は `main` ブランチ、`v*` タグ、手動トリガーでマルチアーキテクチャ（linux/amd64、linux/arm64）イメージをビルドし、GitHub Container Registry へプッシュします:

```bash
docker pull ghcr.io/axmishell/axmipic:latest
```

タグ戦略: デフォルトブランチには `main`、`latest`、`sha-<short>`、バージョンタグには `1.2` / `1.2.3`。Pull Request はビルドのみ（amd64）で、プッシュされません。

## Docker Compose の使用

リポジトリのルートには `docker-compose.yml` があり、デフォルトで `ghcr.io/axmishell/axmipic:latest` を参照し、**セットアップモード**を使用します。完全な内容は次のとおりです（名前付きボリュームを使用、推奨）:

```yaml
services:
  axmipic:
    image: ghcr.io/axmishell/axmipic:latest
    restart: unless-stopped
    ports:
      - "8080:8080"
    # データベースがホスト上で動作している場合、host.docker.internal 経由で到達できます。
    extra_hosts:
      - "host.docker.internal:host-gateway"
    volumes:
      - axmipic-data:/app/data
      - axmipic-config:/app/configs

volumes:
  axmipic-data:
  axmipic-config:
```

起動します:

```bash
docker compose up -d
docker compose logs -f
```

起動後、**`http://<host>:8080/install`** に手動でアクセスし、セットアップモードでデータベース接続、サイトアドレス、管理者アカウントを入力して初期化を完了します。ホームページはこのページへリダイレクトされず、初期化にトークンは不要になりました。

### 1Panel ネットワーク構成

1Panel の `1panel-network` を使用し、PostgreSQL / MySQL も同じネットワーク上のコンテナである場合は、次の完全な構成を使用できます。データベースのホストにはデータベースコンテナ名を入力するだけです:

```yaml
services:
  axmipic:
    image: ghcr.io/axmishell/axmipic:latest
    restart: unless-stopped
    ports:
      - "127.0.0.1:18105:8080"
    # PostgreSQL / MySQL がコンテナではなくホスト上で動作している場合はこれを残し、
    # セットアップウィザードでデータベースホストとして host.docker.internal を使用してください。
    extra_hosts:
      - "host.docker.internal:host-gateway"
    volumes:
      - axmipic-data:/app/data
      - axmipic-config:/app/configs
    networks:
      - 1panel-network

volumes:
  axmipic-data:
  axmipic-config:

networks:
  1panel-network:
    external: true
    name: 1panel-network
```

使用前にネットワークが存在することを確認してください:

```bash
docker network ls | grep 1panel-network
```

ポート `18105` は必要に応じて変更できます。前に 1Panel のリバースプロキシがある場合は、`127.0.0.1` のみで待ち受けていても問題ありません。

### ホストディレクトリのバインドマウント

設定とデータをホストのディレクトリに直接保持したい場合は、次の完全な構成を使用できます（1Panel ネットワークを例にしています）:

```yaml
services:
  axmipic:
    image: ghcr.io/axmishell/axmipic:latest
    restart: unless-stopped
    ports:
      - "127.0.0.1:18105:8080"
    extra_hosts:
      - "host.docker.internal:host-gateway"
    volumes:
      - ./data:/app/data
      - ./config:/app/configs
    networks:
      - 1panel-network

networks:
  1panel-network:
    external: true
    name: 1panel-network
```

ただし、**空のディレクトリを直接マウントすることはできません**。名前付きボリュームと異なり、バインドマウントはイメージに同梱された `config.yaml` を自動的にコピーせず、コンテナは非 root ユーザー（uid 65532）で動作します。まず compose ファイルのあるディレクトリで次を実行してください:

```bash
mkdir -p ./config ./data
cp configs/config.example.yaml ./config/config.yaml
chown -R 65532:65532 ./config ./data
```

ホスト上にリポジトリのソースがない場合は、イメージからデフォルト設定を抽出します:

```bash
docker create --name axmipic-tmp ghcr.io/axmishell/axmipic:latest
docker cp axmipic-tmp:/app/configs/config.yaml ./config/config.yaml
docker rm axmipic-tmp
chown -R 65532:65532 ./config ./data
```

`./data` と `./config` はどちらも uid 65532 で書き込み可能である必要があります。そうでないと、セットアップでデータベースファイル、アップロード、マスターキー、`config.yaml` を保存できません。ホストディレクトリの権限を調整できない場合は、名前付きボリュームを使用してください。

- セットアップモードはデータベース接続などの設定を名前付きボリューム `axmipic-config`（コンテナ内の `/app/configs/config.yaml`）に書き込み、対象のデータベースを初期化します。現在のプロセスは直ちに新しい接続を引き継ぐため、再起動せずに管理者コンソールへログインでき、以降の再起動でもその設定が再利用されます。
- ホストディレクトリのバインドマウントを使用する場合は、上記「ホストディレクトリのバインドマウント」の説明に従って設定を準備し権限を付与してください。空のディレクトリでは設定ファイルが存在しないか、設定の書き戻し時にセットアップが失敗します。
- データ（SQLite データベース、ローカルアップロード、自動生成されたマスターキー）は名前付きボリューム `axmipic-data` に保存されます。`docker compose down` ではデータは削除されませんが、`docker compose down -v` ではデータボリュームも削除されます。
- カスタマイズするには、サービスに `environment:` を追加して `AXMIPIC_*` で個々の設定を上書きするか（たとえば PostgreSQL の場合は `AXMIPIC_DATABASE_DRIVER=postgres`、`AXMIPIC_DATABASE_DSN=host=... port=5432 user=... password=... dbname=... sslmode=disable`）、`configs/config.yaml` をマウントします。`AXMIPIC_INSTALL_DISABLED=true` + `AXMIPIC_AUTH_BOOTSTRAP_ADMIN=admin:<password>` でセットアップモードをスキップすることもできます。
- イメージは distroless で、ヘルスチェックは内蔵されていません。外部から `GET /healthz` をプローブすることでヘルスチェックを行えます。このエンドポイントはプロセスが稼働していることを示すため常に HTTP 200 を返し、レスポンス本文の `data.status`（`ok` / `degraded`）と `data.database.status`（`ok` / `error`）がデータベース接続が利用可能かどうかを反映します。データベースが異常な場合はホームページに警告が表示され、接続が正常な場合は何も表示されません。

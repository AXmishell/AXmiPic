# 設定

設定ファイルは YAML 形式で、`AXMIPIC_` プレフィックスを持つ環境変数による上書きに対応しています（例: `AXMIPIC_SERVER_PORT=9000`）。完全な例は `configs/config.example.yaml` を参照してください。

> **ブートストラップ項目と実行時設定**: `config.yaml` に保持する必要があるのは**ブートストラップ項目**のみです — `server`（listen/`base_url`）、`database`、初期 `storage`、`install`、`logging`、および任意で `auth.encryption_key`/`auth.jwt_secret`（マスターキー）。その他の設定（権限トグル、アップロード/処理/レート制限/セキュリティ/SMS/メンテナンス/サイトなど）はデータベースに保存され、管理者コンソールの「システム設定」で変更できます。初回起動時は設定ファイルがシード用のフォールバックとして使用され、以降はデータベースが優先されます。**`config.yaml` にシークレットを平文で記述しないでください**: 読み戻す必要があるシークレット（ストレージキー、決済キー、SMTP パスワードなど）はデータベースに暗号化して保存され、マスターキーは環境変数または 0600 のキーファイルで提供する必要があります。

## サーバー

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  base_url: "http://localhost:8080"   # ローカル保存画像の公開 URL を生成する際に使用
  trust_proxy: false                    # 信頼できるリバースプロキシ配下にある場合のみ true に設定
  client_ip:
    source: "remote"                    # remote | x-forwarded-for | x-real-ip | cf-connecting-ip | true-client-ip | x-client-ip | forwarded | custom
    header: ""                          # source=custom の場合のヘッダー名
    trusted_proxies: []                 # 信頼できるプロキシの CIDR。プロキシソースで空の場合は remote にフォールバック
    xff_depth: 0                        # 0=右側から最初の信頼できないアドレス
  read_timeout_sec: 30
  write_timeout_sec: 30
  shutdown_timeout_sec: 10
  pprof_enabled: false                   # 管理者認証で保護された pprof をマウント（/api/v1/admin/pprof/）
```

> `X-Forwarded-For` / `X-Real-IP` は `trust_proxy` が `true` の場合にのみ解析されます。直接デプロイの場合は `false` のままにしてください。そうしないと、クライアントが送信元 IP を偽装してレート制限を回避できます。

## フロントエンドのホットリロード（開発）

HMR モードは設定ファイルで有効化し、**デフォルトでは無効**です（コンパイル時に埋め込まれたフロントエンド成果物が使用されます）:

```yaml
web:
  hot_reload:
    enabled: false                    # ローカル開発時は true に変更
    target: "http://127.0.0.1:5173"   # Vite 開発サーバーのアドレス
```

有効化すると、バックエンドは一致しないフロントエンドリクエスト（ページ、モジュール、HMR WebSocket）を `target` へリバースプロキシするため、ブラウザはバックエンドのポート（デフォルト `8080`）にアクセスするだけでよく、フロントエンドの変更は再ビルドやリロードなしで即座にホットリロードされます。`/api`、`/i/`、`/healthz` は引き続きバックエンドが直接処理し、転送されません。

使用方法:

```bash
cd web && pnpm dev        # 先に Vite 開発サーバーを起動
# configs/config.yaml で web.hot_reload.enabled: true を設定してからバックエンドを起動
./bin/axmipic -config configs/config.yaml
```

> ローカル開発専用です。本番環境では有効化しないでください。本番環境ではこのセクションを削除するか `false` のままにし、リリース前に `make build` を実行してフロントエンド成果物を再埋め込みしてください。環境変数で上書きすることもできます: `AXMIPIC_WEB_HOT_RELOAD_ENABLED=true`、`AXMIPIC_WEB_HOT_RELOAD_TARGET=http://127.0.0.1:5173`。

**実際のクライアント IP**: `server.client_ip` はレート制限、ゲストクォータ、ログに使用する IP を制御します。
- `source: remote`（デフォルト）はすべての転送ヘッダーを無視し、ピアアドレスを直接使用します。最も安全な選択肢です。
- リバースプロキシ/Cloudflare 配下では、対応するソース（`x-forwarded-for`、`cf-connecting-ip` など）を選択し、**必ず** `trusted_proxies`（信頼できるプロキシの CIDR）を設定してください。転送ヘッダーは直接のピアが信頼できるネットワーク内にある場合にのみ解析され、それ以外は偽装を防ぐためにピアアドレスが使用されます。
- `x-forwarded-for` / `forwarded` はデフォルトで「右側から最初の信頼できないアドレス」を使用します。`xff_depth` で右側からスキップする信頼できるプロキシの数を指定することもできます。
- 無効なヘッダー値は自動的にピアアドレスへフォールバックします。

## データベース

```yaml
database:
  driver: "sqlite"                      # sqlite | postgres | mysql
  dsn: "./data/axmipic.db"              # sqlite の場合はファイルパス
  # postgres は libpq 接続文字列または URL を使用:
  #   host=127.0.0.1 port=5432 user=axmipic password=secret dbname=axmipic sslmode=disable
  #   postgres://axmipic:secret@127.0.0.1:5432/axmipic?sslmode=disable
  # mysql は go-sql-driver の DSN を使用:
  #   axmipic:secret@tcp(127.0.0.1:3306)/axmipic
  # 接続プールの上限（デフォルト 25 / 5 / 30 分）。0 は無制限を意味します（max_idle_conns が 0 の
  # 場合はアイドル接続を保持しません）。
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime_minutes: 30
```

SQLite は自動的に `busy_timeout` と WAL モードを有効化します。

MySQL 5.7 以降に対応しています（MariaDB 互換）。接続文字列には `parseTime=true`、`charset=utf8mb4`、`loc=Local` が自動的に追加されます（明示的に指定された値は上書きされません）。

MySQL 5.7 のサーバー側デフォルト文字セットは `latin1` です（8.0 以降は `utf8mb4`）。テーブル作成時にアプリケーションは明示的に `DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci` を付与するため、データベースのデフォルト文字セットが `latin1` でも中国語のテキストは正しく保存されます。それでも utf8mb4 でデータベースを作成することを推奨します:

```sql
CREATE DATABASE axmipic CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

データベースに**旧バージョンで作成された** latin1 のテーブルが含まれている場合、起動ログに警告が出力され、テーブル名が一覧表示されます。これらは手動で変換する必要があります（テーブル全体を書き換えるため、オフピーク時に実行してください）:

```sql
ALTER TABLE `images` CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
-- 警告に表示された他のテーブルにも同様に実行します
```

5.7 では、`utf8mb4` で長いインデックス（`images.key` の `varchar(255)` ユニークインデックスなど）をサポートするために、InnoDB のデフォルトの large prefix（`innodb_large_prefix=ON`、`innodb_default_row_format=DYNAMIC`）を維持してください。

## ストレージ

```yaml
storage:
  driver: "local"                       # local | s3 | qiniu | oss | cos
  local:
    root: "./data/uploads"              # ローカルファイルストレージのディレクトリ
  s3:
    endpoint: "https://s3.amazonaws.com"
    region: "us-east-1"
    bucket: "axmipic"
    access_key_id: ""
    secret_access_key: ""
    secure: true
    use_path_style: false
    public_base_url: ""                 # 任意、CDN またはカスタム公開ドメイン
    presign_expiry_sec: 900
  qiniu:
    access_key: ""
    secret_key: ""
    bucket: "axmipic"
    domain: "https://cdn.example.com"
    upload_host: ""
    zone: ""
    private: false
    use_https: true
    presign_expiry_sec: 3600
  oss:                                  # Alibaba Cloud OSS
    endpoint: ""                        # 任意。空の場合は region から導出
    region: "cn-hangzhou"               # oss-cn-hangzhou も指定可能
    bucket: "axmipic"
    access_key_id: ""
    access_key_secret: ""
    domain: ""                          # 任意、カスタム/CDN ドメイン
    private: false
    use_https: true
    presign_expiry_sec: 900
  cos:                                  # Tencent Cloud COS
    bucket: "axmipic-1250000000"        # name-APPID
    region: "ap-guangzhou"
    secret_id: ""
    secret_key: ""
    domain: ""                          # 任意、カスタム/CDN ドメイン
    private: false
    use_https: true
    presign_expiry_sec: 900
```

設定ファイルで構成したストレージは常にフォールバックバックエンドとして機能します。追加のバックエンドは管理者コンソールの「ストレージ設定」で動的に追加し、実行中に切り替えられます。

**ストレージプール**: 「ストレージ設定 → ストレージプール」で複数のバックエンドをプールにまとめ、現在の書き込みプールを選択できます。プールは新しいオブジェクトの書き込み先バックエンドのみを決定します。既存の画像は常に `images.storage_id` に従って元のバックエンドから読み取られます。対応する戦略:

- `weighted_rr`: メンバーの重みによる重み付きラウンドロビン（デフォルト）
- `priority`: `Priority` 値が小さいメンバーを優先し、同じ優先度内では重み付きラウンドロビン。

ストレージプールを有効化する前に、システムはすべての過去の画像に `storage_id` があるかを確認します。初回アップグレード時、古い画像は自動的に以前の現在のバックエンドに割り当てられます。プールのメンバーが無効、または利用可能なメンバーがない場合、アップロードはフォールバックストレージへ黙って書き込むのではなく失敗します。

## アップロードと処理

```yaml
upload:
  max_size_mb: 20
  allowed_mime_types:
    - "image/jpeg"
    - "image/png"
    - "image/gif"
    - "image/webp"

processing:
  enabled: true
  max_width: 4096
  max_height: 4096
  default_quality: 82
  allowed_formats: ["jpeg", "png", "gif", "webp", "avif"]
  cache_mb: 64                          # 派生画像のインプロセスキャッシュ（MiB）、0 で無効
  max_concurrency: 0                    # 同時レンダリング数、0 は CPU コアごとに 1
  max_decode_pixels: 16000000           # 画像ごとのデコード可能ピクセル上限（解凍爆弾の防止 / メモリ上限）
  max_render_memory_mb: 256             # 同時レンダリングの総メモリ予算（MiB）
```

## ランタイムとメモリ

画像のデコードは一時メモリの主な発生源です。以下のオプションでピークを制限し、GC がより積極的にメモリを返却できるようにします:

```yaml
processing:
  # max_concurrency、max_decode_pixels、max_render_memory_mb を下げるとピークメモリを
  # 大幅に削減できます。大きすぎる元画像はプロセスを圧迫する代わりにエラーを返して
  # 拒否されます。
  max_concurrency: 2
  max_decode_pixels: 16000000
  max_render_memory_mb: 256

runtime:
  # GOMEMLIMIT に対応: プロセスのソフトメモリ上限（MiB）。0 は自動を意味し、コンテナの
  # cgroup メモリ上限が検出された場合はその 90% を使用し、それ以外は Go のデフォルト
  # （上限なし）を維持します。コンテナではコンテナ上限の 60〜75% を明示的に設定することを
  # 推奨します。
  memory_limit_mb: 0
  # GOGC に対応: 50 はメモリを節約しますが CPU 負荷が増えます。0 は Go のデフォルト（100）を維持。
  gc_percent: 0

plugins:
  # WASM プラグインごとのリニアメモリ上限（MiB）。0 は組み込みのデフォルト 64 を使用。
  memory_limit_mb: 64
  # アイドル自動アンロード（分）: 有効だがこの時間より長く呼び出されていないプラグインを
  # アンロードしてメモリを解放し、次回呼び出し時に透過的に再読み込みします。
  # 0 で無効。
  idle_unload_minutes: 0
  # 起動時の読み込みモード: lazy（デフォルト）は登録のみ行いインスタンス化せず、初回利用時に
  # 読み込みます。eager は起動時に有効なすべてのプラグインをインスタンス化します。
  startup: "lazy"
  # 起動モードに関係なく即座にウォームアップするプラグイン（たとえば有効な SMS チャネル）。
  # 初回呼び出し時のコールドスタートを避けられます。
  preload: []
```

> プラグインには**組み込み**と**外部**があります。組み込みプラグインはホストのビルドに同梱され（デフォルトで信頼され、インプロセスで動作するためサンドボックス不要）、外部プラグインは `runtime: wasm` を使用して wazero サンドボックスで動作する必要があります。レガシーな `email.*` / `sms.*` 設定は初回マイグレーションにのみ使用され、以降は管理者コンソールの「プラグインマーケット」で一元設定してください。

> プラグインの有効化/一時停止とアイドルアンロードはすべて動的で、再起動は不要です。一時停止するとプラグインの wazero ランタイムが即座に解放され、アイドルアンロードは次回呼び出し時に必要に応じて再構築します。「有効」なプラグインのみが起動時にインスタンス化されるため、一時停止したプラグインは起動時にメモリを消費しません。管理者コンソールの「プラグイン」ページには各プラグインの有効/読み込み状態が表示され、`GET /api/v1/admin/plugins/installed` の `memory_bytes` フィールドには読み込まれた WASM プラグインのリニアメモリ使用量が表示されます。

## 通知チャネル（組み込みプラグイン）

SMS とメールの設定は「プラグインマーケット」へ移行しました。設定ファイルの `sms` / `email` は初回起動時のマイグレーションシードとしてのみ使用されます:

```yaml
plugins:
  # 外部プラグインのディレクトリ。組み込みプラグインにはこのディレクトリは不要です。
  dir: "./plugins"

# 以下はレガシーなブートストラップ項目で、初回マイグレーション時のみ読み取られます。以降はプラグイン設定が優先されます。
sms:
  enabled: false
email:
  enabled: false
```

組み込みの通知プラグイン:

- `builtin-sms-log` / `builtin-email-log`: デバッグ用のログチャネル。デフォルトでは現在のチャネル;
- `builtin-sms-http`: 汎用 HTTP SMS ゲートウェイ（`provider`/`endpoint`/`method`）;
- `builtin-email-smtp`: SMTP メール（`host`/`port`/`username`/`password`/`from`/`use_tls`）。

管理者コンソールの「プラグインマーケット」で各フィールドを設定し、「現在のチャネルに設定」で現在の SMS/メールチャネルを切り替えます。次の API を呼び出すこともできます:
`PUT /api/v1/admin/plugins/active`。暗号文フィールドは AES-256-GCM で暗号化して保存され、API からエコーバックされることはありません。


> 標準の環境変数 `GOMEMLIMIT` / `GOGC` でも上書きでき、同等の効果があります。本番環境で大きな画像を処理する場合は、`-tags libvips` でのビルドを推奨します（メモリと CPU の両面で純粋な Go プロセッサより優れています）。

`server.pprof_enabled` を有効にすると、管理者エンドポイント `GET /api/v1/admin/pprof/` からヒープ/ゴルーチン/プロファイルなどのプロファイリングデータを取得できます（管理者トークンが必要）。トラブルシューティングが完了したら速やかに無効化してください。

## 認証とレート制限

```yaml
auth:
  jwt_secret: ""                        # 空の場合は自動生成して永続化（下記参照）
  encryption_key: ""                    # 任意: ストレージバックエンドのシークレット暗号化にのみ使用。空の場合は jwt_secret にフォールバック
  session_ttl_hours: 24
  allow_registration: true
  require_auth: true                    # true の場合、ログイン済みユーザーのみアップロード可能
  default_quota_mb: 1024                # ユーザーごとのクォータ、0 は無制限
  bootstrap_admin: ""                   # 管理者が存在しない場合にのみシード、形式は "username:password"

limits:
  upload_per_minute: 30
  upload_burst: 5
  guest_per_minute: 6
  guest_burst: 2
  image_per_minute: 600                 # 公開画像の読み取り/処理に対する IP ごとのレート制限
  image_burst: 120
  share_per_minute: 30                  # 共有アクセス（パスワード確認）に対する IP ごとのレート制限。ブルートフォースを防止
  share_burst: 10
```

本番環境およびマルチインスタンスのデプロイでは、必ず固定の `auth.jwt_secret` を明示的に設定してください。`jwt_secret` と `encryption_key` の両方が空の場合、サービスはマスターキーを生成してディスクに永続化します（SQLite の場合はデータベースと同じディレクトリの `.axmipic-key`、それ以外は `./data/.axmipic-key`）。これにより、データベースに暗号化済みのストレージシークレットは再起動後も復号できます。シングルインスタンスのデプロイではこの仕組みに任せれば、そのまま動作します。

## ゲストとセットアップ

```yaml
auth:
  # 未認証のゲストによるアップロードを許可（低権限の Guest ロールグループを使用）。
  allow_guest_upload: true
  # 各ゲスト（署名付き Cookie で識別）のクォータ（MiB）とファイルごとの上限（MiB）。
  guest_quota_mb: 64
  guest_upload_max_mb: 5
  # 24 時間のウィンドウ内でのクライアント IP ごとのゲストアップロード総量（MiB、0 は無制限）。
  guest_ip_quota_mb: 512

install:
  # このロックファイルが存在するとインスタンスは初期化済みと見なされます。削除して再起動すると
  # 再びセットアップモードに入ります。
  lock_file: "./data/install.lock"
  # セットアップモードが書き換える設定ファイルのパス。
  config_path: "./configs/config.yaml"
  # セットアップチェックを完全にスキップ（テスト / コンテナオーケストレーション）。フロントエンドのルートは
  # ロックファイルを自動検出せず、未初期化でもサイトを通常どおり表示します。ユーザーは手動で
  # /install にアクセスする必要があります。
  disabled: false
```

## 管理者セキュアエントリ

セキュアエントリは管理者コンソールの「システム設定 → 実行時設定」でデフォルトで有効化され、データベースに保存されます。エントリの形式は次のとおりです:

```text
https://<site-address>/admin/entry/<32-byte-random-token>
```

有効化すると:

- 通常の `/admin/login` は 404 を返します;
- エントリ Cookie がない場合、管理者ログイン API も 404 を返します;
- セキュアエントリにアクセスすると 10 分間有効なエントリ Cookie（HttpOnly）が設定され、`/admin/login` へリダイレクトされます;
- エントリ Cookie はログイン成功後にクリアされます;
- 有効化、無効化、ローテーションにはいずれも現在のパスワードが必要です。TOTP が有効な場合は動的検証コードも必要です。

トークンは AES-GCM で暗号化してデータベースに保存されます。エントリを忘れた場合や緊急に無効化する必要がある場合は、設定ファイルまたは環境変数で強制的に上書きできます:

```yaml
admin_entry:
  # true の場合、この設定が優先され、管理者コンソールからセキュアエントリを変更できなくなります。
  override: true
  enabled: false
  # enabled=true の場合に必須、16〜128 文字の URL セーフな文字列。
  token: ""
```

対応する環境変数:

```bash
AXMIPIC_ADMIN_ENTRY_OVERRIDE=true
AXMIPIC_ADMIN_ENTRY_ENABLED=false
# 特定のエントリを復元/強制する場合:
# AXMIPIC_ADMIN_ENTRY_ENABLED=true
# AXMIPIC_ADMIN_ENTRY_TOKEN=<token>
```

復元後は管理者コンソールからエントリを再生成し、緊急上書き設定を削除してください。

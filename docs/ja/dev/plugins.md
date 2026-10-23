# プラグインシステム

AXmiPic のプラグインには **組み込みプラグイン**と**外部プラグイン**の 2 種類があります:

| 種類 | 実行方法 | 信頼レベル | 用途 |
|------|----------|----------|----------|
| 組み込み（`builtin`） | ホストと一緒にコンパイルされ、プロセス内の Go `Provider` として動作 | デフォルトで信頼され、**サンドボックスなし**、メモリに常駐 | 低リソースで安定した基本チャネル（ログ出力による SMS、HTTP SMS、SMTP メール） |
| 外部（`external`） | `plugins.dir` から読み込まれ、wazero WASM サンドボックスで実行 | 信頼されず、**サンドボックス必須**、デフォルトで遅延読み込み | サードパーティの SMS プロバイダー、将来的な決済/ストレージチャネル |

組み込みプラグインは `plugin.yaml` を読み込まず、WASM ランタイムのメモリも消費せず、アンインストール・置き換え・一時停止もできません。
外部プラグインは `runtime: wasm` のみを使用でき、`process` やその他のランタイムを宣言したマニフェストは即座に拒否されます。

どちらの種類も同じ `category + descriptor` 契約、および同じ設定・有効化・呼び出しインターフェースを共有します。

## ディレクトリ構造

```
plugins/                   プラグインのルートディレクトリ（デフォルト。plugins.dir で設定可能）
  smsbao/
    plugin.yaml            マニフェスト
    plugin.wasm            ビルド成果物
internal/plugin/           ホストフレームワーク（読み込み、登録、サンドボックス、ホストケイパビリティ）
sdk/plugin-go/             Go プラグイン SDK
  examples/smsbao/         サンプルプラグイン（SMSBao、wasm）
  examples/aliyun-sms/     サンプルプラグイン（Alibaba Cloud SMS、wasm）
  examples/tencent-sms/    サンプルプラグイン（Tencent Cloud SMS、wasm）
```

各 `plugins/<name>/` サブディレクトリが 1 つのプラグインであり、`plugin.yaml` で記述されます。

## マニフェスト `plugin.yaml`

```yaml
name: smsbao              # プラグイン名（一意）
version: 0.1.0
category: notify.sms      # ドメイン名前空間: notify.sms / notify.email / payment / ...
runtime: wasm             # 現在は wasm のみ対応
entry: plugin.wasm        # エントリーファイル（プラグインディレクトリからの相対パス）
abi: 1                    # ホストのバージョンと一致する必要があります
capabilities:             # ケイパビリティ宣言。宣言していないものは利用できません
  http:
    hosts: ["api.smsbao.com", "*.example.com"]
# checksum: sha256:<hex>  # 任意。エントリーファイルの完全性を検証します
```

## プラグインの作成（Go SDK）

プラグインは `Provider` インターフェースを実装し、`init` で自身を登録します:

```go
package main

import plugin "github.com/AXmishell/axmipic/sdk/plugin-go"

type myPlugin struct{ user string }

func (p *myPlugin) Describe() plugin.Descriptor {
    return plugin.Descriptor{Title: "Example", Fields: []plugin.Field{
        {Key: "user", Label: "Account", Required: true},
        {Key: "secret", Label: "Secret", Type: plugin.FieldPassword, Secret: true},
    }}
}

func (p *myPlugin) Configure(cfg map[string]string) error { p.user = cfg["user"]; return nil }

func (p *myPlugin) Invoke(op string, in []byte) ([]byte, error) {
    // ホスト経由で、許可リストに制限された HTTP リクエストを送信します
    resp, err := plugin.HTTPDo(plugin.HTTPRequest{Method: "GET", URL: "https://api.example.com/send"})
    if err != nil { return nil, err }
    _ = resp
    return []byte(`{"ok":true}`), nil
}

func init() { plugin.Register(&myPlugin{}) }

func main() {}
```

WASM リアクターとしてビルドします（`package main` であり、空の `func main(){}` を含んでいる必要があります）:

```bash
GOOS=wasip1 GOARCH=wasm CGO_ENABLED=0 \
  go build -buildmode=c-shared -o plugin.wasm .
```

> `-buildmode=c-shared` は `_initialize` をエクスポートします。ホストはこれを使って、ABI 関数を呼び出す前に Go ランタイムを初期化します。
> 通常の `go build` は `_start` をエクスポートして即座に終了するため、プラグインとして読み込むことはできません。

## ABI

プラグインは以下をエクスポートする必要があります:

| エクスポート | 説明 |
|------|------|
| `alloc(size) -> ptr` | ゲストメモリを確保 |
| `dealloc(ptr, size)` | ゲストメモリを解放 |
| `describe() -> i64` | 自己記述 JSON の `(ptr<<32\|len)` を返す |
| `configure(ptr, len) -> i64` | 設定を注入し、結果エンベロープを返す |
| `invoke(opPtr, opLen, inPtr, inLen) -> i64` | 操作を実行し、結果エンベロープを返す |

ホストはプラグインに `axmipic` という名前のインポートモジュールを提供します:

| インポート | 説明 |
|------|------|
| `http_request(ptr, len) -> i64` | 唯一のネットワーク出口。ホストの許可リスト、メソッド、サイズ、タイムアウトを強制 |
| `log(level, ptr, len)` | 構造化ログ |
| `now_millis() -> i64` | 現在の Unix タイムスタンプ（ミリ秒） |

結果は常にエンベロープ `{"result":<base64>, "error":"..."}` でエンコードされ、`ptr/len` は `(ptr<<32|len)` として
`i64` にパックされます。SDK がこれらの詳細をラップするため、プラグイン作者が手動で扱う必要はありません。

## 設定と読み込み

```yaml
plugins:
  enabled: true
  dir: "./plugins"
  http_timeout_sec: 10
  max_http_body_kb: 1024
  trusted_keys: []          # 信頼する Ed25519 公開鍵（base64）。設定すると、インストール時に署名検証が必須になります
  require_signature: false  # true の場合、有効な署名がないものはすべて拒否されます
  index_url: ""             # プラグインマーケットプレイスのインデックス（JSON）URL
  max_archive_mb: 64        # アーカイブ展開後の最大サイズ
  memory_limit_mb: 64       # WASM プラグインごとのリニアメモリ上限（MiB）。0 = 組み込みのデフォルト 64
  idle_unload_minutes: 0    # アイドル時の自動アンロード時間（分）。0 = 無効
  startup: "lazy"           # lazy（デフォルト）は登録のみでインスタンス化しない。eager は起動時にインスタンス化
  preload: []               # 常にウォームアップするプラグイン（例: アクティブな SMS チャネル）
```

起動時に、ホストはまず組み込みプラグインを登録し（常に有効 + ロード済み）、次に `plugins.dir` をスキャンして
外部プラグインを読み込み、各マニフェストとチェックサムを 1 つずつ検証します。外部プラグインはデフォルトで **lazy** モードを使用します。
つまり登録されるだけでインスタンス化はされず、実際に初回使用時にロードされます。1 つのプラグインの失敗が、
他のプラグインやサービスの起動を妨げることはありません。

## プラグインのライフサイクルとメモリ管理

ホストはプラグインの「**論理的な有効化**」と「**物理的なロード**」を分離しており、その結果 3 つの状態が生まれます
（`state` フィールドとして公開されます）:

| `state` | 論理的に有効 | 物理的にロード済み | 意味 |
|---------|:---:|:---:|------|
| `disabled` | ❌ | ❌ | 一時停止中（管理コンソールで無効化）。ロードされておらず、メモリも消費しない |
| `standby` | ✅ | ❌ | 有効だが**まだアクティブ化されていない**。インスタンス化されておらず、初回呼び出し時にロードされる |
| `active` | ✅ | ✅ | 実行中。インスタンス化され、メモリを保持している |

一度登録された組み込みプラグインは常に `enabled + loaded` であり、一時停止やアイドルアンロードの対象にはなりません。
以下の有効化/一時停止とオンデマンドアクティブ化のルールは外部プラグインに適用されます。

- 永続化されるのは**意図**のみです: `settings` の `plugin.state.<name>` ∈ `enabled` / `disabled`。
- `active` は純粋に実行時の状態であり、プロセス再起動後は `standby` に戻り、永続化されることはありません。
- したがって「有効」≠「メモリを消費する」です。**プラグインは実際に使用されたとき（またはプリロードされたとき）にのみアクティブになります**。

完全なライフサイクル:

1. **起動時（lazy、デフォルト）**: ディレクトリのスキャン時には `plugin.yaml` のみが読み込まれ（何もインスタンス化されません）、
   すべてのプラグインは `standby`/`disabled` として登録され、起動時に wazero ランタイムが作成されることもありません。`startup: "eager"` が設定されている場合は、
   すべての `enabled` プラグインが起動時にインスタンス化されます。どちらのモードでも、`preload`
   リストにあるプラグインは即座にアクティブ化されます。
2. **オンデマンドのアクティブ化**: `Invoke`、`Configure`、通知チャネル（`Sender`）の構築、`Test`、または
   `PUT .../reload` の際に、`standby` 状態のプラグインがその場でインスタンス化され `active` になります。初回呼び出しでは
   1 回限りのコールドスタートコスト（WASM デコード + Go ランタイム初期化）が発生します。
3. **有効化/一時停止**: `PUT .../enabled {"enabled":true}` はプラグインを `standby` にするだけです（**ロードはしません**）。
   `{"enabled":false}` は `disabled` にし、ロード済みの場合は即座に `Close` を呼び出してメモリを解放します。ファイルと設定
   は保持され、状態が永続化されるため再起動後も維持されます。
4. **アイドル時の自動アンロード**: `plugins.idle_unload_minutes > 0` の場合、ディスク上のプラグインで `active` でありながら
   その期間を超えて呼び出されていないものがアンロードされます（`standby` に戻り、**有効なまま**です）。次回呼び出し時には透過的に
   再ロードされ、保存された設定が再適用されます。組み込みプラグインは常駐しており、アイドルアンロードされることはありません。
5. **ディスクリプタキャッシュ**: アンロード/一時停止後、プラグインの `descriptor`（フィールド、タイトル）は `settings` の
   `plugin.desc.<name>` にキャッシュされるため、管理画面のフォームやチャネル一覧は引き続き正しく表示でき、プラグインが
   ロードされていないという理由だけでフィールドが失われることはありません。
6. **シャットダウン**: プロセス終了時に、ロード済みのすべてのインスタンスがクローズされます。

並行性の安全性: `Close` と実行中の `Invoke`/`Configure` 呼び出しはミューテックスで直列化されます。同じプラグインの
初回ロードもロック下で二重チェックされるため、「呼び出し中に一時停止する」場合や初回呼び出しが並行する場合でも、
プラグインが 2 回インスタンス化されたり、wazero ランタイムが同時にクローズされたりすることはありません。アクティブ化に
失敗した場合、プラグインは `standby` のままで `last_error` を記録し、有効化の意図が誤って変更されることはありません。

メモリと状態の可視性:

- `GET /api/v1/admin/plugins/installed` は各プラグインの `state`（disabled/standby/active）、
  `builtin`、`sandboxed`、`enabled`、`loaded`、`memory_bytes`（ロード済み WASM プラグインのリニアメモリのバイト数。
  組み込みプラグインは 0）、`last_used`（Unix 秒。0 はロード後に呼び出されていないことを意味します）、`last_error` を返します。
- `GET /api/v1/admin/plugins?category=` は**有効な**（`standby`/`active`）プラグインのみを返します。
  一時停止中のプラグインはチャネル選択に表示されませんが、そのディスクリプタと設定は `/installed` から引き続き表示・編集できます。
- 管理コンソールの「プロセス情報」エンドポイントとオプションの pprof エンドポイントと併せて、全体的なメモリ使用量を把握できます。

> 実践的なアドバイス: デフォルトの `lazy` モードでは、一度も呼び出されないプラグインはメモリをまったく消費しません。SMS
> チャネルを 1 つだけ使用する場合は、他のチャネルを**一時停止**すれば一切ロードされません。アクティブなチャネルを `preload` に追加するか、`idle_unload_minutes`
> （例: `30`〜`60`）を設定することで、「即時応答」と「メモリ節約」を必要に応じて調整できます。

## 管理 API

| メソッド | パス | 説明 |
|------|------|------|
| GET | `/api/v1/admin/plugins?category=` | 有効なプラグインの自己記述を一覧表示（任意でカテゴリ指定） |
| GET | `/api/v1/admin/plugins/installed` | インストール済みのすべてのプラグインとその**状態**を一覧表示: `state`(disabled/standby/active)、`enabled`、`loaded`、`memory_bytes`、`last_used`、`last_error`、`configured` |
| GET | `/api/v1/admin/plugins/active?category=` | 通知カテゴリで現在使用されているプラグインを取得 |
| PUT | `/api/v1/admin/plugins/active` | 現在のチャネルを設定: `{"category":"notify.sms","name":"builtin-sms-http"}` |
| GET | `/api/v1/admin/plugins/registry` | プラグインマーケットプレイスのインデックスを取得 |
| POST | `/api/v1/admin/plugins/install` | プラグインをインストール（URL/インデックス名による JSON、または生のアーカイブボディ） |
| GET | `/api/v1/admin/plugins/{name}` | プラグインの設定を取得。シークレットフィールドは設定済みかどうかのみを返す |
| PUT | `/api/v1/admin/plugins/{name}/config` | 設定を保存し、即座に適用（シークレットは暗号化して保存） |
| PUT | `/api/v1/admin/plugins/{name}/enabled` | プラグインを有効化/一時停止（`{"enabled":bool}`、状態は永続化） |
| POST | `/api/v1/admin/plugins/{name}/test` | 現在の設定でテスト通知を送信 |
| POST | `/api/v1/admin/plugins/{name}/reload` | プラグインを再ロード（有効状態は維持） |
| DELETE | `/api/v1/admin/plugins/{name}` | 外部プラグインをアンインストールして削除（組み込みプラグインは削除不可） |

管理コンソールの「プラグインストア」はプラグイン設定の**唯一のエントリーポイント**です。プラグインの `fields` から動的フォームをレンダリングし、
通常のフィールドはそのまま表示し、`secret` フィールドは決してエコーバックされないパスワード入力として表示します。保存すると設定が即座に適用されます。
フィールドは `visible_when: {key: provider, values: [official]}` で表示条件を宣言でき、コンソールはそれを使って関連フィールドを
動的に表示/非表示にします。
組み込みプラグインもここで設定しますが、その設定は編集のみ可能で、アンインストール・一時停止・再ロードはできません。

## 通知チャネル（組み込み + 外部）

SMS とメールはどちらもプラグイン経由で送信されます。起動時に以下の組み込みプラグインが登録されます:

| プラグイン名 | カテゴリ | 説明 | 主なフィールド |
|--------|------|------|----------|
| `builtin-sms-log` | `notify.sms` | ログ出力による SMS（デバッグ用） | なし |
| `builtin-sms-http` | `notify.sms` | 汎用 HTTP SMS ゲートウェイ | `provider`、`endpoint`、`method` |
| `builtin-email-log` | `notify.email` | ログ出力によるメール（デバッグ用） | なし |
| `builtin-email-smtp` | `notify.email` | SMTP メール（STARTTLS / 暗黙的 TLS） | `host`、`port`、`username`、`password`（暗号化）、`from`、`use_tls` |

外部 SMS プラグイン（`smsbao` など）は、組み込みプラグインと同じインストール、設定、テスト、呼び出しのフローを共有します。各
通知カテゴリは**現在のチャネル**を `plugin.active.<category>` に記録します:

- `PUT /api/v1/admin/plugins/active` は現在のチャネルを設定し、保存後すぐに有効になります。
- 明示的に設定されていない場合、システムは `builtin-sms-log` / `builtin-email-log` にフォールバックします。
- 現在のチャネルが一時停止またはアンインストールされている場合、送信パスは自動的に組み込みのログチャネルにフォールバックし、警告をログに記録します。
- ホストは保存されたプラグイン設定で `Configure` を介してインスタンスを構成し、`notify.Sender` をプラグインの
  `invoke("send", ...)` にブリッジします。メッセージペイロードは中立な形式（`to`/`subject`/`body`/`template`/
  `params`/`sign_name`）で、プラグインが各プロバイダー向けにマッピングします。

> **旧バージョンからの移行**: 初回起動時に、`settings.smtp` とレガシーな `settings.sms` は自動的に
> `builtin-email-smtp` / `builtin-sms-http` のプラグイン設定へ移行され、現在のチャネルが設定されます。移行は
> 1 回だけ実行されます（`plugin.migrated.notify_v1`）。旧通知設定ページは削除されました。「プラグインストア」を使用してください。

## X リンクのインポート（組み込みリゾルバープラグイン）

`builtin-x-resolver` は `import.x` カテゴリに属し、ホストに同梱される組み込みプラグインです。X/Twitter のステータスリンクを
作者と画像 URL に解決することだけを担当し、**データベースやストレージに直接アクセスすることはありません**。
画像のダウンロード、アップロード、重複排除、クォータ、ギャラリーのモデレーションはホストの X インポートサービスが処理します。

```yaml
# 組み込みプラグインは plugin.yaml を必要としません。同等の記述は以下のとおりです
name: builtin-x-resolver
category: import.x
runtime: builtin
abi: 1
```

設定フィールド（プラグインストア → 設定）:

| フィールド | 説明 |
|------|------|
| `provider` | `fxtwitter`（無料、デフォルト）または `official`（公式 X API v2） |
| `bearer_token` | 公式 API 用の Bearer トークン。暗号化して保存 |
| `timeout_sec` | 解決リクエストのタイムアウト。デフォルトは 15 秒 |
| `image_variant` | `orig` / `large` / `medium` |
| `proxy_url` | 任意のプロキシ URL。`http://`、`https://`、`socks5://`、`socks5h://` に対応。空の場合はシステムの環境プロキシを使用 |
| `proxy_username` | プロキシ認証のユーザー名（任意） |
| `proxy_password` | プロキシ認証のパスワード。暗号化して保存（任意） |

`resolve` プラグイン操作の入出力:

```json
// 入力
{ "url": "https://x.com/user/status/123" }

// 出力
{
  "platform": "x",
  "post_id": "123",
  "source_url": "https://x.com/user/status/123",
  "author": { "name": "Display Name", "handle": "user" },
  "media": [
    { "id": "456", "type": "photo", "url": "https://pbs.twimg.com/...", "width": 928, "height": 1232 }
  ]
}
```

ホストは `https://pbs.twimg.com` / `*.twimg.com` からの画像 URL のみを受け入れ、`photo`
タイプのみをインポートします。動画と GIF はまだサポートされていません。

`fxtwitter` はサードパーティの公開エンドポイントであり、その可用性はメンテナーに依存します。高い安定性が求められるデプロイでは、
公式 X API v2 の Bearer トークンを設定してください。

## オンラインインストールと署名

プラグインのアーカイブは zip です（ルートに `plugin.yaml` とエントリーファイルを含みます）。公開ツール `pluginpack`
は `sdk/plugin-go/cmd/pluginpack` にあります:

```bash
cd sdk/plugin-go
# パッケージ化と署名（秘密鍵は base64 の 32 バイトシード、または 64 バイトの Ed25519 秘密鍵）
go run ./cmd/pluginpack -dir ../../plugins/smsbao -out smsbao-0.1.0.zip -key "$PRIVATE_KEY"
# sha256=... と signature=... を出力します
```

インストール方法:

- **ローカルアップロード**: `POST /api/v1/admin/plugins/install` に生の zip バイト列をボディとして送信し、任意でヘッダー
  `X-Plugin-Sha256` と `X-Plugin-Signature` を指定します。
- **URL / インデックス名による指定**: JSON ボディ `{"url":"...","sha256":"...","signature":"..."}`、
  または `{"name":"smsbao"}` を `index_url` 経由でマーケットプレイスから解決します。

有効化/一時停止: `PUT /api/v1/admin/plugins/{name}/enabled` にボディ `{"enabled":true|false}` を送信します。
`lazy` モードでは、`{"enabled":true}` はプラグインを `standby` にするだけで（**すぐにはロードされません**）、初回使用時に
アクティブ化されます。`{"enabled":false}` は登録を解除し、（ディスク上のプラグインについては）インスタンスをクローズして
**即座にメモリを解放**しますが、ファイルと設定は保持されます。状態は永続化され、再起動後も維持されます。一時停止中のプラグインを現在の通知
チャネルに設定することはできませんが、そのディスクリプタと設定は（`plugin.desc.<name>` キャッシュから）引き続き表示・編集できます。有効な
プラグインのアイドル自動アンロードについては、上記の「プラグインのライフサイクルとメモリ管理」を参照してください。

ホストはサイズ、sha256、Ed25519 署名を検証し（`trusted_keys` または
`require_signature` が設定されている場合は必須）、その後アーカイブを安全に展開し（パストラバーサル、シンボリックリンク、過大なコンテンツを拒否）、
マニフェストを検証して、プラグインをホットロードします。

`index_url` にあるマーケットプレイスインデックスの JSON 構造:

```json
{
  "plugins": [
    {
      "name": "smsbao",
      "version": "0.1.0",
      "category": "notify.sms",
      "runtime": "wasm",
      "description": "SMSBao",
      "url": "https://example.com/smsbao-0.1.0.zip",
      "sha256": "…",
      "signature": "…"
    }
  ]
}
```

## セキュリティモデル

- WASM サンドボックス: リニアメモリを制限し（デフォルト 64 MiB、`plugins.memory_limit_mb` で設定可能）、コンテキストごとのタイムアウトによる中断をサポートします。
- ケイパビリティ許可リスト: `capabilities.http.hosts` に含まれないホストは常に拒否されます。宣言されていない場合、すべてのネットワークアクセスが無効になります。
- 外部プラグインは WASM ランタイムのみを使用でき、常に wazero サンドボックス内で実行されます。
- 組み込みプラグインはホストと一緒にコンパイル・配布され、デフォルトで信頼され、プロセス内で実行されます。外部プラグインは組み込みの身元を偽装できません
  （`builtin-` の名前プレフィックスと `runtime: builtin` の両方が拒否されます）。
- シークレットフィールドは、ホストが AES-256-GCM で暗号化してから `settings` テーブルに書き込みます。AAD はプラグイン名とフィールド名をバインドし、
  API レスポンスに平文が含まれることはありません。
- オプションの `checksum` はエントリーファイルの改ざんを防ぎます。

## ビルドとテスト

```bash
make plugin-example   # すべてのサンプル WASM プラグインを plugins/ にビルド
make test-plugin      # プラグインフレームワークのテストを実行（WASM のエンドツーエンドを含む）
```

## ロードマップ

1. ~~**M2**: プラグインを `notify` に接続し、管理画面の動的フォームと認証情報の暗号化、テスト送信を実装。~~ ✅
2. ~~**M3**: Alibaba Cloud / Tencent Cloud SMS プラグイン。~~ ✅（`sdk/plugin-go/examples/` を参照）
3. ~~**M5**: オンラインインストール、Ed25519 署名検証、プラグインマーケットプレイスのインデックス。~~ ✅
4. ~~**プラグインのライフサイクル**: オンデマンドアクティブ化（デフォルトは lazy、初回使用時にロード）、プリロードによるウォームアップ、アイドル自動アンロード、3 状態の可視化。~~ ✅
5. ~~**組み込み/外部の分離**: 組み込み通知プラグイン（SMS/メール）はサンドボックス不要。外部プラグインは WASM サンドボックスに強制。~~ ✅

## 同梱のサンプルプラグイン

| プラグイン | ディレクトリ | ランタイム | 説明 |
|------|------|--------|------|
| SMSBao | `examples/smsbao` | wasm | 汎用 HTTP + MD5 署名 |
| Alibaba Cloud SMS | `examples/aliyun-sms` | wasm | RPC + HMAC-SHA1 署名。テンプレートパラメータは `params` から取得 |
| Tencent Cloud SMS | `examples/tencent-sms` | wasm | TC3-HMAC-SHA256 署名、JSON POST |

すべてのクラウドプロバイダープラグインは、Go 標準ライブラリを使用してプラグイン内で署名を行うため、ホストにプロバイダー固有の依存関係は不要です。
`make plugin-example` を実行すると、すべてのサンプルが `plugins/` にビルドされ、その後、管理コンソールの「プラグインストア」で設定して現在の SMS チャネルに設定できます。

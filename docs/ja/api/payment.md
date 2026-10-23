# 決済チャネル

決済は `internal/payment` のプラグイン可能な `Gateway` インターフェースを通じて実装されており、5 つの組み込みチャネルがあります:

| チャネル | 説明 |
|---------|-------------|
| `manual` | 手動確認: 注文後、管理者が管理者コンソールで支払いを確認します |
| `mock` | モックチェックアウト: 開発専用です。一般ユーザーが自分で支払いを完了できます |
| `alipay` | Alipay 対面決済(QR コード)。注文には RSA2 署名、コールバックには署名検証を使用します |
| `wechat` | WeChat Pay v3 Native QR コード。公式 SDK(`wechatpay-apiv3/wechatpay-go`)に基づき、注文作成とコールバックの署名検証/復号を処理します |
| `epay` | YiPay(彩虹易支付互換)のアグリゲート決済。注文には MD5 署名、非同期通知には検証を使用します |

`payment.default_gateway` でデフォルトのチャネルを選択します。Alipay の場合は `payment.alipay` 配下で `app_id`、`private_key`(アプリケーション秘密鍵)、`public_key`(Alipay 公開鍵)を設定します。WeChat の場合は `payment.wechat` 配下で `app_id`、`mch_id`、`serial_no`(マーチャント証明書のシリアル番号)、`private_key`(マーチャント API 秘密鍵)、`api_v3_key`(32 バイト)、`platform_public_key`(`Wechatpay-Signature` コールバックヘッダーの RSA 検証に**必須**の WeChat Pay プラットフォーム証明書公開鍵)、および任意の `platform_serial_no` を設定します。プラットフォーム公開鍵がない場合、WeChat チャネルは構築を拒否し、検証されていないコールバックによる支払い確認を防ぎます。YiPay の場合は `payment.epay` 配下で `pid`(マーチャント ID)、`key`(MD5 キー)、`gateway_url`(サイトのルートアドレス)を設定し、任意で `api_url`、`submit_url`、`pay_type`(`alipay`/`wxpay`/`qqpay` など)を設定します。認証情報が揃い、`enabled: true` が設定されると、そのチャネルは起動時に登録されます。

実際の支払いをバイパスできないようにするため、一般ユーザーは `mock` 注文のみ自分で完了できます。`manual` 注文は管理者が `POST /api/v1/admin/orders/{id}/pay` で確認する必要があり、`alipay`/`wechat`/`epay` 注文は決済コールバックによってのみ確認でき、コールバックの金額は注文金額と照合されます。

決済設定は管理者コンソールの「システム設定 → 決済設定」からオンラインで維持することもできます。`GET /api/v1/admin/payment` で読み取り(シークレットは設定済みかどうかのみを返します)、`PUT /api/v1/admin/payment` で保存し、即時反映されます(シークレットフィールドが空の場合は既存の値を維持します)。決済の認証情報はマスターキーで暗号化され、データベースの `settings` テーブルに保存されます。設定ファイルの決済設定は初回起動時のフォールバックとして機能し、データベースに書き込まれます。

注文のリクエストボディの `provider` フィールドでチャネルを選択します。決済結果のコールバックアドレスは `POST /api/v1/payments/{provider}/callback` です(Alipay と YiPay はプレーンテキストの `success` を返し、WeChat は 200 を返します)。`GET /api/v1/payment-gateways` は登録済みチャネルの一覧を返します。

# Payment Channels

Payments are implemented through the pluggable `Gateway` interface in `internal/payment`, with five built-in channels:

| Channel | Description |
|---------|-------------|
| `manual` | Manual confirmation: after placing an order, an administrator confirms payment in the admin console |
| `mock` | Mock checkout: for development only; regular users can complete payment themselves |
| `alipay` | Alipay Face-to-Face Payment (QR code), with RSA2 signing for orders and signature verification for callbacks |
| `wechat` | WeChat Pay v3 Native QR code, based on the official SDK (`wechatpay-apiv3/wechatpay-go`), handling order creation plus callback signature verification/decryption |
| `epay` | YiPay (Rainbow YiPay compatible) aggregated payment, with MD5 signing for orders and asynchronous notification verification |

`payment.default_gateway` selects the default channel. For Alipay, configure `app_id`, `private_key` (application private key), and `public_key` (Alipay public key) under `payment.alipay`. For WeChat, configure `app_id`, `mch_id`, `serial_no` (merchant certificate serial number), `private_key` (merchant API private key), `api_v3_key` (32 bytes), `platform_public_key` (WeChat Pay platform certificate public key, **required** for RSA verification of the `Wechatpay-Signature` callback header), and the optional `platform_serial_no` under `payment.wechat`; without the platform public key the WeChat channel refuses to be constructed, preventing confirmation of payments through unverified callbacks. For YiPay, configure `pid` (merchant ID), `key` (MD5 key), and `gateway_url` (site root address) under `payment.epay`, with optional `api_url`, `submit_url`, and `pay_type` (`alipay`/`wxpay`/`qqpay`, etc.). Once credentials are complete and `enabled: true` is set, the channel is registered at startup.

To prevent bypassing real payments, regular users can only complete `mock` orders themselves; `manual` orders must be confirmed by an administrator via `POST /api/v1/admin/orders/{id}/pay`; and `alipay`/`wechat`/`epay` orders can only be confirmed by payment callbacks, with callback amounts checked against the order amount.

Payment settings can also be maintained online in the admin console under "System Settings → Payment Settings": `GET /api/v1/admin/payment` reads them (secrets only report whether they are set), and `PUT /api/v1/admin/payment` saves them with immediate effect (an empty secret field means keep the existing value). Payment credentials are encrypted with the master key and stored in the `settings` table in the database; payment configuration in the configuration file serves as a fallback for the first startup and is written to the database.

The `provider` field in the order request body selects the channel; the payment result callback address is `POST /api/v1/payments/{provider}/callback` (Alipay and YiPay return plain-text `success`; WeChat returns 200). `GET /api/v1/payment-gateways` returns the list of registered channels.

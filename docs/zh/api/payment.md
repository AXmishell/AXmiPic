# 支付渠道

支付通过 `internal/payment` 的可插拔 `Gateway` 接口实现，内置五种渠道：

| 渠道 | 说明 |
|------|------|
| `manual` | 人工核销：下单后由管理员在后台确认付款 |
| `mock` | 模拟收银台：仅用于开发，普通用户可自助完成支付 |
| `alipay` | 支付宝当面付（扫码），RSA2 签名下单与回调验签 |
| `wechat` | 微信支付 v3 Native 扫码，基于官方 SDK（`wechatpay-apiv3/wechatpay-go`）完成下单与回调验签/解密 |
| `epay` | 易支付（彩虹易支付兼容）聚合支付，MD5 签名下单与异步通知验签 |

`payment.default_gateway` 选择默认渠道。支付宝在 `payment.alipay` 配置 `app_id`、`private_key`（应用私钥）、`public_key`（支付宝公钥）；微信在 `payment.wechat` 配置 `app_id`、`mch_id`、`serial_no`（商户证书序列号）、`private_key`（商户 API 私钥）、`api_v3_key`（32 字节）、`platform_public_key`（微信支付平台证书公钥，**必填**，用于对回调 `Wechatpay-Signature` 做 RSA 验签）与可选的 `platform_serial_no`；缺少平台公钥时微信渠道将拒绝构造，避免未验签的回调确认支付。易支付在 `payment.epay` 配置 `pid`（商户号）、`key`（MD5 密钥）、`gateway_url`（站点根地址），可选 `api_url`、`submit_url`、`pay_type`（`alipay`/`wxpay`/`qqpay` 等）。凭据齐备并置 `enabled: true` 后渠道会在启动时注册。

为避免绕过真实支付，普通用户只能自助完成 `mock` 订单；`manual` 订单需管理员通过 `POST /api/v1/admin/orders/{id}/pay` 核销；`alipay`/`wechat`/`epay` 订单只能由支付回调确认，且回调金额会与订单金额核对。

支付设置也可在后台「系统设置 → 支付设置」中在线维护：`GET /api/v1/admin/payment` 读取（密钥仅返回是否已设置），`PUT /api/v1/admin/payment` 保存并即时生效（密钥字段为空表示保持原值）。支付凭据经主密钥加密后保存在数据库的 `settings` 表中；配置文件中的支付配置作为首次启动的兜底并写入数据库。

下单请求体的 `provider` 字段选择渠道；支付结果回调地址为 `POST /api/v1/payments/{provider}/callback`（支付宝与易支付返回纯文本 `success`，微信返回 200）。`GET /api/v1/payment-gateways` 返回已注册渠道列表。

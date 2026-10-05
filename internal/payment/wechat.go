package payment

import (
	"bytes"
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	wechatpay "github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/core/auth"
	"github.com/wechatpay-apiv3/wechatpay-go/core/notify"
	"github.com/wechatpay-apiv3/wechatpay-go/core/option"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/native"
)

// WechatProductionGateway 是微信支付 API v3 的生产地址。
const WechatProductionGateway = "https://api.mch.weixin.qq.com"

// WechatGateway 基于微信支付官方 SDK（wechatpay-apiv3/wechatpay-go）实现
// Native 扫码支付：使用 SDK 完成请求签名与 Native 下单，并使用 SDK 的通知
// 处理器对回调做验签与 AES-256-GCM 解密。
type WechatGateway struct {
	appID      string
	mchID      string
	serialNo   string
	apiV3Key   string
	privateKey *rsa.PrivateKey
	// platformPublicKey 为微信支付公钥；非空时对回调验签。
	platformPublicKey *rsa.PublicKey
	// platformSerialNo 为公钥 ID；非空时校验回调头中的 serial。
	platformSerialNo string
	gatewayURL       string
	httpClient       *http.Client

	once      sync.Once
	client    *wechatpay.Client
	clientErr error
}

// WechatOptions 是构造 WechatGateway 所需的参数。
type WechatOptions struct {
	AppID      string
	MchID      string
	SerialNo   string
	PrivateKey string
	APIv3Key   string
	GatewayURL string
	// PlatformPublicKey 为微信支付平台证书公钥（PEM 或裸 base64）；配置后
	// 对回调的 Wechatpay-Signature 做 RSA 验签。强烈建议配置。
	PlatformPublicKey string
	// PlatformSerialNo 为公钥 ID；配置后校验回调头中的 serial。
	PlatformSerialNo string
	HTTPClient       *http.Client
}

// NewWechatGateway 构造一个微信支付渠道。app_id、mch_id、serial_no、商户私钥、
// APIv3 密钥与平台公钥均为必需；缺少平台公钥将无法验签回调，因此拒绝构造。
func NewWechatGateway(opts WechatOptions) (*WechatGateway, error) {
	if strings.TrimSpace(opts.AppID) == "" || strings.TrimSpace(opts.MchID) == "" {
		return nil, fmt.Errorf("payment: wechat app_id and mch_id are required")
	}
	if strings.TrimSpace(opts.SerialNo) == "" {
		return nil, fmt.Errorf("payment: wechat serial_no is required")
	}
	if len(opts.APIv3Key) != 32 {
		return nil, fmt.Errorf("payment: wechat api_v3_key must be 32 bytes")
	}
	if strings.TrimSpace(opts.PlatformPublicKey) == "" {
		return nil, fmt.Errorf("payment: wechat platform_public_key is required to verify callbacks")
	}
	priv, err := parseRSAPrivateKey(opts.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("payment: wechat private key: %w", err)
	}
	platformKey, err := parseRSAPublicKey(opts.PlatformPublicKey)
	if err != nil {
		return nil, fmt.Errorf("payment: wechat platform public key: %w", err)
	}
	gatewayURL := strings.TrimSpace(opts.GatewayURL)
	if gatewayURL == "" {
		gatewayURL = WechatProductionGateway
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &WechatGateway{
		appID:             opts.AppID,
		mchID:             opts.MchID,
		serialNo:          opts.SerialNo,
		apiV3Key:          opts.APIv3Key,
		privateKey:        priv,
		platformPublicKey: platformKey,
		platformSerialNo:  strings.TrimSpace(opts.PlatformSerialNo),
		gatewayURL:        gatewayURL,
		httpClient:        client,
	}, nil
}

// Name 返回渠道标识。
func (g *WechatGateway) Name() string { return "wechat" }

// Create 通过官方 SDK 调用 Native 下单接口，返回二维码链接。
func (g *WechatGateway) Create(ctx context.Context, order Order) (*CreateResult, error) {
	client, err := g.sdkClient()
	if err != nil {
		return nil, fmt.Errorf("payment: wechat client: %w", err)
	}
	service := native.NativeApiService{Client: client}
	resp, _, err := service.Prepay(ctx, native.PrepayRequest{
		Appid:       wechatpay.String(g.appID),
		Mchid:       wechatpay.String(g.mchID),
		Description: wechatpay.String(truncate(order.Subject, 127)),
		OutTradeNo:  wechatpay.String(order.ID),
		NotifyUrl:   wechatpay.String(order.NotifyURL),
		Amount: &native.Amount{
			Total:    wechatpay.Int64(order.AmountCents),
			Currency: wechatpay.String("CNY"),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("payment: wechat prepay: %w", err)
	}
	if resp == nil || resp.CodeUrl == nil || *resp.CodeUrl == "" {
		return nil, fmt.Errorf("payment: wechat returned an empty code_url")
	}
	return &CreateResult{TradeNo: order.ID, PayURL: *resp.CodeUrl}, nil
}

// VerifyCallback 使用官方 SDK 的通知处理器校验签名并解密回调。
func (g *WechatGateway) VerifyCallback(ctx context.Context, header http.Header, raw []byte) (*Callback, error) {
	handler, err := notify.NewRSANotifyHandler(g.apiV3Key, wechatPubkeyVerifier{
		publicKey: g.platformPublicKey,
		serial:    g.platformSerialNo,
	})
	if err != nil {
		return nil, fmt.Errorf("payment: wechat notify handler: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/", bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("payment: wechat callback request: %w", err)
	}
	if header != nil {
		req.Header = header.Clone()
	}
	var transaction wechatTransaction
	parsed, err := handler.ParseNotifyRequest(ctx, req, &transaction)
	if err != nil {
		return nil, fmt.Errorf("payment: wechat callback verify: %w", err)
	}
	if parsed.EventType != "TRANSACTION.SUCCESS" {
		return nil, fmt.Errorf("payment: wechat event %q is not a successful payment", parsed.EventType)
	}
	return transaction.toCallback(), nil
}

// sdkClient 惰性构建官方 SDK 客户端：使用商户私钥签名，且不校验应答签名
// （应答验签需要平台证书，可选）。
func (g *WechatGateway) sdkClient() (*wechatpay.Client, error) {
	g.once.Do(func() {
		opts := []wechatpay.ClientOption{
			option.WithHTTPClient(g.sdkHTTPClient()),
			option.WithMerchantCredential(g.mchID, g.serialNo, g.privateKey),
			option.WithoutValidator(),
		}
		g.client, g.clientErr = wechatpay.NewClient(context.Background(), opts...)
	})
	return g.client, g.clientErr
}

// sdkHTTPClient 返回 SDK 使用的 HTTP 客户端。当配置了自定义网关地址时，通过
// 重写请求主机把请求指向该地址（便于沙箱与测试）。
func (g *WechatGateway) sdkHTTPClient() *http.Client {
	target, err := url.Parse(g.gatewayURL)
	if err != nil || target.Host == "" || g.gatewayURL == WechatProductionGateway {
		return g.httpClient
	}
	clone := *g.httpClient
	transport := clone.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	clone.Transport = &hostRewriteTransport{base: transport, scheme: target.Scheme, host: target.Host}
	return &clone
}

// hostRewriteTransport 把请求重定向到目标主机。
type hostRewriteTransport struct {
	base   http.RoundTripper
	scheme string
	host   string
}

// RoundTrip 重写请求 URL 后转发。
func (t *hostRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = t.scheme
	req.URL.Host = t.host
	return t.base.RoundTrip(req)
}

// wechatTransaction 是微信支付回调中解密后的交易信息。
type wechatTransaction struct {
	OutTradeNo    string `json:"out_trade_no"`
	TransactionID string `json:"transaction_id"`
	TradeState    string `json:"trade_state"`
	SuccessTime   string `json:"success_time"`
	Amount        struct {
		Total int64 `json:"total"`
	} `json:"amount"`
}

// toCallback 把交易信息转换为通用回调结果。
func (t wechatTransaction) toCallback() *Callback {
	paidAt := time.Now()
	if parsed, err := time.Parse(time.RFC3339, t.SuccessTime); err == nil {
		paidAt = parsed
	}
	return &Callback{
		OrderID:     t.OutTradeNo,
		TradeNo:     t.TransactionID,
		Success:     t.TradeState == "SUCCESS",
		PaidAt:      paidAt,
		AmountCents: t.Amount.Total,
		HasAmount:   true,
	}
}

// wechatPubkeyVerifier 使用微信支付公钥验签，实现 SDK 的 auth.Verifier 接口。
// serial 非空时校验回调头中的序列号。
type wechatPubkeyVerifier struct {
	publicKey *rsa.PublicKey
	serial    string
}

// Verify 校验签名，签名原文格式为 timestamp\nnonce\nbody\n。
func (v wechatPubkeyVerifier) Verify(_ context.Context, serial, message, signature string) error {
	if v.serial != "" && serial != v.serial {
		return fmt.Errorf("payment: wechat callback serial mismatch")
	}
	return verifyRSASignature(message, signature, v.publicKey)
}

// GetSerial 返回可验签的公钥序列号。
func (v wechatPubkeyVerifier) GetSerial(context.Context) (string, error) {
	return v.serial, nil
}

// 确保实现 SDK 的 auth.Verifier 接口。
var _ auth.Verifier = wechatPubkeyVerifier{}

// verifyRSASignature 校验 base64 编码的 SHA256-RSA 签名。
func verifyRSASignature(message, signature string, key *rsa.PublicKey) error {
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("payment: signature is not base64 encoded")
	}
	digest := sha256.Sum256([]byte(message))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig); err != nil {
		return fmt.Errorf("payment: signature mismatch: %w", err)
	}
	return nil
}

// decryptAESGCM 使用 APIv3 密钥（AES-256-GCM）解密回调资源。
func decryptAESGCM(key []byte, ciphertext, nonce, associatedData string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("base64: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short")
	}
	plaintext, err := gcm.Open(nil, []byte(nonce), data, []byte(associatedData))
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

package payment

import (
	"bytes"
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// WechatProductionGateway 是微信支付 API v3 的生产地址。
const WechatProductionGateway = "https://api.mch.weixin.qq.com"

// WechatGateway 实现微信支付 v3 的 Native 扫码下单：使用商户私钥对请求做
// SHA256-RSA 签名，使用 APIv3 密钥解密回调中的支付结果。
type WechatGateway struct {
	appID      string
	mchID      string
	serialNo   string
	apiV3Key   []byte
	privateKey *rsa.PrivateKey
	gatewayURL string
	httpClient *http.Client
}

// WechatOptions 是构造 WechatGateway 所需的参数。
type WechatOptions struct {
	AppID      string
	MchID      string
	SerialNo   string
	PrivateKey string
	APIv3Key   string
	GatewayURL string
	HTTPClient *http.Client
}

// NewWechatGateway 构造一个微信支付渠道。
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
	priv, err := parseRSAPrivateKey(opts.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("payment: wechat private key: %w", err)
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
		appID:      opts.AppID,
		mchID:      opts.MchID,
		serialNo:   opts.SerialNo,
		apiV3Key:   []byte(opts.APIv3Key),
		privateKey: priv,
		gatewayURL: strings.TrimRight(gatewayURL, "/"),
		httpClient: client,
	}, nil
}

// Name 返回渠道标识。
func (g *WechatGateway) Name() string { return "wechat" }

// Create 调用微信支付 Native 下单接口，返回二维码链接。
func (g *WechatGateway) Create(ctx context.Context, order Order) (*CreateResult, error) {
	bizContent, err := json.Marshal(map[string]any{
		"appid":        g.appID,
		"mchid":        g.mchID,
		"description":  truncate(order.Subject, 127),
		"out_trade_no": order.ID,
		"notify_url":   order.NotifyURL,
		"amount": map[string]any{
			"total":    order.AmountCents,
			"currency": "CNY",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("payment: wechat biz content: %w", err)
	}
	endpoint := g.gatewayURL + "/v3/pay/transactions/native"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bizContent))
	if err != nil {
		return nil, fmt.Errorf("payment: wechat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if err := g.signRequest(req, bizContent); err != nil {
		return nil, fmt.Errorf("payment: wechat sign: %w", err)
	}
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("payment: wechat call: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("payment: wechat read: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("payment: wechat rejected order (%d): %s", resp.StatusCode, string(body))
	}
	var parsed struct {
		CodeURL string `json:"code_url"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("payment: wechat decode: %w", err)
	}
	if parsed.CodeURL == "" {
		return nil, fmt.Errorf("payment: wechat returned an empty code_url")
	}
	return &CreateResult{TradeNo: order.ID, PayURL: parsed.CodeURL}, nil
}

// WechatCallbackHeaders 携带微信支付回调的签名头，供平台证书校验使用。
type WechatCallbackHeaders struct {
	Signature string
	Timestamp string
	Nonce     string
	Serial    string
}

// VerifyCallback 解密并校验微信支付 v3 回调。raw 为回调请求体。签名头校验需要
// 微信平台证书公钥；本实现完成资源解密与业务校验，签名头通过
// WithWechatHeaders 传入以便后续扩展。
func (g *WechatGateway) VerifyCallback(ctx context.Context, raw []byte) (*Callback, error) {
	_ = ctx
	var notification struct {
		EventType string `json:"event_type"`
		Resource  struct {
			Algorithm      string `json:"algorithm"`
			Ciphertext     string `json:"ciphertext"`
			Nonce          string `json:"nonce"`
			AssociatedData string `json:"associated_data"`
		} `json:"resource"`
	}
	if err := json.Unmarshal(raw, &notification); err != nil {
		return nil, fmt.Errorf("payment: wechat callback decode: %w", err)
	}
	if notification.EventType != "TRANSACTION.SUCCESS" {
		return nil, fmt.Errorf("payment: wechat event %q is not a successful payment", notification.EventType)
	}
	plaintext, err := g.decryptResource(
		notification.Resource.Ciphertext,
		notification.Resource.Nonce,
		notification.Resource.AssociatedData,
	)
	if err != nil {
		return nil, fmt.Errorf("payment: wechat decrypt: %w", err)
	}
	var transaction struct {
		OutTradeNo    string `json:"out_trade_no"`
		TransactionID string `json:"transaction_id"`
		TradeState    string `json:"trade_state"`
		SuccessTime   string `json:"success_time"`
	}
	if err := json.Unmarshal(plaintext, &transaction); err != nil {
		return nil, fmt.Errorf("payment: wechat transaction decode: %w", err)
	}
	paidAt := time.Now()
	if t, err := time.Parse(time.RFC3339, transaction.SuccessTime); err == nil {
		paidAt = t
	}
	return &Callback{
		OrderID: transaction.OutTradeNo,
		TradeNo: transaction.TransactionID,
		Success: transaction.TradeState == "SUCCESS",
		PaidAt:  paidAt,
	}, nil
}

// signRequest 为请求计算微信支付 v3 的 Authorization 头。
func (g *WechatGateway) signRequest(req *http.Request, body []byte) error {
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	nonce, err := randomHex(16)
	if err != nil {
		return err
	}
	message := strings.Join([]string{
		req.Method,
		req.URL.Path,
		timestamp,
		nonce,
		string(body),
	}, "\n")
	signature, err := signSHA256RSA(message, g.privateKey)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", fmt.Sprintf(
		`WECHATPAY2-SHA256-RSA2048 mchid="%s",nonce_str="%s",signature="%s",timestamp="%s",serial_no="%s"`,
		g.mchID, nonce, signature, timestamp, g.serialNo,
	))
	return nil
}

// decryptResource 使用 APIv3 密钥（AES-256-GCM）解密回调资源。
func (g *WechatGateway) decryptResource(ciphertext, nonce, associatedData string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("base64: %w", err)
	}
	block, err := aes.NewCipher(g.apiV3Key)
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

// signSHA256RSA 使用商户私钥对消息做 SHA256-RSA 签名并返回 base64 结果。
func signSHA256RSA(message string, key *rsa.PrivateKey) (string, error) {
	digest := sha256.Sum256([]byte(message))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

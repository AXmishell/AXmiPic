package payment

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// AlipayProductionGateway 是支付宝网关的生产地址。
const AlipayProductionGateway = "https://openapi.alipay.com/gateway.do"

// AlipayGateway 实现支付宝当面付（alipay.trade.precreate）扫码支付：
// 使用应用私钥做 RSA2 签名请求，使用支付宝公钥校验异步通知。
type AlipayGateway struct {
	appID      string
	gatewayURL string
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	httpClient *http.Client
	// method 为交易接口方法名，默认 alipay.trade.precreate。
	method string
}

// AlipayOptions 是构造 AlipayGateway 所需的参数。
type AlipayOptions struct {
	AppID      string
	PrivateKey string
	PublicKey  string
	GatewayURL string
	// HTTPClient 可选；默认使用带超时的客户端。
	HTTPClient *http.Client
}

// NewAlipayGateway 构造一个支付宝渠道。私钥与公钥缺一不可。
func NewAlipayGateway(opts AlipayOptions) (*AlipayGateway, error) {
	if strings.TrimSpace(opts.AppID) == "" {
		return nil, fmt.Errorf("payment: alipay app_id is required")
	}
	priv, err := parseRSAPrivateKey(opts.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("payment: alipay private key: %w", err)
	}
	pub, err := parseRSAPublicKey(opts.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("payment: alipay public key: %w", err)
	}
	gatewayURL := strings.TrimSpace(opts.GatewayURL)
	if gatewayURL == "" {
		gatewayURL = AlipayProductionGateway
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &AlipayGateway{
		appID:      opts.AppID,
		gatewayURL: gatewayURL,
		privateKey: priv,
		publicKey:  pub,
		httpClient: client,
		method:     "alipay.trade.precreate",
	}, nil
}

// Name 返回渠道标识。
func (g *AlipayGateway) Name() string { return "alipay" }

// Create 调用支付宝当面付下单接口，返回二维码链接。
func (g *AlipayGateway) Create(ctx context.Context, order Order) (*CreateResult, error) {
	bizContent, err := json.Marshal(map[string]any{
		"out_trade_no": order.ID,
		"total_amount": formatYuan(order.AmountCents),
		"subject":      truncate(order.Subject, 256),
	})
	if err != nil {
		return nil, fmt.Errorf("payment: alipay biz content: %w", err)
	}
	params := url.Values{}
	params.Set("app_id", g.appID)
	params.Set("method", g.method)
	params.Set("format", "JSON")
	params.Set("charset", "utf-8")
	params.Set("sign_type", "RSA2")
	params.Set("timestamp", time.Now().Format("2006-01-02 15:04:05"))
	params.Set("version", "1.0")
	params.Set("notify_url", order.NotifyURL)
	params.Set("biz_content", string(bizContent))

	signature, err := signAlipay(params, g.privateKey)
	if err != nil {
		return nil, fmt.Errorf("payment: alipay sign: %w", err)
	}
	params.Set("sign", signature)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.gatewayURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("payment: alipay request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=utf-8")
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("payment: alipay call: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("payment: alipay read: %w", err)
	}

	var parsed struct {
		Response struct {
			Code    string `json:"code"`
			Msg     string `json:"msg"`
			Message string `json:"sub_msg"`
			QRCode  string `json:"qr_code"`
		} `json:"alipay_trade_precreate_response"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("payment: alipay decode: %w", err)
	}
	if parsed.Response.Code != "10000" {
		return nil, fmt.Errorf("payment: alipay rejected order: %s %s", parsed.Response.Msg, parsed.Response.Message)
	}
	return &CreateResult{TradeNo: order.ID, PayURL: parsed.Response.QRCode}, nil
}

// VerifyCallback 校验支付宝异步通知的 RSA2 签名，并解析支付结果。raw 为通知
// 表单的原始查询串（application/x-www-form-urlencoded）。
func (g *AlipayGateway) VerifyCallback(_ context.Context, raw []byte) (*Callback, error) {
	values, err := url.ParseQuery(string(raw))
	if err != nil {
		return nil, fmt.Errorf("payment: alipay callback parse: %w", err)
	}
	signature := values.Get("sign")
	if signature == "" {
		return nil, fmt.Errorf("payment: alipay callback missing signature")
	}
	if !verifyAlipay(values, g.publicKey) {
		return nil, fmt.Errorf("payment: alipay callback signature mismatch")
	}
	if values.Get("app_id") != g.appID {
		return nil, fmt.Errorf("payment: alipay callback app_id mismatch")
	}
	status := values.Get("trade_status")
	success := status == "TRADE_SUCCESS" || status == "TRADE_FINISHED"
	paidAt := time.Now()
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", values.Get("gmt_payment"), time.Local); err == nil {
		paidAt = t
	}
	return &Callback{
		OrderID: values.Get("out_trade_no"),
		TradeNo: values.Get("trade_no"),
		Success: success,
		PaidAt:  paidAt,
	}, nil
}

// signAlipay 对参数（排除 sign 与空值）按字典序拼接后做 RSA2(SHA256) 签名。
func signAlipay(params url.Values, key *rsa.PrivateKey) (string, error) {
	content := canonicalAlipayParams(params)
	digest := sha256.Sum256([]byte(content))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// verifyAlipay 使用公钥校验支付宝参数签名。
func verifyAlipay(params url.Values, key *rsa.PublicKey) bool {
	signature := params.Get("sign")
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return false
	}
	content := canonicalAlipayParams(params)
	digest := sha256.Sum256([]byte(content))
	return rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig) == nil
}

// canonicalAlipayParams 生成待签名串：剔除 sign/sign_type 与空值，按 key 字典序
// 以 k=v& 形式拼接。
func canonicalAlipayParams(params url.Values) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if k == "sign" || k == "sign_type" {
			continue
		}
		if strings.TrimSpace(params.Get(k)) == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(params.Get(k))
		b.WriteString("&")
	}
	return strings.TrimSuffix(b.String(), "&")
}

// formatYuan 把分转换为支付宝要求的元字符串（两位小数）。
func formatYuan(cents int64) string {
	negative := cents < 0
	if negative {
		cents = -cents
	}
	value := fmt.Sprintf("%d.%02d", cents/100, cents%100)
	if negative {
		return "-" + value
	}
	return value
}

// truncate 按符文截断字符串到最多 n 个字符。
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

// parseRSAPrivateKey 解析 PKCS1/PKCS8 的 RSA 私钥；支持 PEM 与裸 base64。
func parseRSAPrivateKey(raw string) (*rsa.PrivateKey, error) {
	der, err := decodePEMorBase64(raw, "RSA PRIVATE KEY", "PRIVATE KEY")
	if err != nil {
		return nil, err
	}
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("parse pkcs8: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA private key")
	}
	return key, nil
}

// parseRSAPublicKey 解析 RSA 公钥；支持 PEM 与裸 base64。
func parseRSAPublicKey(raw string) (*rsa.PublicKey, error) {
	der, err := decodePEMorBase64(raw, "PUBLIC KEY")
	if err != nil {
		return nil, err
	}
	if key, err := x509.ParsePKIXPublicKey(der); err == nil {
		if rsaKey, ok := key.(*rsa.PublicKey); ok {
			return rsaKey, nil
		}
		return nil, fmt.Errorf("not an RSA public key")
	}
	key, err := x509.ParsePKCS1PublicKey(der)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	return key, nil
}

// decodePEMorBase64 将 PEM 块或裸 base64 字符串解码为 DER 字节。
func decodePEMorBase64(raw string, blockTypes ...string) ([]byte, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("empty key")
	}
	if strings.Contains(trimmed, "-----BEGIN") {
		block, _ := pem.Decode([]byte(trimmed))
		if block == nil {
			return nil, fmt.Errorf("invalid PEM block")
		}
		return block.Bytes, nil
	}
	// 去掉换行与空白后按 base64 解码。
	cleaned := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, trimmed)
	der, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	return der, nil
}

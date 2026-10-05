package payment

import (
	"context"
	"crypto/md5"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// 易支付（彩虹易支付及其兼容实现）默认接口路径与签名算法。
const (
	epayDefaultAPIURL    = "/mapi.php"
	epayDefaultSubmitURL = "/submit.php"
	epaySignTypeMD5      = "MD5"
)

// EpayGateway 实现「易支付」聚合支付协议（彩虹易支付兼容）：使用商户 PID 与
// 密钥做 MD5 签名下单，并对异步通知做签名校验。它不依赖任何第三方 SDK，直接
// 对接官方 HTTP 接口。支付方式是网关侧的通道标识（alipay / wxpay / qqpay 等）。
type EpayGateway struct {
	pid        string
	key        string
	payType    string
	apiURL     string
	submitURL  string
	signType   string
	httpClient *http.Client
}

// EpayOptions 是构造 EpayGateway 所需的参数。
type EpayOptions struct {
	// PID 为易支付商户号。
	PID string
	// Key 为商户密钥（用于 MD5 签名）。
	Key string
	// GatewayURL 为易支付站点根地址，例如 https://pay.example.com。
	GatewayURL string
	// APIURL 为下单接口地址；留空时使用 <GatewayURL>/mapi.php。
	APIURL string
	// SubmitURL 为收银台地址；留空时使用 <GatewayURL>/submit.php。
	SubmitURL string
	// PayType 为支付通道：alipay、wxpay、qqpay 等；留空默认 alipay。
	PayType string
	// HTTPClient 可选；默认使用带超时的客户端。
	HTTPClient *http.Client
}

// NewEpayGateway 构造一个易支付渠道。PID、密钥与网关地址缺一不可。
func NewEpayGateway(opts EpayOptions) (*EpayGateway, error) {
	pid := strings.TrimSpace(opts.PID)
	if pid == "" {
		return nil, fmt.Errorf("payment: epay pid is required")
	}
	key := strings.TrimSpace(opts.Key)
	if key == "" {
		return nil, fmt.Errorf("payment: epay key is required")
	}
	base := strings.TrimRight(strings.TrimSpace(opts.GatewayURL), "/")
	apiURL := strings.TrimSpace(opts.APIURL)
	if apiURL == "" {
		if base == "" {
			return nil, fmt.Errorf("payment: epay gateway_url is required")
		}
		apiURL = base + epayDefaultAPIURL
	}
	submitURL := strings.TrimSpace(opts.SubmitURL)
	if submitURL == "" {
		if base == "" {
			return nil, fmt.Errorf("payment: epay gateway_url is required")
		}
		submitURL = base + epayDefaultSubmitURL
	}
	payType := strings.TrimSpace(opts.PayType)
	if payType == "" {
		payType = "alipay"
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &EpayGateway{
		pid:        pid,
		key:        key,
		payType:    payType,
		apiURL:     apiURL,
		submitURL:  submitURL,
		signType:   epaySignTypeMD5,
		httpClient: client,
	}, nil
}

// Name 返回渠道标识。
func (g *EpayGateway) Name() string { return "epay" }

// Create 通过 mapi.php 接口下单；接口不可用时回退为收银台跳转链接。
func (g *EpayGateway) Create(ctx context.Context, order Order) (*CreateResult, error) {
	params := g.buildParams(order)
	payURL, tradeNo, err := g.createViaAPI(ctx, params)
	if err != nil {
		// mapi.php 不可用时回退到 submit.php 跳转（GET 形式，多数站点兼容）。
		if g.submitURL != "" {
			return &CreateResult{TradeNo: order.ID, PayURL: g.submitURL + "?" + params.Encode()}, nil
		}
		return nil, err
	}
	return &CreateResult{TradeNo: tradeNo, PayURL: payURL}, nil
}

// buildParams 构造下单参数并追加签名。
func (g *EpayGateway) buildParams(order Order) url.Values {
	params := url.Values{}
	params.Set("pid", g.pid)
	params.Set("type", g.payType)
	params.Set("out_trade_no", order.ID)
	params.Set("notify_url", order.NotifyURL)
	params.Set("return_url", order.ReturnURL)
	params.Set("name", truncate(order.Subject, 127))
	params.Set("money", formatYuan(order.AmountCents))
	params.Set("sign_type", g.signType)
	params.Set("sign", epaySign(params, g.key))
	return params
}

// createViaAPI 调用 mapi.php 并解析返回的支付地址。
func (g *EpayGateway) createViaAPI(ctx context.Context, params url.Values) (payURL, tradeNo string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.apiURL, strings.NewReader(params.Encode()))
	if err != nil {
		return "", "", fmt.Errorf("payment: epay request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=utf-8")
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("payment: epay call: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", "", fmt.Errorf("payment: epay read: %w", err)
	}
	var parsed struct {
		Code      json.RawMessage `json:"code"`
		Msg       string          `json:"msg"`
		TradeNo   string          `json:"trade_no"`
		PayURL    string          `json:"payurl"`
		QRCode    string          `json:"qrcode"`
		URLScheme string          `json:"urlscheme"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", "", fmt.Errorf("payment: epay decode: %w", err)
	}
	if !epaySuccessCode(parsed.Code) {
		return "", "", fmt.Errorf("payment: epay rejected order: %s", parsed.Msg)
	}
	payURL = firstNonEmpty(parsed.PayURL, parsed.QRCode, parsed.URLScheme)
	if payURL == "" {
		return "", "", fmt.Errorf("payment: epay returned an empty pay url")
	}
	if parsed.TradeNo != "" {
		tradeNo = parsed.TradeNo
	}
	return payURL, tradeNo, nil
}

// VerifyCallback 校验易支付异步通知的 MD5 签名并解析支付结果。raw 为通知表单的
// 原始查询串（application/x-www-form-urlencoded）。
func (g *EpayGateway) VerifyCallback(_ context.Context, _ http.Header, raw []byte) (*Callback, error) {
	values, err := url.ParseQuery(string(raw))
	if err != nil {
		return nil, fmt.Errorf("payment: epay callback parse: %w", err)
	}
	if values.Get("sign") == "" {
		return nil, fmt.Errorf("payment: epay callback missing signature")
	}
	if !g.verify(values) {
		return nil, fmt.Errorf("payment: epay callback signature mismatch")
	}
	if values.Get("pid") != g.pid {
		return nil, fmt.Errorf("payment: epay callback pid mismatch")
	}
	status := values.Get("trade_status")
	success := status == "TRADE_SUCCESS"
	callback := &Callback{
		OrderID: values.Get("out_trade_no"),
		TradeNo: values.Get("trade_no"),
		Success: success,
		PaidAt:  time.Now(),
	}
	if money := values.Get("money"); money != "" {
		if cents, err := parseYuan(money); err == nil {
			callback.AmountCents = cents
			callback.HasAmount = true
		}
	}
	return callback, nil
}

// verify 使用商户密钥校验签名（常量时间比较）。
func (g *EpayGateway) verify(params url.Values) bool {
	want := epaySign(params, g.key)
	got := strings.ToLower(strings.TrimSpace(params.Get("sign")))
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// epaySign 计算易支付的 MD5 签名：剔除 sign/sign_type 与空值后按 key 字典序拼接
// 为 k=v&k=v，末尾直接追加密钥再取 MD5 小写十六进制。
func epaySign(params url.Values, key string) string {
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
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(params.Get(k))
	}
	b.WriteString(key)
	sum := md5.Sum([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// epaySuccessCode 判断易支付返回的业务码是否表示成功（1 或 "1"）。
func epaySuccessCode(raw json.RawMessage) bool {
	s := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	return s == "1"
}

// firstNonEmpty 返回第一个非空字符串。
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

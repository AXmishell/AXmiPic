// Package payment 提供可插拔的支付渠道抽象。真实渠道（支付宝、微信）可在此
// 接口下实现签名、下单与回调校验；未配置凭据时回退到日志/模拟渠道。
package payment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// ErrUnsupported 在渠道不支持某项操作时返回。
var ErrUnsupported = errors.New("payment: operation is not supported by this gateway")

// Order 是发起支付所需的订单信息。
type Order struct {
	ID          string
	Subject     string
	AmountCents int64
	// NotifyURL 为支付结果回调地址。
	NotifyURL string
	// ReturnURL 为支付完成后跳转地址。
	ReturnURL string
}

// CreateResult 是发起支付的结果。对于扫码/跳转支付，PayURL 由客户端展示。
type CreateResult struct {
	// TradeNo 为渠道流水号（若由渠道生成）。
	TradeNo string
	// PayURL 为收银台或二维码地址。
	PayURL string
}

// Callback 是支付回调的解析结果。
type Callback struct {
	OrderID string
	TradeNo string
	Success bool
	PaidAt  time.Time
}

// Gateway 是支付渠道的通用接口。
type Gateway interface {
	// Name 返回渠道标识，例如 alipay、wechat、manual、mock。
	Name() string
	// Create 发起一笔支付。
	Create(ctx context.Context, order Order) (*CreateResult, error)
	// VerifyCallback 校验并解析支付回调的原始请求。
	VerifyCallback(ctx context.Context, raw []byte) (*Callback, error)
}

// ---- 手动渠道 ----

// ManualGateway 不接入任何支付机构，仅把订单标记为待人工确认。管理员可在后台
// 手动核销订单。
type ManualGateway struct{}

// Name 返回渠道标识。
func (ManualGateway) Name() string { return "manual" }

// Create 返回一个占位结果，表示订单已创建、等待人工确认。
func (ManualGateway) Create(_ context.Context, order Order) (*CreateResult, error) {
	return &CreateResult{TradeNo: "manual-" + order.ID}, nil
}

// VerifyCallback 对手动渠道不可用。
func (ManualGateway) VerifyCallback(_ context.Context, _ []byte) (*Callback, error) {
	return nil, ErrUnsupported
}

// ---- 模拟渠道 ----

// MockGateway 用于开发与测试：立即返回一个可访问的收银台地址，并接受任何
// 回调（提示：生产环境不要启用）。
type MockGateway struct {
	// BaseURL 为收银台地址前缀。
	BaseURL string
	logger  *slog.Logger
}

// NewMockGateway 构造一个模拟渠道。
func NewMockGateway(baseURL string, logger *slog.Logger) *MockGateway {
	return &MockGateway{BaseURL: baseURL, logger: logger}
}

// Name 返回渠道标识。
func (g *MockGateway) Name() string { return "mock" }

// Create 生成一个假的支付链接。
func (g *MockGateway) Create(_ context.Context, order Order) (*CreateResult, error) {
	tradeNo := "mock-" + randomHex(8)
	payURL := fmt.Sprintf("%s/pay/mock?order=%s&amount=%d", g.BaseURL, order.ID, order.AmountCents)
	if g.logger != nil {
		g.logger.Info("mock payment created",
			slog.String("order", order.ID),
			slog.Int64("amount_cents", order.AmountCents),
		)
	}
	return &CreateResult{TradeNo: tradeNo, PayURL: payURL}, nil
}

// VerifyCallback 接受任意回调（仅用于开发）。
func (g *MockGateway) VerifyCallback(_ context.Context, _ []byte) (*Callback, error) {
	return nil, ErrUnsupported
}

// randomHex 返回 n 字节的随机十六进制字符串。
func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "0000000000000000"
	}
	return hex.EncodeToString(buf)
}

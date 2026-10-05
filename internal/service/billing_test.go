package service_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/payment"
	"github.com/AXmishell/axmipic/internal/service"
	"github.com/AXmishell/axmipic/internal/store"
)

func newBilling(t *testing.T) (*service.BillingService, *store.Repository) {
	t.Helper()
	repo := newRepo(t)
	svc := service.NewBillingService(repo, "http://example.test",
		[]payment.Gateway{payment.ManualGateway{}, payment.NewMockGateway("http://example.test", nil)}, "manual")
	return svc, repo
}

func TestPlanAndFreeOrder(t *testing.T) {
	svc, repo := newBilling(t)
	ctx := context.Background()
	newCustomer(t, repo, "u1")
	u1 := &auth.Principal{UserID: "u1", Username: "u1", Role: auth.RoleUser}

	plan, err := svc.CreatePlan(ctx, service.PlanInput{Name: "免费套餐", PriceCents: 0, QuotaMB: 256, Active: true})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	if _, err := svc.CreatePlan(ctx, service.PlanInput{Name: "免费套餐", Active: true}); !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("duplicate plan err = %v, want ErrInvalidInput", err)
	}

	order, err := svc.CreateOrder(ctx, u1, plan.ID, "", "")
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	// 免费订单创建后即完成。
	if order.Status != store.OrderPaid {
		t.Fatalf("free order status = %q, want paid", order.Status)
	}
	account, err := repo.GetAccountByID(ctx, store.RoleCustomer, "u1")
	if err != nil {
		t.Fatalf("GetAccountByID: %v", err)
	}
	if account.QuotaBytes != 256<<20 {
		t.Fatalf("quota = %d, want %d", account.QuotaBytes, int64(256<<20))
	}

	if _, err := svc.CreateOrder(ctx, u1, "missing", "", ""); !errors.Is(err, service.ErrPlanNotFound) {
		t.Fatalf("missing plan err = %v, want ErrPlanNotFound", err)
	}
}

func TestCouponDiscountAndLimits(t *testing.T) {
	svc, repo := newBilling(t)
	ctx := context.Background()
	newCustomer(t, repo, "u1")
	u1 := &auth.Principal{UserID: "u1", Username: "u1", Role: auth.RoleUser}

	plan, err := svc.CreatePlan(ctx, service.PlanInput{Name: "付费套餐", PriceCents: 1000, QuotaMB: 512, Active: true})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	fixed, err := svc.CreateCoupon(ctx, service.CouponInput{Code: "save100", Type: store.CouponFixed, Value: 300, PerUserLimit: 1, Active: true})
	if err != nil {
		t.Fatalf("CreateCoupon fixed: %v", err)
	}
	_ = fixed

	dto, discount, err := svc.ValidateCoupon(ctx, u1, "SAVE100", 1000)
	if err != nil || discount != 300 || dto.Code != "SAVE100" {
		t.Fatalf("ValidateCoupon = %+v, %d, %v", dto, discount, err)
	}

	order, err := svc.CreateOrder(ctx, u1, plan.ID, "save100", "mock")
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if order.AmountCents != 700 || order.DiscountCents != 300 {
		t.Fatalf("order amount = %d discount = %d", order.AmountCents, order.DiscountCents)
	}
	if order.Status != store.OrderPending {
		t.Fatalf("paid order status = %q, want pending", order.Status)
	}

	// 支付后应用配额。
	paid, err := svc.PayOrder(ctx, u1, order.ID)
	if err != nil {
		t.Fatalf("PayOrder: %v", err)
	}
	if paid.Status != store.OrderPaid {
		t.Fatalf("status = %q, want paid", paid.Status)
	}
	account, _ := repo.GetAccountByID(ctx, store.RoleCustomer, "u1")
	if account.QuotaBytes != 512<<20 {
		t.Fatalf("quota = %d, want 512MiB", account.QuotaBytes)
	}

	// 达到每用户使用上限后不可再用。
	if _, err := svc.CreateOrder(ctx, u1, plan.ID, "save100", ""); !errors.Is(err, service.ErrCouponInvalid) {
		t.Fatalf("second use err = %v, want ErrCouponInvalid", err)
	}

	// 门槛校验。
	premium, _ := svc.CreateCoupon(ctx, service.CouponInput{Code: "big", Type: store.CouponFixed, Value: 500, MinAmountCents: 5000, Active: true})
	if _, _, err := svc.ValidateCoupon(ctx, u1, premium.Code, 1000); !errors.Is(err, service.ErrCouponBelowMinimum) {
		t.Fatalf("below minimum err = %v, want ErrCouponBelowMinimum", err)
	}
	if _, _, err := svc.ValidateCoupon(ctx, u1, "missing", 1000); !errors.Is(err, service.ErrCouponNotFound) {
		t.Fatalf("missing coupon err = %v, want ErrCouponNotFound", err)
	}
}

func TestPercentCoupon(t *testing.T) {
	repo := newRepo(t)
	svc2 := service.NewBillingService(repo, "http://example.test", []payment.Gateway{payment.ManualGateway{}}, "manual")
	ctx := context.Background()
	newCustomer(t, repo, "u1")
	u1 := &auth.Principal{UserID: "u1", Username: "u1", Role: auth.RoleUser}
	plan, _ := svc2.CreatePlan(ctx, service.PlanInput{Name: "P", PriceCents: 2000, Active: true})
	if _, err := svc2.CreateCoupon(ctx, service.CouponInput{Code: "off20", Type: store.CouponPercent, Value: 20, Active: true}); err != nil {
		t.Fatalf("CreateCoupon: %v", err)
	}
	order, err := svc2.CreateOrder(ctx, u1, plan.ID, "off20", "")
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if order.DiscountCents != 400 || order.AmountCents != 1600 {
		t.Fatalf("order = %+v, want 400 discount / 1600 amount", order)
	}
}

func TestTicketLifecycle(t *testing.T) {
	svc, _ := newBilling(t)
	ctx := context.Background()
	user := &auth.Principal{UserID: "u1", Username: "u1", Role: auth.RoleUser}
	admin := &auth.Principal{UserID: "a1", Username: "a1", Role: auth.RoleAdmin}

	ticket, err := svc.CreateTicket(ctx, user, "无法上传", "upload", "上传时出错", "high")
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	if ticket.Status != store.TicketOpen || len(ticket.Messages) != 1 {
		t.Fatalf("ticket = %+v", ticket)
	}

	// 管理员回复后状态变为已回复。
	replied, err := svc.ReplyTicket(ctx, admin, ticket.ID, "请重试")
	if err != nil {
		t.Fatalf("ReplyTicket: %v", err)
	}
	if replied.Status != store.TicketAnswered || len(replied.Messages) != 2 {
		t.Fatalf("replied = %+v", replied)
	}
	if replied.Messages[1].AuthorRole != "admin" {
		t.Fatalf("reply role = %q, want admin", replied.Messages[1].AuthorRole)
	}

	// 其他用户不能查看。
	other := &auth.Principal{UserID: "u2", Role: auth.RoleUser}
	if _, err := svc.GetTicket(ctx, other, ticket.ID); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("other user err = %v, want ErrForbidden", err)
	}

	closed, err := svc.SetTicketStatus(ctx, ticket.ID, store.TicketClosed)
	if err != nil || closed.Status != store.TicketClosed {
		t.Fatalf("SetTicketStatus = %+v, err = %v", closed, err)
	}

	if _, err := svc.CreateTicket(ctx, user, "", "x", "b", ""); !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("empty subject err = %v, want ErrInvalidInput", err)
	}
}

// stubGateway 是一个仅用于测试的支付渠道。
type stubGateway struct{ name string }

func (g stubGateway) Name() string { return g.name }

func (g stubGateway) Create(context.Context, payment.Order) (*payment.CreateResult, error) {
	return &payment.CreateResult{TradeNo: "stub-" + g.name}, nil
}

func (g stubGateway) VerifyCallback(context.Context, http.Header, []byte) (*payment.Callback, error) {
	return nil, payment.ErrUnsupported
}

// TestPlanDurationSetsAndRevertsExpiry 验证套餐有效期按天顺延，并在到期后回退
// 到默认角色组与配额。
func TestPlanDurationSetsAndRevertsExpiry(t *testing.T) {
	svc, repo := newBilling(t)
	ctx := context.Background()
	newCustomer(t, repo, "u1")
	policies := service.NewPolicyService(repo, service.PolicyDefaults{
		QuotaBytes:       100 << 20,
		UploadMaxBytes:   5 << 20,
		AllowedMIMETypes: []string{"image/png"},
		Processing: service.ProcessingSettings{
			Enabled:        true,
			MaxWidth:       2048,
			MaxHeight:      2048,
			DefaultQuality: 80,
			AllowedFormats: []string{"png"},
		},
	})
	if err := policies.SeedDefaults(ctx); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	svc.SetPolicyService(policies)

	plan, err := svc.CreatePlan(ctx, service.PlanInput{Name: "月付", PriceCents: 0, QuotaMB: 512, DurationDays: 30, Active: true})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	u1 := &auth.Principal{UserID: "u1", Username: "u1", Role: auth.RoleUser}
	for i := 0; i < 2; i++ {
		if _, err := svc.CreateOrder(ctx, u1, plan.ID, "", ""); err != nil {
			t.Fatalf("CreateOrder #%d: %v", i+1, err)
		}
	}

	account, err := repo.GetAccountByID(ctx, store.RoleCustomer, "u1")
	if err != nil {
		t.Fatalf("GetAccountByID: %v", err)
	}
	if account.PlanExpiresAt == nil {
		t.Fatal("plan expiry was not set")
	}
	// 两次购买各 30 天，应从现在起顺延约 60 天。
	if d := time.Until(*account.PlanExpiresAt); d < 59*24*time.Hour || d > 61*24*time.Hour {
		t.Fatalf("expiry in %v, want ~60 days", d)
	}

	// 到期后回退到默认角色组与配额。
	handled, err := svc.ExpirePlans(ctx, time.Now().AddDate(0, 0, 61))
	if err != nil {
		t.Fatalf("ExpirePlans: %v", err)
	}
	if handled != 1 {
		t.Fatalf("handled = %d, want 1", handled)
	}
	reverted, err := repo.GetAccountByID(ctx, store.RoleCustomer, "u1")
	if err != nil {
		t.Fatalf("GetAccountByID after expiry: %v", err)
	}
	if reverted.PlanExpiresAt != nil {
		t.Fatalf("plan expiry not cleared: %v", reverted.PlanExpiresAt)
	}
	if reverted.QuotaBytes != 100<<20 {
		t.Fatalf("quota = %d, want default 100MiB", reverted.QuotaBytes)
	}
}

// TestPayOrderProviderRestrictions 验证普通用户不能自助完成非 mock 订单，
// 从而避免绕过真实支付；管理员仍可核销 manual 订单。
func TestPayOrderProviderRestrictions(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	newCustomer(t, repo, "u1")
	u1 := &auth.Principal{UserID: "u1", Username: "u1", Role: auth.RoleUser}
	admin := &auth.Principal{UserID: "a1", Username: "a1", Role: auth.RoleAdmin}

	svc := service.NewBillingService(repo, "http://example.test",
		[]payment.Gateway{payment.ManualGateway{}, stubGateway{name: "alipay"}}, "manual")
	plan, err := svc.CreatePlan(ctx, service.PlanInput{Name: "付费套餐", PriceCents: 1000, QuotaMB: 128, Active: true})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	manualOrder, err := svc.CreateOrder(ctx, u1, plan.ID, "", "manual")
	if err != nil {
		t.Fatalf("CreateOrder manual: %v", err)
	}
	if _, err := svc.PayOrder(ctx, u1, manualOrder.ID); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("self-complete manual err = %v, want ErrForbidden", err)
	}
	if _, err := svc.PayOrder(ctx, admin, manualOrder.ID); err != nil {
		t.Fatalf("admin confirm manual: %v", err)
	}

	realOrder, err := svc.CreateOrder(ctx, u1, plan.ID, "", "alipay")
	if err != nil {
		t.Fatalf("CreateOrder alipay: %v", err)
	}
	if _, err := svc.PayOrder(ctx, u1, realOrder.ID); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("self-complete alipay err = %v, want ErrForbidden", err)
	}
}

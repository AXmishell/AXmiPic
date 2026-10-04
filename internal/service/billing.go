package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/payment"
	"github.com/AXmishell/axmipic/internal/store"
)

// 计费与工单服务返回的错误。
var (
	// ErrPlanNotFound 表示套餐不存在。
	ErrPlanNotFound = errors.New("service: plan not found")
	// ErrOrderNotFound 表示订单不存在。
	ErrOrderNotFound = errors.New("service: order not found")
	// ErrCouponNotFound 表示优惠券不存在。
	ErrCouponNotFound = errors.New("service: coupon not found")
	// ErrCouponInvalid 表示优惠券不可用（停用、过期或已用尽）。
	ErrCouponInvalid = errors.New("service: coupon is not usable")
	// ErrCouponBelowMinimum 表示未达到优惠券使用门槛。
	ErrCouponBelowMinimum = errors.New("service: order amount is below the coupon minimum")
	// ErrTicketNotFound 表示工单不存在。
	ErrTicketNotFound = errors.New("service: ticket not found")
)

// PlanDTO 是套餐在 API 中的表示形式。
type PlanDTO struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	PriceCents   int64     `json:"price_cents"`
	DurationDays int       `json:"duration_days"`
	QuotaMB      int64     `json:"quota_mb"`
	RoleGroupID  string    `json:"role_group_id,omitempty"`
	Active       bool      `json:"active"`
	SortOrder    int       `json:"sort_order"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// PlanInput 是创建或更新套餐的输入。
type PlanInput struct {
	Name         string
	Description  string
	PriceCents   int64
	DurationDays int
	QuotaMB      int64
	RoleGroupID  string
	Active       bool
	SortOrder    int
}

// OrderDTO 是订单在 API 中的表示形式。
type OrderDTO struct {
	ID            string     `json:"id"`
	UserID        string     `json:"user_id"`
	PlanID        string     `json:"plan_id"`
	PlanName      string     `json:"plan_name"`
	AmountCents   int64      `json:"amount_cents"`
	DiscountCents int64      `json:"discount_cents"`
	CouponCode    string     `json:"coupon_code,omitempty"`
	Status        string     `json:"status"`
	Provider      string     `json:"provider"`
	TradeNo       string     `json:"trade_no,omitempty"`
	PayURL        string     `json:"pay_url,omitempty"`
	PaidAt        *time.Time `json:"paid_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// CouponDTO 是优惠券在 API 中的表示形式。
type CouponDTO struct {
	ID             string     `json:"id"`
	Code           string     `json:"code"`
	Type           string     `json:"type"`
	Value          int64      `json:"value"`
	MinAmountCents int64      `json:"min_amount_cents"`
	MaxUses        int64      `json:"max_uses"`
	Used           int64      `json:"used"`
	PerUserLimit   int64      `json:"per_user_limit"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	Active         bool       `json:"active"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// CouponInput 是创建或更新优惠券的输入。
type CouponInput struct {
	Code           string
	Type           string
	Value          int64
	MinAmountCents int64
	MaxUses        int64
	PerUserLimit   int64
	ExpiresAt      *time.Time
	Active         bool
}

// TicketDTO 是工单在 API 中的表示形式。
type TicketDTO struct {
	ID        string             `json:"id"`
	UserID    string             `json:"user_id"`
	Username  string             `json:"username,omitempty"`
	Subject   string             `json:"subject"`
	Category  string             `json:"category"`
	Status    string             `json:"status"`
	Priority  string             `json:"priority"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
	Messages  []TicketMessageDTO `json:"messages,omitempty"`
}

// TicketMessageDTO 是工单消息在 API 中的表示形式。
type TicketMessageDTO struct {
	ID         string    `json:"id"`
	AuthorID   string    `json:"author_id"`
	AuthorRole string    `json:"author_role"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}

// BillingService 管理套餐、订单、优惠券与支付渠道。
type BillingService struct {
	repo     *store.Repository
	gateways map[string]payment.Gateway
	// defaultGateway 为未指定渠道时使用；可为空。
	defaultGateway string
	baseURL        string
}

// NewBillingService 构造一个 BillingService。
func NewBillingService(repo *store.Repository, baseURL string, gateways []payment.Gateway, defaultGateway string) *BillingService {
	byName := make(map[string]payment.Gateway, len(gateways))
	for _, g := range gateways {
		if g != nil {
			byName[g.Name()] = g
		}
	}
	if defaultGateway == "" {
		defaultGateway = "manual"
	}
	return &BillingService{repo: repo, gateways: byName, defaultGateway: defaultGateway, baseURL: strings.TrimRight(baseURL, "/")}
}

// Gateways 返回已注册的支付渠道名称。
func (s *BillingService) Gateways() []string {
	names := make([]string, 0, len(s.gateways))
	for name := range s.gateways {
		names = append(names, name)
	}
	return names
}

// Gateway 返回指定名称的支付渠道。
func (s *BillingService) Gateway(name string) (payment.Gateway, bool) {
	g, ok := s.gateways[name]
	return g, ok
}

// HandleCallback 校验某个渠道的支付回调，并在成功时确认订单。它返回处理是否
// 成功，供回调处理器决定响应内容。
func (s *BillingService) HandleCallback(ctx context.Context, provider string, raw []byte) (*OrderDTO, error) {
	gateway, ok := s.gateways[provider]
	if !ok {
		return nil, fmt.Errorf("%w: unknown payment provider %q", ErrInvalidInput, provider)
	}
	callback, err := gateway.VerifyCallback(ctx, raw)
	if err != nil {
		return nil, fmt.Errorf("handle callback: %w", err)
	}
	if callback.OrderID == "" {
		return nil, fmt.Errorf("%w: callback missing order id", ErrInvalidInput)
	}
	if !callback.Success {
		return nil, fmt.Errorf("%w: callback indicates an unsuccessful payment", ErrInvalidInput)
	}
	return s.ConfirmOrder(ctx, callback.OrderID, provider, callback.TradeNo)
}

// ---- 套餐 ----

// ListPlans 返回套餐；activeOnly 为真时仅返回启用的。
func (s *BillingService) ListPlans(ctx context.Context, activeOnly bool) ([]PlanDTO, error) {
	plans, err := s.repo.ListPlans(ctx, activeOnly)
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	dtos := make([]PlanDTO, 0, len(plans))
	for i := range plans {
		dtos = append(dtos, *planToDTO(&plans[i]))
	}
	return dtos, nil
}

// CreatePlan 创建套餐。
func (s *BillingService) CreatePlan(ctx context.Context, in PlanInput) (*PlanDTO, error) {
	if err := validatePlanInput(in); err != nil {
		return nil, err
	}
	plan := &store.Plan{
		ID:           uuid.NewString(),
		Name:         strings.TrimSpace(in.Name),
		Description:  strings.TrimSpace(in.Description),
		PriceCents:   in.PriceCents,
		DurationDays: in.DurationDays,
		QuotaMB:      in.QuotaMB,
		RoleGroupID:  optionalID(in.RoleGroupID),
		Active:       in.Active,
		SortOrder:    in.SortOrder,
	}
	if err := s.repo.CreatePlan(ctx, plan); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, fmt.Errorf("%w: plan name already exists", ErrInvalidInput)
		}
		return nil, fmt.Errorf("create plan: %w", err)
	}
	return planToDTO(plan), nil
}

// UpdatePlan 修改套餐。
func (s *BillingService) UpdatePlan(ctx context.Context, id string, in PlanInput) (*PlanDTO, error) {
	if err := validatePlanInput(in); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	description := strings.TrimSpace(in.Description)
	roleGroupID := in.RoleGroupID
	updated, err := s.repo.UpdatePlan(ctx, id, store.PlanUpdate{
		Name:         &name,
		Description:  &description,
		PriceCents:   &in.PriceCents,
		DurationDays: &in.DurationDays,
		QuotaMB:      &in.QuotaMB,
		RoleGroupID:  &roleGroupID,
		Active:       &in.Active,
		SortOrder:    &in.SortOrder,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrPlanNotFound
		}
		return nil, fmt.Errorf("update plan: %w", err)
	}
	return planToDTO(updated), nil
}

// DeletePlan 删除套餐。
func (s *BillingService) DeletePlan(ctx context.Context, id string) error {
	if err := s.repo.DeletePlan(ctx, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrPlanNotFound
		}
		return fmt.Errorf("delete plan: %w", err)
	}
	return nil
}

// ---- 优惠券 ----

// ListCoupons 返回全部优惠券。
func (s *BillingService) ListCoupons(ctx context.Context) ([]CouponDTO, error) {
	coupons, err := s.repo.ListCoupons(ctx)
	if err != nil {
		return nil, fmt.Errorf("list coupons: %w", err)
	}
	dtos := make([]CouponDTO, 0, len(coupons))
	for i := range coupons {
		dtos = append(dtos, *couponToDTO(&coupons[i]))
	}
	return dtos, nil
}

// CreateCoupon 创建优惠券。
func (s *BillingService) CreateCoupon(ctx context.Context, in CouponInput) (*CouponDTO, error) {
	code, couponType, err := validateCouponInput(in)
	if err != nil {
		return nil, err
	}
	coupon := &store.Coupon{
		ID:             uuid.NewString(),
		Code:           code,
		Type:           couponType,
		Value:          in.Value,
		MinAmountCents: in.MinAmountCents,
		MaxUses:        in.MaxUses,
		PerUserLimit:   in.PerUserLimit,
		ExpiresAt:      in.ExpiresAt,
		Active:         in.Active,
	}
	if err := s.repo.CreateCoupon(ctx, coupon); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, fmt.Errorf("%w: coupon code already exists", ErrInvalidInput)
		}
		return nil, fmt.Errorf("create coupon: %w", err)
	}
	return couponToDTO(coupon), nil
}

// UpdateCoupon 修改优惠券。
func (s *BillingService) UpdateCoupon(ctx context.Context, id string, in CouponInput) (*CouponDTO, error) {
	code, couponType, err := validateCouponInput(in)
	if err != nil {
		return nil, err
	}
	update := store.CouponUpdate{
		Code:           &code,
		Type:           &couponType,
		Value:          &in.Value,
		MinAmountCents: &in.MinAmountCents,
		MaxUses:        &in.MaxUses,
		PerUserLimit:   &in.PerUserLimit,
		Active:         &in.Active,
	}
	if in.ExpiresAt != nil {
		update.ExpiresAt = in.ExpiresAt
	} else {
		update.ClearExpiry = true
	}
	updated, err := s.repo.UpdateCoupon(ctx, id, update)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrCouponNotFound
		}
		return nil, fmt.Errorf("update coupon: %w", err)
	}
	return couponToDTO(updated), nil
}

// DeleteCoupon 删除优惠券。
func (s *BillingService) DeleteCoupon(ctx context.Context, id string) error {
	if err := s.repo.DeleteCoupon(ctx, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrCouponNotFound
		}
		return fmt.Errorf("delete coupon: %w", err)
	}
	return nil
}

// ValidateCoupon 预览某张优惠券对给定金额的折扣，返回折扣金额（分）。
func (s *BillingService) ValidateCoupon(ctx context.Context, principal *auth.Principal, code string, amountCents int64) (*CouponDTO, int64, error) {
	coupon, err := s.repo.GetCouponByCode(ctx, strings.ToUpper(strings.TrimSpace(code)))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, 0, ErrCouponNotFound
		}
		return nil, 0, fmt.Errorf("validate coupon: %w", err)
	}
	if err := s.couponUsable(ctx, coupon, principal.UserID, amountCents); err != nil {
		return nil, 0, err
	}
	return couponToDTO(coupon), couponDiscount(coupon, amountCents), nil
}

// couponUsable 校验优惠券当前是否可用于某个用户与金额。
func (s *BillingService) couponUsable(ctx context.Context, coupon *store.Coupon, userID string, amountCents int64) error {
	if !coupon.Active {
		return fmt.Errorf("%w: coupon is inactive", ErrCouponInvalid)
	}
	if coupon.ExpiresAt != nil && time.Now().After(*coupon.ExpiresAt) {
		return fmt.Errorf("%w: coupon has expired", ErrCouponInvalid)
	}
	if coupon.MaxUses > 0 && coupon.Used >= coupon.MaxUses {
		return fmt.Errorf("%w: coupon is exhausted", ErrCouponInvalid)
	}
	if coupon.PerUserLimit > 0 {
		count, err := s.repo.CountUserRedemptions(ctx, coupon.ID, userID)
		if err != nil {
			return fmt.Errorf("validate coupon: %w", err)
		}
		if count >= coupon.PerUserLimit {
			return fmt.Errorf("%w: you have reached the usage limit", ErrCouponInvalid)
		}
	}
	if amountCents < coupon.MinAmountCents {
		return ErrCouponBelowMinimum
	}
	return nil
}

// ---- 订单与支付 ----

// CreateOrder 为某个套餐创建订单，可选地应用优惠券。渠道为空时使用默认渠道。
func (s *BillingService) CreateOrder(ctx context.Context, principal *auth.Principal, planID, couponCode, provider string) (*OrderDTO, error) {
	plan, err := s.repo.GetPlanByID(ctx, planID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrPlanNotFound
		}
		return nil, fmt.Errorf("create order: %w", err)
	}
	if !plan.Active {
		return nil, fmt.Errorf("%w: plan is not available", ErrInvalidInput)
	}
	if provider == "" {
		provider = s.defaultGateway
	}
	gateway, ok := s.gateways[provider]
	if !ok {
		return nil, fmt.Errorf("%w: unknown payment provider %q", ErrInvalidInput, provider)
	}

	discount := int64(0)
	var couponID *string
	if code := strings.TrimSpace(couponCode); code != "" {
		coupon, err := s.repo.GetCouponByCode(ctx, strings.ToUpper(code))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, ErrCouponNotFound
			}
			return nil, fmt.Errorf("create order: %w", err)
		}
		if err := s.couponUsable(ctx, coupon, principal.UserID, plan.PriceCents); err != nil {
			return nil, err
		}
		discount = couponDiscount(coupon, plan.PriceCents)
		couponID = &coupon.ID
	}
	amount := plan.PriceCents - discount
	if amount < 0 {
		amount = 0
	}

	order := &store.Order{
		ID:            uuid.NewString(),
		UserID:        principal.UserID,
		PlanID:        plan.ID,
		PlanName:      plan.Name,
		AmountCents:   amount,
		CouponID:      couponID,
		DiscountCents: discount,
		Status:        store.OrderPending,
		Provider:      provider,
	}
	if err := s.repo.CreateOrder(ctx, order); err != nil {
		return nil, fmt.Errorf("create order: %w", err)
	}
	// 立即消费优惠券，避免并发下单绕过使用次数限制。
	if couponID != nil {
		if err := s.repo.RedeemCoupon(ctx, *couponID, principal.UserID, order.ID); err != nil {
			_ = s.repo.UpdateOrderStatus(ctx, order.ID, store.OrderCancelled)
			return nil, mapCouponError(err)
		}
	}

	result, err := gateway.Create(ctx, payment.Order{
		ID:          order.ID,
		Subject:     "AXmiPic 套餐：" + plan.Name,
		AmountCents: amount,
		NotifyURL:   s.baseURL + "/api/v1/payments/" + provider + "/callback",
		ReturnURL:   s.baseURL + "/orders",
	})
	if err != nil {
		return nil, fmt.Errorf("create order: %w", err)
	}
	dto := orderToDTO(order)
	dto.TradeNo = result.TradeNo
	dto.PayURL = result.PayURL
	// 免费订单直接完成。
	if amount == 0 {
		if err := s.applyOrder(ctx, order.ID); err != nil {
			return nil, err
		}
		updated, _ := s.repo.GetOrderByID(ctx, order.ID)
		if updated != nil {
			paid := orderToDTO(updated)
			paid.PayURL = result.PayURL
			return paid, nil
		}
	}
	return dto, nil
}

// ListOrders 返回某个主体可见的订单：管理员可见全部，其余仅自己的。
func (s *BillingService) ListOrders(ctx context.Context, principal *auth.Principal) ([]OrderDTO, error) {
	var orders []store.Order
	var err error
	if principal.IsAdmin() {
		orders, err = s.repo.ListOrders(ctx)
	} else {
		orders, err = s.repo.ListOrdersByUser(ctx, principal.UserID)
	}
	if err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}
	dtos := make([]OrderDTO, 0, len(orders))
	for i := range orders {
		dtos = append(dtos, *orderToDTO(&orders[i]))
	}
	return dtos, nil
}

// PayOrder 模拟/发起一笔订单的支付完成（用于手动与模拟渠道）。
func (s *BillingService) PayOrder(ctx context.Context, principal *auth.Principal, orderID string) (*OrderDTO, error) {
	order, err := s.repo.GetOrderByID(ctx, orderID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, fmt.Errorf("pay order: %w", err)
	}
	if !principal.IsAdmin() && order.UserID != principal.UserID {
		return nil, ErrForbidden
	}
	if order.Status != store.OrderPending {
		return nil, fmt.Errorf("%w: order is not pending", ErrInvalidInput)
	}
	if err := s.applyOrder(ctx, order.ID); err != nil {
		return nil, err
	}
	updated, err := s.repo.GetOrderByID(ctx, order.ID)
	if err != nil {
		return nil, fmt.Errorf("pay order: %w", err)
	}
	return orderToDTO(updated), nil
}

// ConfirmOrder 供支付回调使用：按订单 id 标记为已支付并应用套餐权益。
func (s *BillingService) ConfirmOrder(ctx context.Context, orderID, provider, tradeNo string) (*OrderDTO, error) {
	order, err := s.repo.GetOrderByID(ctx, orderID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, fmt.Errorf("confirm order: %w", err)
	}
	if order.Status == store.OrderPaid {
		return orderToDTO(order), nil
	}
	if err := s.applyOrder(ctx, order.ID); err != nil {
		return nil, err
	}
	updated, _ := s.repo.GetOrderByID(ctx, order.ID)
	return orderToDTO(updated), nil
}

// applyOrder 把订单标记为已支付，并将套餐权益应用到用户。
func (s *BillingService) applyOrder(ctx context.Context, orderID string) error {
	order, err := s.repo.GetOrderByID(ctx, orderID)
	if err != nil {
		return fmt.Errorf("apply order: %w", err)
	}
	paid, err := s.repo.MarkOrderPaid(ctx, orderID, order.Provider, order.TradeNo, time.Now())
	if err != nil {
		return fmt.Errorf("apply order: %w", err)
	}
	if !paid {
		// 已被处理，幂等返回。
		return nil
	}
	if err := s.grantPlan(ctx, order); err != nil {
		return err
	}
	return nil
}

// grantPlan 依据套餐调整用户的角色组与配额。
func (s *BillingService) grantPlan(ctx context.Context, order *store.Order) error {
	plan, err := s.repo.GetPlanByID(ctx, order.PlanID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// 套餐已被删除：订单仍然有效，仅跳过权益。
			return nil
		}
		return fmt.Errorf("grant plan: %w", err)
	}
	update := store.UserUpdate{}
	if plan.QuotaMB > 0 {
		quota := plan.QuotaMB << 20
		update.QuotaBytes = &quota
	}
	if plan.RoleGroupID != nil && *plan.RoleGroupID != "" {
		groupID := *plan.RoleGroupID
		update.RoleGroupID = &groupID
	}
	if update.QuotaBytes == nil && update.RoleGroupID == nil {
		return nil
	}
	if _, err := s.repo.UpdateCustomer(ctx, order.UserID, update); err != nil {
		return fmt.Errorf("grant plan: %w", err)
	}
	return nil
}

// ---- 工单 ----

// CreateTicket 创建一个工单，并把首条消息写入工单。
func (s *BillingService) CreateTicket(ctx context.Context, principal *auth.Principal, subject, category, body, priority string) (*TicketDTO, error) {
	subject = strings.TrimSpace(subject)
	if subject == "" || len([]rune(subject)) > 200 {
		return nil, fmt.Errorf("%w: subject must be 1-200 characters", ErrInvalidInput)
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, fmt.Errorf("%w: message must not be empty", ErrInvalidInput)
	}
	priority = normalizePriority(priority)
	ticket := &store.Ticket{
		ID:       uuid.NewString(),
		UserID:   principal.UserID,
		Subject:  subject,
		Category: strings.TrimSpace(category),
		Status:   store.TicketOpen,
		Priority: priority,
	}
	if err := s.repo.CreateTicket(ctx, ticket); err != nil {
		return nil, fmt.Errorf("create ticket: %w", err)
	}
	if err := s.repo.CreateTicketMessage(ctx, &store.TicketMessage{
		ID:         uuid.NewString(),
		TicketID:   ticket.ID,
		AuthorID:   principal.UserID,
		AuthorRole: "author",
		Body:       body,
	}); err != nil {
		return nil, fmt.Errorf("create ticket: %w", err)
	}
	return s.ticketDTO(ctx, ticket, true)
}

// ListTickets 返回主体可见的工单：管理员可见全部（可按状态过滤），其余仅自己的。
func (s *BillingService) ListTickets(ctx context.Context, principal *auth.Principal, status string) ([]TicketDTO, error) {
	var tickets []store.Ticket
	var err error
	if principal.IsAdmin() {
		tickets, err = s.repo.ListTickets(ctx, status)
	} else {
		tickets, err = s.repo.ListTicketsByUser(ctx, principal.UserID)
	}
	if err != nil {
		return nil, fmt.Errorf("list tickets: %w", err)
	}
	dtos := make([]TicketDTO, 0, len(tickets))
	names := s.usernamesFor(ctx, tickets)
	for i := range tickets {
		dto := ticketToDTO(&tickets[i])
		dto.Username = names[tickets[i].UserID]
		dtos = append(dtos, *dto)
	}
	return dtos, nil
}

// GetTicket 返回单个工单及其消息，并强制校验访问权限。
func (s *BillingService) GetTicket(ctx context.Context, principal *auth.Principal, id string) (*TicketDTO, error) {
	ticket, err := s.repo.GetTicketByID(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrTicketNotFound
		}
		return nil, fmt.Errorf("get ticket: %w", err)
	}
	if !principal.IsAdmin() && ticket.UserID != principal.UserID {
		return nil, ErrForbidden
	}
	return s.ticketDTO(ctx, ticket, true)
}

// ReplyTicket 在工单中追加一条消息。管理员回复会把状态置为已回复；用户回复
// 会把状态重置为待处理。
func (s *BillingService) ReplyTicket(ctx context.Context, principal *auth.Principal, id, body string) (*TicketDTO, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, fmt.Errorf("%w: message must not be empty", ErrInvalidInput)
	}
	ticket, err := s.repo.GetTicketByID(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrTicketNotFound
		}
		return nil, fmt.Errorf("reply ticket: %w", err)
	}
	if !principal.IsAdmin() && ticket.UserID != principal.UserID {
		return nil, ErrForbidden
	}
	role := "author"
	status := store.TicketOpen
	if principal.IsAdmin() {
		role = "admin"
		status = store.TicketAnswered
	}
	if err := s.repo.CreateTicketMessage(ctx, &store.TicketMessage{
		ID:         uuid.NewString(),
		TicketID:   ticket.ID,
		AuthorID:   principal.UserID,
		AuthorRole: role,
		Body:       body,
	}); err != nil {
		return nil, fmt.Errorf("reply ticket: %w", err)
	}
	if _, err := s.repo.UpdateTicketStatus(ctx, id, status); err != nil {
		return nil, fmt.Errorf("reply ticket: %w", err)
	}
	updated, _ := s.repo.GetTicketByID(ctx, id)
	return s.ticketDTO(ctx, updated, true)
}

// SetTicketStatus 更新工单状态（仅管理员）。
func (s *BillingService) SetTicketStatus(ctx context.Context, id, status string) (*TicketDTO, error) {
	switch status {
	case store.TicketOpen, store.TicketAnswered, store.TicketClosed:
	default:
		return nil, fmt.Errorf("%w: unknown ticket status %q", ErrInvalidInput, status)
	}
	updated, err := s.repo.UpdateTicketStatus(ctx, id, status)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrTicketNotFound
		}
		return nil, fmt.Errorf("set ticket status: %w", err)
	}
	return s.ticketDTO(ctx, updated, false)
}

// ticketDTO 组装工单的对外表示；withMessages 为真时带上消息列表。
func (s *BillingService) ticketDTO(ctx context.Context, ticket *store.Ticket, withMessages bool) (*TicketDTO, error) {
	dto := ticketToDTO(ticket)
	if names, err := s.repo.UsernamesByIDs(ctx, []string{ticket.UserID}); err == nil {
		dto.Username = names[ticket.UserID]
	}
	if withMessages {
		messages, err := s.repo.ListTicketMessages(ctx, ticket.ID)
		if err != nil {
			return nil, fmt.Errorf("ticket messages: %w", err)
		}
		dtos := make([]TicketMessageDTO, 0, len(messages))
		for i := range messages {
			dtos = append(dtos, TicketMessageDTO{
				ID:         messages[i].ID,
				AuthorID:   messages[i].AuthorID,
				AuthorRole: messages[i].AuthorRole,
				Body:       messages[i].Body,
				CreatedAt:  messages[i].CreatedAt,
			})
		}
		dto.Messages = dtos
	}
	return dto, nil
}

// usernamesFor 批量解析工单所有者的用户名。
func (s *BillingService) usernamesFor(ctx context.Context, tickets []store.Ticket) map[string]string {
	ids := make([]string, 0, len(tickets))
	seen := make(map[string]struct{}, len(tickets))
	for i := range tickets {
		if _, ok := seen[tickets[i].UserID]; !ok {
			seen[tickets[i].UserID] = struct{}{}
			ids = append(ids, tickets[i].UserID)
		}
	}
	names, err := s.repo.UsernamesByIDs(ctx, ids)
	if err != nil {
		return map[string]string{}
	}
	return names
}

func couponDiscount(coupon *store.Coupon, amountCents int64) int64 {
	switch coupon.Type {
	case store.CouponPercent:
		discount := amountCents * coupon.Value / 100
		if discount > amountCents {
			discount = amountCents
		}
		return discount
	default:
		if coupon.Value > amountCents {
			return amountCents
		}
		return coupon.Value
	}
}

func validatePlanInput(in PlanInput) error {
	name := strings.TrimSpace(in.Name)
	if name == "" || len([]rune(name)) > 64 {
		return fmt.Errorf("%w: plan name must be 1-64 characters", ErrInvalidInput)
	}
	if in.PriceCents < 0 {
		return fmt.Errorf("%w: price must not be negative", ErrInvalidInput)
	}
	if in.DurationDays < 0 {
		return fmt.Errorf("%w: duration must not be negative", ErrInvalidInput)
	}
	if in.QuotaMB < 0 {
		return fmt.Errorf("%w: quota must not be negative", ErrInvalidInput)
	}
	return nil
}

func validateCouponInput(in CouponInput) (string, string, error) {
	code := strings.ToUpper(strings.TrimSpace(in.Code))
	if code == "" || len(code) > 64 {
		return "", "", fmt.Errorf("%w: coupon code must be 1-64 characters", ErrInvalidInput)
	}
	couponType := strings.TrimSpace(in.Type)
	switch couponType {
	case store.CouponFixed:
		if in.Value <= 0 {
			return "", "", fmt.Errorf("%w: fixed discount must be positive", ErrInvalidInput)
		}
	case store.CouponPercent:
		if in.Value <= 0 || in.Value > 100 {
			return "", "", fmt.Errorf("%w: percent discount must be between 1 and 100", ErrInvalidInput)
		}
	default:
		return "", "", fmt.Errorf("%w: unknown coupon type %q", ErrInvalidInput, in.Type)
	}
	if in.MinAmountCents < 0 || in.MaxUses < 0 || in.PerUserLimit < 0 {
		return "", "", fmt.Errorf("%w: coupon limits must not be negative", ErrInvalidInput)
	}
	return code, couponType, nil
}

func normalizePriority(priority string) string {
	switch strings.TrimSpace(priority) {
	case "low":
		return "low"
	case "high":
		return "high"
	default:
		return "normal"
	}
}

// optionalID 把空字符串转换为 nil 指针。
func optionalID(id string) *string {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func mapCouponError(err error) error {
	switch {
	case errors.Is(err, store.ErrCouponInactive),
		errors.Is(err, store.ErrCouponExpired),
		errors.Is(err, store.ErrCouponExhausted),
		errors.Is(err, store.ErrCouponLimitReached):
		return fmt.Errorf("%w: %v", ErrCouponInvalid, err)
	case errors.Is(err, store.ErrNotFound):
		return ErrCouponNotFound
	default:
		return fmt.Errorf("coupon: %w", err)
	}
}

func planToDTO(plan *store.Plan) *PlanDTO {
	roleGroupID := ""
	if plan.RoleGroupID != nil {
		roleGroupID = *plan.RoleGroupID
	}
	return &PlanDTO{
		ID:           plan.ID,
		Name:         plan.Name,
		Description:  plan.Description,
		PriceCents:   plan.PriceCents,
		DurationDays: plan.DurationDays,
		QuotaMB:      plan.QuotaMB,
		RoleGroupID:  roleGroupID,
		Active:       plan.Active,
		SortOrder:    plan.SortOrder,
		CreatedAt:    plan.CreatedAt,
		UpdatedAt:    plan.UpdatedAt,
	}
}

func orderToDTO(order *store.Order) *OrderDTO {
	dto := &OrderDTO{
		ID:            order.ID,
		UserID:        order.UserID,
		PlanID:        order.PlanID,
		PlanName:      order.PlanName,
		AmountCents:   order.AmountCents,
		DiscountCents: order.DiscountCents,
		Status:        order.Status,
		Provider:      order.Provider,
		TradeNo:       order.TradeNo,
		PaidAt:        order.PaidAt,
		CreatedAt:     order.CreatedAt,
		UpdatedAt:     order.UpdatedAt,
	}
	return dto
}

func couponToDTO(coupon *store.Coupon) *CouponDTO {
	return &CouponDTO{
		ID:             coupon.ID,
		Code:           coupon.Code,
		Type:           coupon.Type,
		Value:          coupon.Value,
		MinAmountCents: coupon.MinAmountCents,
		MaxUses:        coupon.MaxUses,
		Used:           coupon.Used,
		PerUserLimit:   coupon.PerUserLimit,
		ExpiresAt:      coupon.ExpiresAt,
		Active:         coupon.Active,
		CreatedAt:      coupon.CreatedAt,
		UpdatedAt:      coupon.UpdatedAt,
	}
}

func ticketToDTO(ticket *store.Ticket) *TicketDTO {
	return &TicketDTO{
		ID:        ticket.ID,
		UserID:    ticket.UserID,
		Subject:   ticket.Subject,
		Category:  ticket.Category,
		Status:    ticket.Status,
		Priority:  ticket.Priority,
		CreatedAt: ticket.CreatedAt,
		UpdatedAt: ticket.UpdatedAt,
	}
}

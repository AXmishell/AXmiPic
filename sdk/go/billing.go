package axmipic

import "context"

// PlanInput 是创建或更新套餐的输入。
type PlanInput struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	PriceCents   int64  `json:"price_cents"`
	DurationDays int    `json:"duration_days"`
	QuotaMB      int64  `json:"quota_mb"`
	RoleGroupID  string `json:"role_group_id,omitempty"`
	Active       bool   `json:"active"`
	SortOrder    int    `json:"sort_order"`
}

// CouponInput 是创建或更新优惠券的输入。
type CouponInput struct {
	Code           string  `json:"code"`
	Type           string  `json:"type"`
	Value          int64   `json:"value"`
	MinAmountCents int64   `json:"min_amount_cents"`
	MaxUses        int64   `json:"max_uses"`
	PerUserLimit   int64   `json:"per_user_limit"`
	ExpiresAt      *string `json:"expires_at,omitempty"`
	Active         bool    `json:"active"`
}

// OrderInput 是下单请求。
type OrderInput struct {
	PlanID     string `json:"plan_id"`
	CouponCode string `json:"coupon_code,omitempty"`
	Provider   string `json:"provider,omitempty"`
}

// TicketInput 是创建工单的输入。
type TicketInput struct {
	Subject  string `json:"subject"`
	Category string `json:"category,omitempty"`
	Body     string `json:"body"`
	Priority string `json:"priority,omitempty"`
}

// CouponValidation 是优惠券校验结果。
type CouponValidation struct {
	Coupon        Coupon `json:"coupon"`
	DiscountCents int64  `json:"discount_cents"`
}

// ---- 套餐（公开可读） ----

// ListPlans 返回启用的套餐（无需登录）。
func (c *Client) ListPlans(ctx context.Context) ([]Plan, error) {
	var out []Plan
	if err := c.get(ctx, "/api/v1/plans", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListPaymentGateways 返回已启用的支付渠道。
func (c *Client) ListPaymentGateways(ctx context.Context) ([]string, error) {
	var out []string
	if err := c.get(ctx, "/api/v1/payment-gateways", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ---- 优惠券与订单 ----

// ValidateCoupon 校验优惠券并返回折扣。
func (c *Client) ValidateCoupon(ctx context.Context, code string, amountCents int64) (*CouponValidation, error) {
	var out CouponValidation
	in := map[string]any{"code": code, "amount_cents": amountCents}
	if err := c.postJSON(ctx, "/api/v1/coupons/validate", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateOrder 下单。
func (c *Client) CreateOrder(ctx context.Context, in OrderInput) (*Order, error) {
	var out Order
	if err := c.postJSON(ctx, "/api/v1/orders", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListOrders 返回当前账号可见的订单。
func (c *Client) ListOrders(ctx context.Context) ([]Order, error) {
	var out []Order
	if err := c.get(ctx, "/api/v1/orders", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// PayOrder 完成一笔手动/模拟订单的支付。
func (c *Client) PayOrder(ctx context.Context, id string) (*Order, error) {
	var out Order
	if err := c.postJSON(ctx, "/api/v1/orders/"+id+"/pay", map[string]any{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ---- 工单 ----

// CreateTicket 创建一个工单。
func (c *Client) CreateTicket(ctx context.Context, in TicketInput) (*Ticket, error) {
	var out Ticket
	if err := c.postJSON(ctx, "/api/v1/tickets", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListTickets 返回当前账号可见的工单。
func (c *Client) ListTickets(ctx context.Context, status string) ([]Ticket, error) {
	var out []Ticket
	path := encodeQuery("/api/v1/tickets", map[string]string{"status": status})
	if err := c.get(ctx, path, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetTicket 返回工单详情（含消息）。
func (c *Client) GetTicket(ctx context.Context, id string) (*Ticket, error) {
	var out Ticket
	if err := c.get(ctx, "/api/v1/tickets/"+id, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ReplyTicket 回复工单。
func (c *Client) ReplyTicket(ctx context.Context, id, body string) (*Ticket, error) {
	var out Ticket
	if err := c.postJSON(ctx, "/api/v1/tickets/"+id+"/reply", map[string]string{"body": body}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ---- 管理：套餐 / 优惠券 / 工单 ----

// AdminListPlans 返回全部套餐。
func (c *Client) AdminListPlans(ctx context.Context) ([]Plan, error) {
	var out []Plan
	if err := c.get(ctx, "/api/v1/admin/plans", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// AdminCreatePlan 创建套餐。
func (c *Client) AdminCreatePlan(ctx context.Context, in PlanInput) (*Plan, error) {
	var out Plan
	if err := c.postJSON(ctx, "/api/v1/admin/plans", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminUpdatePlan 修改套餐。
func (c *Client) AdminUpdatePlan(ctx context.Context, id string, in PlanInput) (*Plan, error) {
	var out Plan
	if err := c.putJSON(ctx, "/api/v1/admin/plans/"+id, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminDeletePlan 删除套餐。
func (c *Client) AdminDeletePlan(ctx context.Context, id string) error {
	return c.delete(ctx, "/api/v1/admin/plans/"+id, nil)
}

// AdminListCoupons 返回全部优惠券。
func (c *Client) AdminListCoupons(ctx context.Context) ([]Coupon, error) {
	var out []Coupon
	if err := c.get(ctx, "/api/v1/admin/coupons", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// AdminCreateCoupon 创建优惠券。
func (c *Client) AdminCreateCoupon(ctx context.Context, in CouponInput) (*Coupon, error) {
	var out Coupon
	if err := c.postJSON(ctx, "/api/v1/admin/coupons", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminUpdateCoupon 修改优惠券。
func (c *Client) AdminUpdateCoupon(ctx context.Context, id string, in CouponInput) (*Coupon, error) {
	var out Coupon
	if err := c.putJSON(ctx, "/api/v1/admin/coupons/"+id, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminDeleteCoupon 删除优惠券。
func (c *Client) AdminDeleteCoupon(ctx context.Context, id string) error {
	return c.delete(ctx, "/api/v1/admin/coupons/"+id, nil)
}

// AdminSetTicketStatus 更新工单状态。
func (c *Client) AdminSetTicketStatus(ctx context.Context, id, status string) (*Ticket, error) {
	var out Ticket
	if err := c.patchJSON(ctx, "/api/v1/admin/tickets/"+id, map[string]string{"status": status}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

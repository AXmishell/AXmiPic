package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// newID 返回一个新的 UUID 字符串，供 store 层内部构造记录 id。
func newID() string { return uuid.NewString() }

// ---- 套餐 ----

// CreatePlan 插入一个套餐。
func (r *Repository) CreatePlan(ctx context.Context, plan *Plan) error {
	if err := r.db.WithContext(ctx).Create(plan).Error; err != nil {
		return fmt.Errorf("store: create plan: %w", err)
	}
	return nil
}

// GetPlanByID 返回具有给定 id 的套餐，或 ErrNotFound。
func (r *Repository) GetPlanByID(ctx context.Context, id string) (*Plan, error) {
	var plan Plan
	if err := r.first(ctx, &plan, "id = ?", id); err != nil {
		return nil, fmt.Errorf("store: plan %q: %w", id, err)
	}
	return &plan, nil
}

// ListPlans 返回套餐；activeOnly 为真时仅返回启用的。按 SortOrder 与创建时间排序。
func (r *Repository) ListPlans(ctx context.Context, activeOnly bool) ([]Plan, error) {
	query := r.db.WithContext(ctx).Order("sort_order ASC, created_at ASC")
	if activeOnly {
		query = query.Where("active = ?", true)
	}
	var plans []Plan
	if err := query.Find(&plans).Error; err != nil {
		return nil, fmt.Errorf("store: list plans: %w", err)
	}
	return plans, nil
}

// PlanUpdate 携带可选的套餐字段更改。
type PlanUpdate struct {
	Name         *string
	Description  *string
	PriceCents   *int64
	DurationDays *int
	QuotaMB      *int64
	RoleGroupID  *string
	Active       *bool
	SortOrder    *int
}

// UpdatePlan 应用可选字段更改并返回更新后的记录。
func (r *Repository) UpdatePlan(ctx context.Context, id string, update PlanUpdate) (*Plan, error) {
	fields := map[string]any{}
	if update.Name != nil {
		fields["name"] = *update.Name
	}
	if update.Description != nil {
		fields["description"] = *update.Description
	}
	if update.PriceCents != nil {
		fields["price_cents"] = *update.PriceCents
	}
	if update.DurationDays != nil {
		fields["duration_days"] = *update.DurationDays
	}
	if update.QuotaMB != nil {
		fields["quota_mb"] = *update.QuotaMB
	}
	if update.RoleGroupID != nil {
		if *update.RoleGroupID == "" {
			fields["role_group_id"] = nil
		} else {
			fields["role_group_id"] = *update.RoleGroupID
		}
	}
	if update.Active != nil {
		fields["active"] = *update.Active
	}
	if update.SortOrder != nil {
		fields["sort_order"] = *update.SortOrder
	}
	if len(fields) > 0 {
		if err := r.db.WithContext(ctx).Model(&Plan{}).Where("id = ?", id).Updates(fields).Error; err != nil {
			return nil, fmt.Errorf("store: update plan %q: %w", id, err)
		}
	}
	return r.GetPlanByID(ctx, id)
}

// DeletePlan 删除一个套餐。
func (r *Repository) DeletePlan(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).Delete(&Plan{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("store: delete plan %q: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("store: plan %q: %w", id, ErrNotFound)
	}
	return nil
}

// ---- 订单 ----

// CreateOrder 插入一个订单。
func (r *Repository) CreateOrder(ctx context.Context, order *Order) error {
	if err := r.db.WithContext(ctx).Create(order).Error; err != nil {
		return fmt.Errorf("store: create order: %w", err)
	}
	return nil
}

// GetOrderByID 返回具有给定 id 的订单，或 ErrNotFound。
func (r *Repository) GetOrderByID(ctx context.Context, id string) (*Order, error) {
	var order Order
	if err := r.first(ctx, &order, "id = ?", id); err != nil {
		return nil, fmt.Errorf("store: order %q: %w", id, err)
	}
	return &order, nil
}

// ListOrdersByUser 返回某个用户的订单，最新的在前。
func (r *Repository) ListOrdersByUser(ctx context.Context, userID string) ([]Order, error) {
	var orders []Order
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).
		Order("created_at DESC").Find(&orders).Error; err != nil {
		return nil, fmt.Errorf("store: list orders: %w", err)
	}
	return orders, nil
}

// ListOrders 返回全部订单，最新的在前。
func (r *Repository) ListOrders(ctx context.Context) ([]Order, error) {
	var orders []Order
	if err := r.db.WithContext(ctx).Order("created_at DESC").Find(&orders).Error; err != nil {
		return nil, fmt.Errorf("store: list orders: %w", err)
	}
	return orders, nil
}

// MarkOrderPaid 在订单尚未支付时将其标记为已支付，返回更新是否生效。
func (r *Repository) MarkOrderPaid(ctx context.Context, id, provider, tradeNo string, paidAt time.Time) (bool, error) {
	result := r.db.WithContext(ctx).Model(&Order{}).
		Where("id = ? AND status = ?", id, OrderPending).
		Updates(map[string]any{
			"status":   OrderPaid,
			"provider": provider,
			"trade_no": tradeNo,
			"paid_at":  paidAt,
		})
	if result.Error != nil {
		return false, fmt.Errorf("store: mark order paid %q: %w", id, result.Error)
	}
	return result.RowsAffected == 1, nil
}

// UpdateOrderStatus 更新订单状态（例如取消）。
func (r *Repository) UpdateOrderStatus(ctx context.Context, id, status string) error {
	if err := r.db.WithContext(ctx).Model(&Order{}).Where("id = ?", id).
		Update("status", status).Error; err != nil {
		return fmt.Errorf("store: update order status %q: %w", id, err)
	}
	return nil
}

// ---- 优惠券 ----

// CreateCoupon 插入一张优惠券。
func (r *Repository) CreateCoupon(ctx context.Context, coupon *Coupon) error {
	if err := r.db.WithContext(ctx).Create(coupon).Error; err != nil {
		return fmt.Errorf("store: create coupon: %w", err)
	}
	return nil
}

// GetCouponByID 返回具有给定 id 的优惠券，或 ErrNotFound。
func (r *Repository) GetCouponByID(ctx context.Context, id string) (*Coupon, error) {
	var coupon Coupon
	if err := r.first(ctx, &coupon, "id = ?", id); err != nil {
		return nil, fmt.Errorf("store: coupon %q: %w", id, err)
	}
	return &coupon, nil
}

// GetCouponByCode 返回具有给定代码的优惠券，或 ErrNotFound。
func (r *Repository) GetCouponByCode(ctx context.Context, code string) (*Coupon, error) {
	var coupon Coupon
	if err := r.first(ctx, &coupon, "code = ?", code); err != nil {
		return nil, fmt.Errorf("store: coupon code %q: %w", code, err)
	}
	return &coupon, nil
}

// ListCoupons 返回全部优惠券，最新的在前。
func (r *Repository) ListCoupons(ctx context.Context) ([]Coupon, error) {
	var coupons []Coupon
	if err := r.db.WithContext(ctx).Order("created_at DESC").Find(&coupons).Error; err != nil {
		return nil, fmt.Errorf("store: list coupons: %w", err)
	}
	return coupons, nil
}

// CouponUpdate 携带可选的优惠券字段更改。
type CouponUpdate struct {
	Code           *string
	Type           *string
	Value          *int64
	MinAmountCents *int64
	MaxUses        *int64
	PerUserLimit   *int64
	ExpiresAt      *time.Time
	ClearExpiry    bool
	Active         *bool
}

// UpdateCoupon 应用可选字段更改并返回更新后的记录。
func (r *Repository) UpdateCoupon(ctx context.Context, id string, update CouponUpdate) (*Coupon, error) {
	fields := map[string]any{}
	if update.Code != nil {
		fields["code"] = *update.Code
	}
	if update.Type != nil {
		fields["type"] = *update.Type
	}
	if update.Value != nil {
		fields["value"] = *update.Value
	}
	if update.MinAmountCents != nil {
		fields["min_amount_cents"] = *update.MinAmountCents
	}
	if update.MaxUses != nil {
		fields["max_uses"] = *update.MaxUses
	}
	if update.PerUserLimit != nil {
		fields["per_user_limit"] = *update.PerUserLimit
	}
	if update.ClearExpiry {
		fields["expires_at"] = nil
	} else if update.ExpiresAt != nil {
		fields["expires_at"] = *update.ExpiresAt
	}
	if update.Active != nil {
		fields["active"] = *update.Active
	}
	if len(fields) > 0 {
		if err := r.db.WithContext(ctx).Model(&Coupon{}).Where("id = ?", id).Updates(fields).Error; err != nil {
			return nil, fmt.Errorf("store: update coupon %q: %w", id, err)
		}
	}
	return r.GetCouponByID(ctx, id)
}

// DeleteCoupon 删除一张优惠券，并清理其使用记录。
func (r *Repository) DeleteCoupon(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("coupon_id = ?", id).Delete(&CouponRedemption{}).Error; err != nil {
			return err
		}
		result := tx.Delete(&Coupon{}, "id = ?", id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// CountUserRedemptions 统计某个用户对某张优惠券已使用的次数。
func (r *Repository) CountUserRedemptions(ctx context.Context, couponID, userID string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&CouponRedemption{}).
		Where("coupon_id = ? AND user_id = ?", couponID, userID).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("store: count coupon redemptions: %w", err)
	}
	return count, nil
}

// RedeemCoupon 在事务中原子性地校验并消费一张优惠券：递增使用次数并记录
// 一次使用。当校验失败时返回相应的 sentinel 错误。
func (r *Repository) RedeemCoupon(ctx context.Context, couponID, userID, orderID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var coupon Coupon
		if err := tx.First(&coupon, "id = ?", couponID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if !coupon.Active {
			return ErrCouponInactive
		}
		if coupon.ExpiresAt != nil && time.Now().After(*coupon.ExpiresAt) {
			return ErrCouponExpired
		}
		if coupon.MaxUses > 0 && coupon.Used >= coupon.MaxUses {
			return ErrCouponExhausted
		}
		if coupon.PerUserLimit > 0 {
			var used int64
			if err := tx.Model(&CouponRedemption{}).
				Where("coupon_id = ? AND user_id = ?", couponID, userID).
				Count(&used).Error; err != nil {
				return err
			}
			if used >= coupon.PerUserLimit {
				return ErrCouponLimitReached
			}
		}
		if err := tx.Model(&Coupon{}).Where("id = ?", couponID).
			UpdateColumn("used", gorm.Expr("used + 1")).Error; err != nil {
			return err
		}
		redemption := &CouponRedemption{
			ID:       newID(),
			CouponID: couponID,
			UserID:   userID,
			OrderID:  orderID,
		}
		return tx.Create(redemption).Error
	})
}

// 优惠券相关的 sentinel 错误。
var (
	// ErrCouponInactive 表示优惠券已停用。
	ErrCouponInactive = errors.New("store: coupon is inactive")
	// ErrCouponExpired 表示优惠券已过期。
	ErrCouponExpired = errors.New("store: coupon has expired")
	// ErrCouponExhausted 表示优惠券使用次数已达上限。
	ErrCouponExhausted = errors.New("store: coupon is exhausted")
	// ErrCouponLimitReached 表示该用户使用次数已达上限。
	ErrCouponLimitReached = errors.New("store: coupon per-user limit reached")
)

// ---- 工单 ----

// CreateTicket 插入一个工单。
func (r *Repository) CreateTicket(ctx context.Context, ticket *Ticket) error {
	if err := r.db.WithContext(ctx).Create(ticket).Error; err != nil {
		return fmt.Errorf("store: create ticket: %w", err)
	}
	return nil
}

// GetTicketByID 返回具有给定 id 的工单，或 ErrNotFound。
func (r *Repository) GetTicketByID(ctx context.Context, id string) (*Ticket, error) {
	var ticket Ticket
	if err := r.first(ctx, &ticket, "id = ?", id); err != nil {
		return nil, fmt.Errorf("store: ticket %q: %w", id, err)
	}
	return &ticket, nil
}

// ListTicketsByUser 返回某个用户的工单，最新的在前。
func (r *Repository) ListTicketsByUser(ctx context.Context, userID string) ([]Ticket, error) {
	var tickets []Ticket
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).
		Order("updated_at DESC").Find(&tickets).Error; err != nil {
		return nil, fmt.Errorf("store: list tickets: %w", err)
	}
	return tickets, nil
}

// ListTickets 返回全部工单，可按状态过滤，最近更新的在前。
func (r *Repository) ListTickets(ctx context.Context, status string) ([]Ticket, error) {
	query := r.db.WithContext(ctx).Order("updated_at DESC")
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var tickets []Ticket
	if err := query.Find(&tickets).Error; err != nil {
		return nil, fmt.Errorf("store: list tickets: %w", err)
	}
	return tickets, nil
}

// UpdateTicketStatus 更新工单状态。
func (r *Repository) UpdateTicketStatus(ctx context.Context, id, status string) (*Ticket, error) {
	result := r.db.WithContext(ctx).Model(&Ticket{}).Where("id = ?", id).
		Update("status", status)
	if result.Error != nil {
		return nil, fmt.Errorf("store: update ticket %q: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, fmt.Errorf("store: ticket %q: %w", id, ErrNotFound)
	}
	return r.GetTicketByID(ctx, id)
}

// CreateTicketMessage 插入一条工单消息。
func (r *Repository) CreateTicketMessage(ctx context.Context, msg *TicketMessage) error {
	if err := r.db.WithContext(ctx).Create(msg).Error; err != nil {
		return fmt.Errorf("store: create ticket message: %w", err)
	}
	return nil
}

// ListTicketMessages 返回某个工单的消息，按时间正序。
func (r *Repository) ListTicketMessages(ctx context.Context, ticketID string) ([]TicketMessage, error) {
	var messages []TicketMessage
	if err := r.db.WithContext(ctx).Where("ticket_id = ?", ticketID).
		Order("created_at ASC").Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("store: list ticket messages: %w", err)
	}
	return messages, nil
}

// TouchTicket 更新工单的更新时间。
func (r *Repository) TouchTicket(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).Model(&Ticket{}).Where("id = ?", id).
		UpdateColumn("updated_at", time.Now()).Error; err != nil {
		return fmt.Errorf("store: touch ticket %q: %w", id, err)
	}
	return nil
}

package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/AXmishell/axmipic/internal/service"
)

type planRequest struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	PriceCents   int64  `json:"price_cents"`
	DurationDays int    `json:"duration_days"`
	QuotaMB      int64  `json:"quota_mb"`
	RoleGroupID  string `json:"role_group_id"`
	Active       bool   `json:"active"`
	SortOrder    int    `json:"sort_order"`
}

type couponRequest struct {
	Code           string     `json:"code"`
	Type           string     `json:"type"`
	Value          int64      `json:"value"`
	MinAmountCents int64      `json:"min_amount_cents"`
	MaxUses        int64      `json:"max_uses"`
	PerUserLimit   int64      `json:"per_user_limit"`
	ExpiresAt      *time.Time `json:"expires_at"`
	Active         bool       `json:"active"`
}

type orderRequest struct {
	PlanID   string `json:"plan_id"`
	Coupon   string `json:"coupon_code"`
	Provider string `json:"provider"`
}

type couponValidateRequest struct {
	Code        string `json:"code"`
	AmountCents int64  `json:"amount_cents"`
}

type ticketRequest struct {
	Subject  string `json:"subject"`
	Category string `json:"category"`
	Body     string `json:"body"`
	Priority string `json:"priority"`
}

type ticketReplyRequest struct {
	Body string `json:"body"`
}

type ticketStatusRequest struct {
	Status string `json:"status"`
}

// ---- 套餐 ----

// listPlans 返回启用的套餐（无需登录）。
func (h *Handler) listPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := h.billing.ListPlans(r.Context(), true)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, plans)
}

func (h *Handler) adminListPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := h.billing.ListPlans(r.Context(), false)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, plans)
}

func (h *Handler) adminCreatePlan(w http.ResponseWriter, r *http.Request) {
	body, ok := decodePlan(w, r)
	if !ok {
		return
	}
	plan, err := h.billing.CreatePlan(r.Context(), body)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeCreated(w, plan)
}

func (h *Handler) adminUpdatePlan(w http.ResponseWriter, r *http.Request) {
	body, ok := decodePlan(w, r)
	if !ok {
		return
	}
	plan, err := h.billing.UpdatePlan(r.Context(), chi.URLParam(r, "id"), body)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, plan)
}

func (h *Handler) adminDeletePlan(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.billing.DeletePlan(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, map[string]string{"id": id})
}

// ---- 优惠券 ----

func (h *Handler) adminListCoupons(w http.ResponseWriter, r *http.Request) {
	coupons, err := h.billing.ListCoupons(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, coupons)
}

func (h *Handler) adminCreateCoupon(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeCoupon(w, r)
	if !ok {
		return
	}
	coupon, err := h.billing.CreateCoupon(r.Context(), body)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeCreated(w, coupon)
}

func (h *Handler) adminUpdateCoupon(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeCoupon(w, r)
	if !ok {
		return
	}
	coupon, err := h.billing.UpdateCoupon(r.Context(), chi.URLParam(r, "id"), body)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, coupon)
}

func (h *Handler) adminDeleteCoupon(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.billing.DeleteCoupon(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, map[string]string{"id": id})
}

// validateCoupon 预览优惠券折扣。
func (h *Handler) validateCoupon(w http.ResponseWriter, r *http.Request) {
	var body couponValidateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	coupon, discount, err := h.billing.ValidateCoupon(r.Context(), principalOf(r), body.Code, body.AmountCents)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, map[string]any{"coupon": coupon, "discount_cents": discount})
}

// ---- 订单 ----

func (h *Handler) createOrder(w http.ResponseWriter, r *http.Request) {
	var body orderRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	order, err := h.billing.CreateOrder(r.Context(), principalOf(r), body.PlanID, body.Coupon, body.Provider)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeCreated(w, order)
}

func (h *Handler) listOrders(w http.ResponseWriter, r *http.Request) {
	orders, err := h.billing.ListOrders(r.Context(), principalOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, orders)
}

// payOrder 完成一笔手动/模拟订单的支付。
func (h *Handler) payOrder(w http.ResponseWriter, r *http.Request) {
	order, err := h.billing.PayOrder(r.Context(), principalOf(r), chi.URLParam(r, "id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, order)
}

// ---- 工单 ----

func (h *Handler) createTicket(w http.ResponseWriter, r *http.Request) {
	var body ticketRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	ticket, err := h.billing.CreateTicket(r.Context(), principalOf(r), body.Subject, body.Category, body.Body, body.Priority)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeCreated(w, ticket)
}

func (h *Handler) listTickets(w http.ResponseWriter, r *http.Request) {
	tickets, err := h.billing.ListTickets(r.Context(), principalOf(r), r.URL.Query().Get("status"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, tickets)
}

func (h *Handler) getTicket(w http.ResponseWriter, r *http.Request) {
	ticket, err := h.billing.GetTicket(r.Context(), principalOf(r), chi.URLParam(r, "id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, ticket)
}

func (h *Handler) replyTicket(w http.ResponseWriter, r *http.Request) {
	var body ticketReplyRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	ticket, err := h.billing.ReplyTicket(r.Context(), principalOf(r), chi.URLParam(r, "id"), body.Body)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, ticket)
}

func (h *Handler) adminSetTicketStatus(w http.ResponseWriter, r *http.Request) {
	var body ticketStatusRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	ticket, err := h.billing.SetTicketStatus(r.Context(), chi.URLParam(r, "id"), body.Status)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, ticket)
}

func decodePlan(w http.ResponseWriter, r *http.Request) (service.PlanInput, bool) {
	var body planRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return service.PlanInput{}, false
	}
	return service.PlanInput{
		Name:         body.Name,
		Description:  body.Description,
		PriceCents:   body.PriceCents,
		DurationDays: body.DurationDays,
		QuotaMB:      body.QuotaMB,
		RoleGroupID:  body.RoleGroupID,
		Active:       body.Active,
		SortOrder:    body.SortOrder,
	}, true
}

func decodeCoupon(w http.ResponseWriter, r *http.Request) (service.CouponInput, bool) {
	var body couponRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return service.CouponInput{}, false
	}
	return service.CouponInput{
		Code:           body.Code,
		Type:           body.Type,
		Value:          body.Value,
		MinAmountCents: body.MinAmountCents,
		MaxUses:        body.MaxUses,
		PerUserLimit:   body.PerUserLimit,
		ExpiresAt:      body.ExpiresAt,
		Active:         body.Active,
	}, true
}

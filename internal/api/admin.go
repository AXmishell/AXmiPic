package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/AXmishell/axmipic/internal/service"
)

func (h *Handler) adminStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.admin.Stats(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, stats)
}

// adminCustomerPage 是普通用户列表的分页响应。
type adminCustomerPage struct {
	Items    []service.UserDTO `json:"items"`
	Total    int64             `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
}

// adminCustomers 列出普通账户。
//
// 未提供任何分页/筛选参数时保持旧的数组响应（Go SDK 依赖该形状）；提供
// page、page_size、keyword、status、role_group_id 或 sort 任一参数时返回
// 分页对象。
func (h *Handler) adminCustomers(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	paged := query.Has("page") || query.Has("page_size") || query.Has("keyword") ||
		query.Has("status") || query.Has("role_group_id") || query.Has("sort")
	if !paged {
		users, err := h.admin.ListCustomers(r.Context())
		if err != nil {
			h.fail(w, r, err)
			return
		}
		writeOK(w, users)
		return
	}

	const defaultPageSize, maxPageSize = 20, 200
	page := queryInt(r, "page")
	if page < 1 {
		page = 1
	}
	pageSize := queryInt(r, "page_size")
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	status := strings.ToLower(strings.TrimSpace(query.Get("status")))
	if status != "enabled" && status != "disabled" {
		status = ""
	}
	items, total, err := h.admin.SearchCustomers(r.Context(), service.CustomerQuery{
		Offset:      (page - 1) * pageSize,
		Limit:       pageSize,
		Keyword:     strings.TrimSpace(query.Get("keyword")),
		Status:      status,
		RoleGroupID: strings.TrimSpace(query.Get("role_group_id")),
		Sort:        strings.TrimSpace(query.Get("sort")),
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, adminCustomerPage{Items: items, Total: total, Page: page, PageSize: pageSize})
}

// adminAdmins 列出特权账户。
func (h *Handler) adminAdmins(w http.ResponseWriter, r *http.Request) {
	users, err := h.admin.ListAdmins(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, users)
}

type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Handler) adminCreateAdmin(w http.ResponseWriter, r *http.Request) {
	var body createUserRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	user, err := h.admin.RegisterAdmin(r.Context(), h.accounts, body.Username, body.Password)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeCreated(w, user)
}

type updateUserRequest struct {
	Disabled    *bool   `json:"disabled"`
	RoleGroupID *string `json:"role_group_id"`
	Username    *string `json:"username"`
	Email       *string `json:"email"`
	Password    *string `json:"password"`
}

// updateFunc 对指定角色的账户应用禁用/启用更改。
type updateFunc func(ctx context.Context, actorID, id string, in service.UpdateUserInput) (*service.UserDTO, error)

func (h *Handler) adminUpdateCustomer(w http.ResponseWriter, r *http.Request) {
	h.adminUpdateAccount(w, r, func(ctx context.Context, _, id string, in service.UpdateUserInput) (*service.UserDTO, error) {
		return h.admin.UpdateCustomer(ctx, id, in)
	})
}

func (h *Handler) adminUpdateAdmin(w http.ResponseWriter, r *http.Request) {
	h.adminUpdateAccount(w, r, h.admin.UpdateAdmin)
}

func (h *Handler) adminUpdateAccount(w http.ResponseWriter, r *http.Request, update updateFunc) {
	var body updateUserRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	user, err := update(r.Context(), principalOf(r).UserID, chi.URLParam(r, "id"), service.UpdateUserInput{
		Disabled:    body.Disabled,
		RoleGroupID: body.RoleGroupID,
		Username:    body.Username,
		Email:       body.Email,
		Password:    body.Password,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, user)
}

func (h *Handler) adminDeleteCustomer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.admin.DeleteCustomer(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, map[string]string{"id": id})
}

func (h *Handler) adminDeleteAdmin(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.admin.DeleteAdmin(r.Context(), principalOf(r).UserID, id); err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, map[string]string{"id": id})
}

package api

import (
	"context"
	"encoding/json"
	"net/http"

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

// adminCustomers 列出普通账户。
func (h *Handler) adminCustomers(w http.ResponseWriter, r *http.Request) {
	users, err := h.admin.ListCustomers(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, users)
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
	Disabled *bool `json:"disabled"`
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
	user, err := update(r.Context(), principalOf(r).UserID, chi.URLParam(r, "id"), service.UpdateUserInput{Disabled: body.Disabled})
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

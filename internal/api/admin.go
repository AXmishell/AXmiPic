package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/axmipic/axmipic/internal/service"
)

func (h *Handler) adminStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.admin.Stats(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, stats)
}

func (h *Handler) adminUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.admin.ListUsers(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, users)
}

type updateUserRequest struct {
	Role     *string `json:"role"`
	Disabled *bool   `json:"disabled"`
}

func (h *Handler) adminUpdateUser(w http.ResponseWriter, r *http.Request) {
	var body updateUserRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	user, err := h.admin.UpdateUser(r.Context(), chi.URLParam(r, "id"), service.UpdateUserInput{
		Role:     body.Role,
		Disabled: body.Disabled,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, user)
}

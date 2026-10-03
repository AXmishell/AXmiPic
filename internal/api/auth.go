package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type credentialsRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var body credentialsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	user, err := h.accounts.Register(r.Context(), body.Username, body.Password)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeCreated(w, user)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var body credentialsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	session, err := h.accounts.Login(r.Context(), body.Username, body.Password)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, session)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	principal := principalOf(r)
	if principal == nil || principal.UserID == "" {
		writeError(w, http.StatusUnauthorized, http.StatusUnauthorized, "authentication required")
		return
	}
	user, err := h.accounts.Me(r.Context(), principal.UserID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, user)
}

type createTokenRequest struct {
	Name string `json:"name"`
}

func (h *Handler) createToken(w http.ResponseWriter, r *http.Request) {
	var body createTokenRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	principal := principalOf(r)
	token, err := h.accounts.CreateToken(r.Context(), principal.UserID, body.Name)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeCreated(w, token)
}

func (h *Handler) listTokens(w http.ResponseWriter, r *http.Request) {
	principal := principalOf(r)
	tokens, err := h.accounts.ListTokens(r.Context(), principal.UserID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, tokens)
}

func (h *Handler) deleteToken(w http.ResponseWriter, r *http.Request) {
	principal := principalOf(r)
	id := chi.URLParam(r, "id")
	if err := h.accounts.RevokeToken(r.Context(), principal.UserID, id); err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, map[string]string{"id": id})
}

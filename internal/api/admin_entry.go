package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/AXmishell/axmipic/internal/service"
)

// adminEntryStatus 返回管理员安全入口是否开启，以及当前请求是否持有有效入口 Cookie。
func (h *Handler) adminEntryStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h.adminEntry == nil {
		writeOK(w, map[string]bool{"enabled": false, "unlocked": true})
		return
	}
	enabled := h.adminEntry.Enabled()
	writeOK(w, map[string]bool{
		"enabled":  enabled,
		"unlocked": !enabled || h.adminEntry.Unlocked(r),
	})
}

// requireAdminEntry 在安全入口启用时要求登录请求持有有效的入口 Cookie。
func (h *Handler) requireAdminEntry(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.adminEntry != nil && h.adminEntry.Enabled() && !h.adminEntry.Unlocked(r) {
			writeError(w, http.StatusNotFound, http.StatusNotFound, "not found")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// adminEntryPage 校验安全入口 token，设置短期入口 Cookie 后跳转到登录页。
func (h *Handler) adminEntryPage(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex")

	if h.adminEntry == nil {
		http.NotFound(w, r)
		return
	}
	if !h.adminEntry.Enabled() {
		// 安全入口关闭后，旧书签回到普通登录页。
		http.Redirect(w, r, "/admin/login", http.StatusFound)
		return
	}
	if !h.adminEntry.ValidateToken(token) {
		http.NotFound(w, r)
		return
	}
	if err := h.adminEntry.SetEntryCookie(w, h.secureRequest(r)); err != nil {
		h.logger.ErrorContext(r.Context(), "failed to set admin entry cookie", slog.Any("error", err))
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/admin/login", http.StatusFound)
}

// adminLoginPage 在安全入口启用时，仅允许持有入口 Cookie 的请求加载登录页。
func (h *Handler) adminLoginPage(w http.ResponseWriter, r *http.Request) {
	if h.adminEntry != nil && h.adminEntry.Enabled() && !h.adminEntry.Unlocked(r) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex")
	if h.static == nil {
		http.NotFound(w, r)
		return
	}
	h.static.ServeHTTP(w, r)
}

// adminGetAdminEntry 返回安全入口状态与完整入口 URL。
func (h *Handler) adminGetAdminEntry(w http.ResponseWriter, r *http.Request) {
	if h.adminEntry == nil {
		writeError(w, http.StatusNotFound, http.StatusNotFound, "admin entry is not available")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeOK(w, h.adminEntryStatusPayload(r))
}

type adminEntryUpdateRequest struct {
	Enabled         bool   `json:"enabled"`
	CurrentPassword string `json:"current_password"`
	TOTPCode        string `json:"totp_code"`
}

type adminEntryAuthRequest struct {
	CurrentPassword string `json:"current_password"`
	TOTPCode        string `json:"totp_code"`
}

// adminUpdateAdminEntry 开启或关闭管理员安全入口。
func (h *Handler) adminUpdateAdminEntry(w http.ResponseWriter, r *http.Request) {
	if h.adminEntry == nil {
		writeError(w, http.StatusNotFound, http.StatusNotFound, "admin entry is not available")
		return
	}
	var body adminEntryUpdateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := h.accounts.VerifySensitiveAction(r.Context(), principalOf(r), body.CurrentPassword, body.TOTPCode); err != nil {
		h.fail(w, r, err)
		return
	}
	if _, err := h.adminEntry.SetEnabled(r.Context(), body.Enabled); err != nil {
		h.failAdminEntry(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeOK(w, h.adminEntryStatusPayload(r))
}

// adminRotateAdminEntry 生成新的安全入口 token，旧入口立即失效。
func (h *Handler) adminRotateAdminEntry(w http.ResponseWriter, r *http.Request) {
	if h.adminEntry == nil {
		writeError(w, http.StatusNotFound, http.StatusNotFound, "admin entry is not available")
		return
	}
	var body adminEntryAuthRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := h.accounts.VerifySensitiveAction(r.Context(), principalOf(r), body.CurrentPassword, body.TOTPCode); err != nil {
		h.fail(w, r, err)
		return
	}
	if _, err := h.adminEntry.Rotate(r.Context()); err != nil {
		h.failAdminEntry(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeOK(w, h.adminEntryStatusPayload(r))
}

func (h *Handler) failAdminEntry(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, service.ErrAdminEntryManaged) {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "admin entry is managed by configuration")
		return
	}
	h.fail(w, r, err)
}

func (h *Handler) adminEntryStatusPayload(r *http.Request) map[string]any {
	status := h.adminEntry.Status(h.adminEntryBaseURL(r))
	payload := map[string]any{
		"enabled": status.Enabled,
	}
	if !status.UpdatedAt.IsZero() {
		payload["updated_at"] = status.UpdatedAt
	}
	if status.Enabled {
		payload["url"] = status.URL
	}
	return payload
}

func (h *Handler) adminEntryBaseURL(r *http.Request) string {
	if baseURL := strings.TrimRight(strings.TrimSpace(h.runtime.BaseURL), "/"); baseURL != "" {
		return baseURL
	}
	scheme := "http"
	if h.secureRequest(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func (h *Handler) secureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.HasPrefix(strings.ToLower(h.runtime.BaseURL), "https://")
}

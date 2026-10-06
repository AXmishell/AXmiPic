package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/AXmishell/axmipic/internal/service"
)

// adminGetSettingDomain 返回指定设置域的当前值。
func (h *Handler) adminGetSettingDomain(w http.ResponseWriter, r *http.Request) {
	if h.settings == nil {
		writeError(w, http.StatusNotFound, http.StatusNotFound, "settings unavailable")
		return
	}
	domain, ok := h.settings.Domain(chi.URLParam(r, "domain"))
	if !ok {
		writeError(w, http.StatusNotFound, http.StatusNotFound, "unknown settings domain")
		return
	}
	writeOK(w, domain.Get())
}

// adminUpdateSettingDomain 校验并保存指定设置域。
func (h *Handler) adminUpdateSettingDomain(w http.ResponseWriter, r *http.Request) {
	if h.settings == nil {
		writeError(w, http.StatusNotFound, http.StatusNotFound, "settings unavailable")
		return
	}
	domain, ok := h.settings.Domain(chi.URLParam(r, "domain"))
	if !ok {
		writeError(w, http.StatusNotFound, http.StatusNotFound, "unknown settings domain")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxJSONBody))
	if err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	value, err := domain.Update(r.Context(), body)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, value)
}

// adminGetSMTP 返回当前 SMTP 设置（密码仅返回是否已设置）。
func (h *Handler) adminGetSMTP(w http.ResponseWriter, r *http.Request) {
	if h.settings == nil {
		writeOK(w, service.SMTPConfigDTO{})
		return
	}
	writeOK(w, h.settings.SMTP())
}

// adminUpdateSMTP 更新 SMTP 设置并即时生效。
func (h *Handler) adminUpdateSMTP(w http.ResponseWriter, r *http.Request) {
	var body service.SMTPInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if h.settings == nil {
		writeError(w, http.StatusServiceUnavailable, http.StatusServiceUnavailable, "settings service is unavailable")
		return
	}
	cfg, err := h.settings.UpdateSMTP(r.Context(), body)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, cfg)
}

// adminGetPayment 返回当前支付设置（密钥仅返回是否已设置）。
func (h *Handler) adminGetPayment(w http.ResponseWriter, r *http.Request) {
	if h.settings == nil {
		writeOK(w, service.PaymentSettingsDTO{})
		return
	}
	writeOK(w, h.settings.Payment())
}

// adminGetAuth 返回可在线切换的权限开关。
func (h *Handler) adminGetAuth(w http.ResponseWriter, r *http.Request) {
	if h.settings == nil {
		writeOK(w, service.AuthConfig{})
		return
	}
	writeOK(w, h.settings.Auth())
}

// adminUpdateAuth 更新权限开关并即时生效（无需重启）。
func (h *Handler) adminUpdateAuth(w http.ResponseWriter, r *http.Request) {
	var body service.AuthConfig
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if h.settings == nil {
		writeError(w, http.StatusServiceUnavailable, http.StatusServiceUnavailable, "settings service is unavailable")
		return
	}
	cfg, err := h.settings.UpdateAuth(r.Context(), body)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, cfg)
}

// adminGetModeration 返回图片广场 AI 审查设置（密钥仅返回是否已设置）。
func (h *Handler) adminGetModeration(w http.ResponseWriter, r *http.Request) {
	if h.settings == nil {
		writeOK(w, service.ModerationSettingsDTO{})
		return
	}
	writeOK(w, h.settings.Moderation())
}

// adminUpdateModeration 更新图片广场 AI 审查设置并即时生效（无需重启）。
func (h *Handler) adminUpdateModeration(w http.ResponseWriter, r *http.Request) {
	var body service.ModerationSettingsInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if h.settings == nil {
		writeError(w, http.StatusServiceUnavailable, http.StatusServiceUnavailable, "settings service is unavailable")
		return
	}
	cfg, err := h.settings.UpdateModeration(r.Context(), body)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, cfg)
}

// adminUpdatePayment 更新支付设置并即时生效（无需重启）。
func (h *Handler) adminUpdatePayment(w http.ResponseWriter, r *http.Request) {
	var body service.PaymentSettingsInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if h.settings == nil {
		writeError(w, http.StatusServiceUnavailable, http.StatusServiceUnavailable, "settings service is unavailable")
		return
	}
	cfg, err := h.settings.UpdatePayment(r.Context(), body)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, cfg)
}

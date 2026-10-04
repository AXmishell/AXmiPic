package api

import (
	"encoding/json"
	"net/http"

	"github.com/AXmishell/axmipic/internal/service"
)

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

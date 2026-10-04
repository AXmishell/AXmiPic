package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/service"
	"github.com/AXmishell/axmipic/internal/storage"
)

// principalOf 返回附加到请求上的已认证主体，匿名请求则返回
// nil。
func principalOf(r *http.Request) *auth.Principal {
	principal, _ := auth.FromContext(r.Context())
	return principal
}

// envelope 是统一的 JSON 响应体。
type envelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func writeJSON(w http.ResponseWriter, status int, body envelope) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("api: encode response", slog.Any("error", err))
	}
}

func writeOK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, envelope{Code: 0, Message: "ok", Data: data})
}

func writeCreated(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusCreated, envelope{Code: 0, Message: "ok", Data: data})
}

func writeError(w http.ResponseWriter, status, code int, message string) {
	writeJSON(w, status, envelope{Code: code, Message: message, Data: nil})
}

// fail 将领域错误映射为 HTTP 响应，并记录非预期的错误。
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrFileTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, http.StatusRequestEntityTooLarge, err.Error())
	case errors.Is(err, service.ErrUnsupportedType):
		writeError(w, http.StatusUnsupportedMediaType, http.StatusUnsupportedMediaType, err.Error())
	case errors.Is(err, service.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrPresignUnsupported):
		writeError(w, http.StatusNotImplemented, http.StatusNotImplemented, err.Error())
	case errors.Is(err, service.ErrProcessingUnsupported):
		writeError(w, http.StatusNotImplemented, http.StatusNotImplemented, err.Error())
	case errors.Is(err, service.ErrProcessingFailed):
		writeError(w, http.StatusUnprocessableEntity, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, service.ErrForbidden):
		writeError(w, http.StatusForbidden, http.StatusForbidden, err.Error())
	case errors.Is(err, service.ErrUserExists):
		writeError(w, http.StatusConflict, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, http.StatusUnauthorized, err.Error())
	case errors.Is(err, service.ErrRegistrationDisabled):
		writeError(w, http.StatusForbidden, http.StatusForbidden, err.Error())
	case errors.Is(err, service.ErrTokenNotFound):
		writeError(w, http.StatusNotFound, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrQuotaExceeded):
		writeError(w, http.StatusInsufficientStorage, http.StatusInsufficientStorage, err.Error())
	case errors.Is(err, service.ErrStorageNotFound):
		writeError(w, http.StatusNotFound, http.StatusNotFound, "storage backend not found")
	case errors.Is(err, service.ErrStorageInUse):
		writeError(w, http.StatusConflict, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrStorageConfig):
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrAlbumNotFound):
		writeError(w, http.StatusNotFound, http.StatusNotFound, "album not found")
	case errors.Is(err, service.ErrRoleGroupNotFound):
		writeError(w, http.StatusNotFound, http.StatusNotFound, "role group not found")
	case errors.Is(err, service.ErrUserNotFound):
		writeError(w, http.StatusNotFound, http.StatusNotFound, "user not found")
	case errors.Is(err, service.ErrShareNotFound):
		writeError(w, http.StatusNotFound, http.StatusNotFound, "share not found")
	case errors.Is(err, service.ErrShareUnavailable):
		writeError(w, http.StatusGone, http.StatusGone, err.Error())
	case errors.Is(err, service.ErrSharePasswordRequired):
		writeError(w, http.StatusUnauthorized, http.StatusUnauthorized, err.Error())
	case errors.Is(err, service.ErrShareInvalidPassword):
		writeError(w, http.StatusUnauthorized, http.StatusUnauthorized, err.Error())
	case errors.Is(err, service.ErrAnnouncementNotFound):
		writeError(w, http.StatusNotFound, http.StatusNotFound, "announcement not found")
	case errors.Is(err, service.ErrReportNotFound):
		writeError(w, http.StatusNotFound, http.StatusNotFound, "report not found")
	case errors.Is(err, service.ErrPageNotFound):
		writeError(w, http.StatusNotFound, http.StatusNotFound, "page not found")
	case errors.Is(err, service.ErrPlanNotFound):
		writeError(w, http.StatusNotFound, http.StatusNotFound, "plan not found")
	case errors.Is(err, service.ErrOrderNotFound):
		writeError(w, http.StatusNotFound, http.StatusNotFound, "order not found")
	case errors.Is(err, service.ErrCouponNotFound):
		writeError(w, http.StatusNotFound, http.StatusNotFound, "coupon not found")
	case errors.Is(err, service.ErrCouponInvalid):
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrCouponBelowMinimum):
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrTicketNotFound):
		writeError(w, http.StatusNotFound, http.StatusNotFound, "ticket not found")
	case errors.Is(err, service.ErrPolicyNotFound):
		writeError(w, http.StatusNotFound, http.StatusNotFound, "policy not found")
	case errors.Is(err, service.ErrRoleGroupInUse):
		writeError(w, http.StatusConflict, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrPolicyInUse):
		writeError(w, http.StatusConflict, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrNotFound):
		writeError(w, http.StatusNotFound, http.StatusNotFound, "image not found")
	case errors.Is(err, storage.ErrInvalidKey):
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, "invalid image key")
	default:
		h.logger.ErrorContext(r.Context(), "request failed", slog.Any("error", err))
		writeError(w, http.StatusInternalServerError, http.StatusInternalServerError, "internal server error")
	}
}

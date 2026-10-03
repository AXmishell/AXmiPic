package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/axmipic/axmipic/internal/auth"
	"github.com/axmipic/axmipic/internal/service"
)

// principalOf returns the authenticated principal attached to the request, or
// nil for an anonymous request.
func principalOf(r *http.Request) *auth.Principal {
	principal, _ := auth.FromContext(r.Context())
	return principal
}

// envelope is the uniform JSON response body.
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

// fail maps a domain error to an HTTP response, logging unexpected errors.
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
	case errors.Is(err, service.ErrNotFound):
		writeError(w, http.StatusNotFound, http.StatusNotFound, "image not found")
	default:
		h.logger.ErrorContext(r.Context(), "request failed", slog.Any("error", err))
		writeError(w, http.StatusInternalServerError, http.StatusInternalServerError, "internal server error")
	}
}

package api

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/axmipic/axmipic/internal/service"
	"github.com/axmipic/axmipic/internal/storage"
)

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, envelope{Code: 0, Message: "ok", Data: map[string]string{"status": "ok"}})
}

func (h *Handler) listImages(w http.ResponseWriter, r *http.Request) {
	result, err := h.svc.List(r.Context(), queryInt(r, "page"), queryInt(r, "page_size"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, result)
}

func (h *Handler) getImage(w http.ResponseWriter, r *http.Request) {
	dto, err := h.svc.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, dto)
}

func (h *Handler) deleteImage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.svc.Delete(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, map[string]string{"id": id})
}

// serveImage streams a stored object by its slash-separated key.
func (h *Handler) serveImage(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "*")
	dto, err := h.svc.GetByKey(r.Context(), key)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	object, err := h.storage.Get(r.Context(), key)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			h.fail(w, r, service.ErrNotFound)
			return
		}
		h.fail(w, r, err)
		return
	}
	defer func() {
		if closeErr := object.Close(); closeErr != nil {
			h.logger.WarnContext(r.Context(), "close stored object", slog.Any("error", closeErr))
		}
	}()

	w.Header().Set("Content-Type", dto.MimeType)
	w.Header().Set("Content-Length", strconv.FormatInt(dto.Size, 10))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, object); err != nil {
		// The status line and headers are already sent, so only logging is
		// possible here.
		h.logger.WarnContext(r.Context(), "stream stored object", slog.Any("error", err))
	}
}

// queryInt parses an integer query parameter, returning 0 when absent or
// invalid so that the service can apply its defaults.
func queryInt(r *http.Request, name string) int {
	value, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil {
		return 0
	}
	return value
}

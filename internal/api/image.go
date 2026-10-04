package api

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/AXmishell/axmipic/internal/imaging"
	"github.com/AXmishell/axmipic/internal/service"
	"github.com/AXmishell/axmipic/internal/storage"
)

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, envelope{Code: 0, Message: "ok", Data: map[string]string{"status": "ok"}})
}

func (h *Handler) listImages(w http.ResponseWriter, r *http.Request) {
	result, err := h.svc.List(r.Context(), principalOf(r), queryInt(r, "page"), queryInt(r, "page_size"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, result)
}

func (h *Handler) getImage(w http.ResponseWriter, r *http.Request) {
	dto, err := h.svc.Get(r.Context(), principalOf(r), chi.URLParam(r, "id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, dto)
}

func (h *Handler) deleteImage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.svc.Delete(r.Context(), principalOf(r), id); err != nil {
		h.fail(w, r, err)
		return
	}
	writeOK(w, map[string]string{"id": id})
}

// serveImage streams a stored object by its slash-separated key, applying an
// on-the-fly transformation when query parameters request one.
func (h *Handler) serveImage(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "*")
	req, err := parseTransformQuery(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, http.StatusBadRequest, err.Error())
		return
	}
	// Require registered metadata so on-the-fly transformation cannot serve
	// arbitrary objects straight from the storage backend.
	dto, err := h.svc.GetByKey(r.Context(), key)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if h.imaging == nil || !h.imaging.Enabled() || req.Empty() {
		h.serveOriginal(w, r, dto)
		return
	}

	opts, err := h.imaging.Options(req)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	etag := transformETag(key, opts)
	if match := r.Header.Get("If-None-Match"); match == etag {
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusNotModified)
		return
	}

	result, err := h.imaging.Render(r.Context(), key, opts)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", result.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(result.Data)))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", etag)
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := w.Write(result.Data); err != nil {
		h.logger.WarnContext(r.Context(), "write processed image", slog.Any("error", err))
	}
}

// serveOriginal streams the stored object unchanged.
func (h *Handler) serveOriginal(w http.ResponseWriter, r *http.Request, dto *service.ImageDTO) {
	object, err := h.storage.Get(r.Context(), dto.Key)
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
	if r.Method == http.MethodHead {
		return
	}
	if _, err := io.Copy(w, object); err != nil {
		// The status line and headers are already sent, so only logging is
		// possible here.
		h.logger.WarnContext(r.Context(), "stream stored object", slog.Any("error", err))
	}
}

// parseTransformQuery reads transformation parameters from the query string.
func parseTransformQuery(query url.Values) (service.TransformRequest, error) {
	var req service.TransformRequest
	var err error
	if req.Width, err = queryIntStrict(query, "w"); err != nil {
		return req, err
	}
	if req.Height, err = queryIntStrict(query, "h"); err != nil {
		return req, err
	}
	if req.Quality, err = queryIntStrict(query, "q"); err != nil {
		return req, err
	}
	if req.Rotate, err = queryIntStrict(query, "r"); err != nil {
		return req, err
	}
	req.Fit = query.Get("fit")
	req.Format = query.Get("f")
	req.Enlarge = parseBool(query.Get("enlarge"))
	return req, nil
}

func queryIntStrict(query url.Values, name string) (int, error) {
	raw := query.Get(name)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %q query parameter", name)
	}
	return value, nil
}

func parseBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// transformETag returns a stable ETag for an original key plus transformation.
func transformETag(key string, opts imaging.Options) string {
	sum := sha256.Sum256([]byte(key + "|" + fmt.Sprintf("%+v", opts)))
	return fmt.Sprintf(`"%x"`, sum[:16])
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

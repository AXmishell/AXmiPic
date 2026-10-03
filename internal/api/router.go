// Package api implements the HTTP API of AXmiPic.
package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/axmipic/axmipic/internal/service"
	"github.com/axmipic/axmipic/internal/storage"
)

// Handler holds the dependencies shared by all HTTP handlers.
type Handler struct {
	svc            *service.UploadService
	storage        storage.Storage
	maxUploadBytes int64
	logger         *slog.Logger
}

// NewHandler constructs the API handler set. maxUploadMB is the configured
// single-file limit used to bound request bodies.
func NewHandler(svc *service.UploadService, backend storage.Storage, maxUploadMB int, logger *slog.Logger) *Handler {
	return &Handler{
		svc:            svc,
		storage:        backend,
		maxUploadBytes: int64(maxUploadMB) << 20,
		logger:         logger,
	}
}

// NewRouter builds the HTTP router with middleware and routes registered.
func NewRouter(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(requestLogger(h.logger))
	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/healthz", h.health)
	r.Get("/i/*", h.serveImage)

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/upload", h.uploadImage)
		r.Post("/upload/presign", h.presignUpload)
		r.Post("/upload/confirm", h.confirmUpload)
		r.Get("/images", h.listImages)
		r.Get("/images/{id}", h.getImage)
		r.Delete("/images/{id}", h.deleteImage)
	})

	return r
}

// requestLogger logs each request with method, path, status, and duration.
func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			logger.InfoContext(r.Context(), "http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", ww.Status()),
				slog.Int("bytes", ww.BytesWritten()),
				slog.Duration("duration", time.Since(start)),
			)
		})
	}
}

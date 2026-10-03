// Package api implements the HTTP API of AXmiPic.
package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/axmipic/axmipic/internal/auth"
	"github.com/axmipic/axmipic/internal/service"
	"github.com/axmipic/axmipic/internal/storage"
)

// Deps are the dependencies required to build the API router.
type Deps struct {
	Upload        *service.UploadService
	Imaging       *service.ImagingService
	Accounts      *service.AccountService
	Storage       storage.Storage
	Authenticator *auth.Authenticator
	UploadLimiter *auth.UploadLimiter
	RequireAuth   bool
	MaxUploadMB   int
	Logger        *slog.Logger
}

// Handler holds the dependencies shared by all HTTP handlers.
type Handler struct {
	svc            *service.UploadService
	imaging        *service.ImagingService
	accounts       *service.AccountService
	storage        storage.Storage
	maxUploadBytes int64
	logger         *slog.Logger
}

// NewRouter builds the HTTP router with middleware and routes registered.
func NewRouter(d Deps) http.Handler {
	h := &Handler{
		svc:            d.Upload,
		imaging:        d.Imaging,
		accounts:       d.Accounts,
		storage:        d.Storage,
		maxUploadBytes: int64(d.MaxUploadMB) << 20,
		logger:         d.Logger,
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(requestLogger(d.Logger))
	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/healthz", h.health)
	r.Get("/i/*", h.serveImage)

	r.Route("/api/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(d.UploadLimiter.Middleware)
			r.Post("/auth/register", h.register)
			r.Post("/auth/login", h.login)
		})

		r.Group(func(r chi.Router) {
			r.Use(d.Authenticator.Authenticate)

			r.Group(func(r chi.Router) {
				if d.RequireAuth {
					r.Use(auth.RequireAuth)
				}
				r.Use(d.UploadLimiter.Middleware)
				r.Post("/upload", h.uploadImage)
				r.Post("/upload/presign", h.presignUpload)
				r.Post("/upload/confirm", h.confirmUpload)
			})

			r.Group(func(r chi.Router) {
				r.Use(auth.RequireAuth)
				r.Get("/auth/me", h.me)
				r.Post("/tokens", h.createToken)
				r.Get("/tokens", h.listTokens)
				r.Delete("/tokens/{id}", h.deleteToken)
				r.Get("/images", h.listImages)
				r.Get("/images/{id}", h.getImage)
				r.Delete("/images/{id}", h.deleteImage)
			})
		})
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

// Package api implements the HTTP API of AXmiPic.
package api

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/service"
	"github.com/AXmishell/axmipic/internal/storage"
)

// Deps are the dependencies required to build the API router.
type Deps struct {
	Upload        *service.UploadService
	Imaging       *service.ImagingService
	Accounts      *service.AccountService
	Admin         *service.AdminService
	Storage       storage.Storage
	Authenticator *auth.Authenticator
	UploadLimiter *auth.UploadLimiter
	// ImageLimiter rate-limits public image serving and transformation by IP.
	ImageLimiter *auth.RateLimiter
	// Static, when non-nil, serves the single-page app for unmatched routes.
	Static      http.Handler
	RequireAuth bool
	// TrustProxy enables parsing the client IP from X-Forwarded-For / X-Real-IP.
	TrustProxy  bool
	MaxUploadMB int
	Logger      *slog.Logger
}

// Handler holds the dependencies shared by all HTTP handlers.
type Handler struct {
	svc            *service.UploadService
	imaging        *service.ImagingService
	accounts       *service.AccountService
	admin          *service.AdminService
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
		admin:          d.Admin,
		storage:        d.Storage,
		maxUploadBytes: int64(d.MaxUploadMB) << 20,
		logger:         d.Logger,
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	if d.TrustProxy {
		// Only honour forwarding headers when explicitly configured to sit
		// behind a trusted proxy; otherwise clients could spoof their IP to
		// evade rate limiting.
		r.Use(trustProxyIP)
	}
	r.Use(securityHeaders)
	r.Use(middleware.Recoverer)
	r.Use(requestLogger(d.Logger))
	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/healthz", h.health)
	r.With(d.ImageLimiter.Middleware).Get("/i/*", h.serveImage)
	r.With(d.ImageLimiter.Middleware).Head("/i/*", h.serveImage)

	r.Route("/api/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(d.UploadLimiter.Middleware)
			r.Post("/auth/register", h.register)
			r.Post("/auth/login", h.login)
			r.Post("/admin/auth/login", h.adminLogin)
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

			r.Group(func(r chi.Router) {
				r.Use(auth.RequireAdmin)
				r.Get("/admin/stats", h.adminStats)
				r.Get("/admin/customers", h.adminCustomers)
				r.Get("/admin/admins", h.adminAdmins)
				r.Post("/admin/admins", h.adminCreateAdmin)
				r.Patch("/admin/customers/{id}", h.adminUpdateCustomer)
				r.Patch("/admin/admins/{id}", h.adminUpdateAdmin)
				r.Delete("/admin/customers/{id}", h.adminDeleteCustomer)
				r.Delete("/admin/admins/{id}", h.adminDeleteAdmin)
			})
		})
	})

	if d.Static != nil {
		r.Handle("/*", d.Static)
	}

	return r
}

// securityHeaders applies defense-in-depth response headers to every reply.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// trustProxyIP rewrites RemoteAddr from forwarding headers. It is only installed
// when server.trust_proxy is enabled, because clients can otherwise forge those
// headers to spoof their IP. It replaces the deprecated chi middleware.RealIP,
// which trusts those headers unconditionally.
func trustProxyIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ip := forwardedClientIP(r); ip != "" {
			r.RemoteAddr = ip
		}
		next.ServeHTTP(w, r)
	})
}

// forwardedClientIP returns the client IP advertised by a trusted proxy,
// preferring X-Real-IP (typically the proxy's peer address) and falling back to
// the first X-Forwarded-For entry.
func forwardedClientIP(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first, _, _ := strings.Cut(xff, ",")
		return strings.TrimSpace(first)
	}
	return ""
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

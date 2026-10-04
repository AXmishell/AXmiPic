// Package api 实现 AXmiPic 的 HTTP API。
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
)

// Deps 是构建 API 路由所需的依赖项。
type Deps struct {
	Upload        *service.UploadService
	Imaging       *service.ImagingService
	Accounts      *service.AccountService
	Admin         *service.AdminService
	Albums        *service.AlbumService
	Storage       *service.StorageService
	Policies      *service.PolicyService
	Shares        *service.ShareService
	Site          *service.SiteService
	Billing       *service.BillingService
	Authenticator *auth.Authenticator
	UploadLimiter *auth.UploadLimiter
	// ImageLimiter 按 IP 对公开图片服务和转换进行限流。
	ImageLimiter *auth.RateLimiter
	// Static 非 nil 时，为未匹配的路由提供单页应用服务。
	Static      http.Handler
	RequireAuth bool
	// TrustProxy 启用从 X-Forwarded-For / X-Real-IP 解析客户端 IP。
	TrustProxy  bool
	MaxUploadMB int
	Logger      *slog.Logger
}

// Handler 保存所有 HTTP 处理函数共享的依赖项。
type Handler struct {
	svc            *service.UploadService
	imaging        *service.ImagingService
	accounts       *service.AccountService
	admin          *service.AdminService
	albums         *service.AlbumService
	storageSvc     *service.StorageService
	policies       *service.PolicyService
	shares         *service.ShareService
	site           *service.SiteService
	billing        *service.BillingService
	maxUploadBytes int64
	logger         *slog.Logger
}

// NewRouter 构建 HTTP 路由并注册中间件和路由。
func NewRouter(d Deps) http.Handler {
	h := &Handler{
		svc:            d.Upload,
		imaging:        d.Imaging,
		accounts:       d.Accounts,
		admin:          d.Admin,
		albums:         d.Albums,
		storageSvc:     d.Storage,
		policies:       d.Policies,
		shares:         d.Shares,
		site:           d.Site,
		billing:        d.Billing,
		maxUploadBytes: int64(d.MaxUploadMB) << 20,
		logger:         d.Logger,
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	if d.TrustProxy {
		// 仅在明确配置为位于可信代理之后时才信任转发头；否则客户端可能
		// 伪造其 IP 以规避限流。
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

			// 公开只读接口：图片广场、公开相册、用户资料与分享访问无需登录。
			r.Group(func(r chi.Router) {
				r.Get("/plaza", h.listPlaza)
				r.Get("/plaza/albums", h.listPublicAlbums)
				r.Get("/albums/{id}", h.getAlbum)
				r.Get("/albums/{id}/images", h.listAlbumImages)
				r.Get("/users/{id}", h.publicProfile)
				r.Get("/shares/{token}", h.shareInfo)
				r.Post("/shares/{token}/access", h.shareAccess)
				r.Get("/announcements", h.listAnnouncements)
				r.Get("/pages/{slug}", h.getPage)
				r.Get("/plans", h.listPlans)
				r.Get("/payment-gateways", h.listPaymentGateways)
				r.Post("/payments/{provider}/callback", h.paymentCallback)
			})

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
				r.Get("/auth/policies", h.authPolicies)
				r.Post("/tokens", h.createToken)
				r.Get("/tokens", h.listTokens)
				r.Delete("/tokens/{id}", h.deleteToken)
				r.Get("/images", h.listImages)
				r.Get("/images/{id}", h.getImage)
				r.Patch("/images/{id}", h.renameImage)
				r.Delete("/images/{id}", h.deleteImage)
				r.Post("/images/batch", h.batchImages)
				r.Get("/albums", h.listAlbums)
				r.Post("/albums", h.createAlbum)
				r.Patch("/albums/{id}", h.updateAlbum)
				r.Delete("/albums/{id}", h.deleteAlbum)
				r.Get("/shares", h.listShares)
				r.Post("/shares", h.createShare)
				r.Delete("/shares/{id}", h.revokeShare)
				r.Post("/reports", h.createReport)
				r.Post("/coupons/validate", h.validateCoupon)
				r.Get("/orders", h.listOrders)
				r.Post("/orders", h.createOrder)
				r.Post("/orders/{id}/pay", h.payOrder)
				r.Get("/tickets", h.listTickets)
				r.Post("/tickets", h.createTicket)
				r.Get("/tickets/{id}", h.getTicket)
				r.Post("/tickets/{id}/reply", h.replyTicket)
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
				r.Get("/admin/storage", h.storageList)
				r.Post("/admin/storage", h.storageCreate)
				r.Get("/admin/storage/{id}", h.storageGet)
				r.Put("/admin/storage/{id}", h.storageUpdate)
				r.Delete("/admin/storage/{id}", h.storageDelete)
				r.Post("/admin/storage/{id}/activate", h.storageActivate)

				r.Get("/admin/role-groups", h.adminListRoleGroups)
				r.Post("/admin/role-groups", h.adminCreateRoleGroup)
				r.Get("/admin/role-groups/{id}", h.adminGetRoleGroup)
				r.Put("/admin/role-groups/{id}", h.adminUpdateRoleGroup)
				r.Delete("/admin/role-groups/{id}", h.adminDeleteRoleGroup)
				r.Post("/admin/role-groups/{id}/policies", h.adminAttachPolicy)
				r.Delete("/admin/role-groups/{id}/policies/{policyID}", h.adminDetachPolicy)

				r.Get("/admin/policies", h.adminListPolicies)
				r.Post("/admin/policies", h.adminCreatePolicy)
				r.Get("/admin/policies/{id}", h.adminGetPolicy)
				r.Put("/admin/policies/{id}", h.adminUpdatePolicy)
				r.Delete("/admin/policies/{id}", h.adminDeletePolicy)

				r.Get("/admin/announcements", h.adminListAnnouncements)
				r.Post("/admin/announcements", h.adminCreateAnnouncement)
				r.Put("/admin/announcements/{id}", h.adminUpdateAnnouncement)
				r.Delete("/admin/announcements/{id}", h.adminDeleteAnnouncement)

				r.Get("/admin/reports", h.adminListReports)
				r.Patch("/admin/reports/{id}", h.adminUpdateReport)

				r.Get("/admin/pages", h.adminListPages)
				r.Post("/admin/pages", h.adminCreatePage)
				r.Put("/admin/pages/{id}", h.adminUpdatePage)
				r.Delete("/admin/pages/{id}", h.adminDeletePage)

				r.Get("/admin/plans", h.adminListPlans)
				r.Post("/admin/plans", h.adminCreatePlan)
				r.Put("/admin/plans/{id}", h.adminUpdatePlan)
				r.Delete("/admin/plans/{id}", h.adminDeletePlan)

				r.Get("/admin/coupons", h.adminListCoupons)
				r.Post("/admin/coupons", h.adminCreateCoupon)
				r.Put("/admin/coupons/{id}", h.adminUpdateCoupon)
				r.Delete("/admin/coupons/{id}", h.adminDeleteCoupon)

				r.Get("/admin/tickets/{id}", h.getTicket)
				r.Patch("/admin/tickets/{id}", h.adminSetTicketStatus)
				r.Post("/admin/tickets/{id}/reply", h.replyTicket)
			})
		})
	})

	if d.Static != nil {
		r.Handle("/*", d.Static)
	}

	return r
}

// securityHeaders 为每个响应应用纵深防御的响应头。
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// trustProxyIP 根据转发头重写 RemoteAddr。它仅在 server.trust_proxy
// 启用时安装，因为否则客户端可以伪造这些头以冒充其 IP。它替代了已废弃的
// chi middleware.RealIP，后者无条件信任这些头。
func trustProxyIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ip := forwardedClientIP(r); ip != "" {
			r.RemoteAddr = ip
		}
		next.ServeHTTP(w, r)
	})
}

// forwardedClientIP 返回可信代理所声明的客户端 IP，优先使用 X-Real-IP
// （通常是代理的对端地址），并回退到 X-Forwarded-For 的第一项。
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

// requestLogger 记录每个请求的方法、路径、状态和耗时。
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

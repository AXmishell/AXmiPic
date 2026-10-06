// Package api 实现 AXmiPic 的 HTTP API。
package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/service"
	"github.com/AXmishell/axmipic/internal/store"
)

// Deps 是构建 API 路由所需的依赖项。
type Deps struct {
	Upload   *service.UploadService
	Imaging  *service.ImagingService
	Accounts *service.AccountService
	Admin    *service.AdminService
	Albums   *service.AlbumService
	Storage  *service.StorageService
	Policies *service.PolicyService
	Shares   *service.ShareService
	Site     *service.SiteService
	Billing  *service.BillingService
	Notify   *service.NotifyService
	Install  *service.InstallService
	// Settings 管理运行时可修改的系统设置（如 SMTP）。
	Settings *service.SettingsService
	// GuestSigner 为匿名访客的签名 cookie 提供签名与校验。
	GuestSigner *auth.GuestSigner
	// Runtime 是实例运行环境信息，供管理端展示（不含密钥）。
	Runtime       RuntimeInfo
	Authenticator *auth.Authenticator
	UploadLimiter *auth.UploadLimiter
	// ImageLimiter 按 IP 对公开图片服务和转换进行限流。
	ImageLimiter *auth.RateLimiter
	// ShareLimiter 按 IP 对分享访问（密码校验）进行限流，防止暴力破解。
	ShareLimiter *auth.RateLimiter
	// PolicyLimiter 为策略驱动的动态限流器；非 nil 且配置了 Policies 时，上传与
	// 图片读取的限流改由角色组策略解析，覆盖 UploadLimiter/ImageLimiter。
	PolicyLimiter *auth.RateLimiter
	// InstallRepo 按安装输入的数据库参数打开仓库。
	InstallRepo func(driver, dsn string) (*store.Repository, error)
	// InstallSeed 在安装过程中创建管理员、角色组与 Guest 账户。
	InstallSeed func(ctx context.Context, repo *store.Repository, in service.InstallInput) error
	// Static 非 nil 时，为未匹配的路由提供单页应用服务。
	Static http.Handler
	// ClientIPResolver 解析客户端真实 IP（可信代理由配置决定）。
	ClientIPResolver *ClientIPResolver
	MaxUploadMB      int
	Logger           *slog.Logger
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
	notify         *service.NotifyService
	install        *service.InstallService
	settings       *service.SettingsService
	guestSigner    *auth.GuestSigner
	runtime        RuntimeInfo
	installRepo    func(driver, dsn string) (*store.Repository, error)
	installSeed    func(ctx context.Context, repo *store.Repository, in service.InstallInput) error
	maxUploadBytes int64
	logger         *slog.Logger
	// clientIP 解析客户端真实 IP，供诊断接口使用。
	clientIP *ClientIPResolver
	// policyRateLimiter 与 policyLimits 用于按角色组策略限流；仅当 Deps.Policies
	// 与 Deps.PolicyLimiter 均非 nil 时启用。
	policyRateLimiter *auth.RateLimiter
	policyLimits      *policyLimits
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
		notify:         d.Notify,
		install:        d.Install,
		settings:       d.Settings,
		guestSigner:    d.GuestSigner,
		runtime:        d.Runtime,
		installRepo:    d.InstallRepo,
		installSeed:    d.InstallSeed,
		maxUploadBytes: int64(d.MaxUploadMB) << 20,
		logger:         d.Logger,
		clientIP:       d.ClientIPResolver,
	}
	if d.PolicyLimiter != nil && d.Policies != nil {
		h.policyRateLimiter = d.PolicyLimiter
		h.policyLimits = newPolicyLimits(d.Policies)
	}

	// 上传/登录限流：优先使用角色组策略驱动的动态限流，否则回退到配置的静态
	// 限流器。
	uploadLimit := d.UploadLimiter.Middleware
	if h.policyRateLimiter != nil {
		uploadLimit = h.uploadRateLimit
	}
	// 公开图片读取限流：同样优先使用策略（匿名访客回退默认）。
	imageLimit := d.ImageLimiter.Middleware
	if h.policyRateLimiter != nil {
		imageLimit = h.imageRateLimit
	}
	// 分享访问（密码校验）按 IP 限流，避免暴力破解。
	shareLimit := d.ShareLimiter.Middleware

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	if d.ClientIPResolver != nil {
		r.Use(d.ClientIPResolver.Middleware())
	}
	r.Use(securityHeaders)
	r.Use(middleware.Recoverer)
	r.Use(requestLogger(d.Logger))
	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/healthz", h.health)
	r.With(imageLimit).Get("/i/*", h.serveImage)
	r.With(imageLimit).Head("/i/*", h.serveImage)

	r.Route("/api/v1", func(r chi.Router) {
		// 安装状态与安装接口无需认证，且在未安装时也应可用。
		r.Get("/install/status", h.installStatus)
		r.Post("/install", h.runInstall)

		r.Group(func(r chi.Router) {
			r.Use(uploadLimit)
			r.Post("/auth/register", h.register)
			r.Post("/auth/register/code", h.sendRegisterCode)
			r.Post("/auth/login", h.login)
			r.Post("/auth/totp/verify", h.verifyTOTPLogin)
			r.Post("/admin/auth/login", h.adminLogin)
			// 找回密码：无需登录，按 IP 限流（服务层再按邮箱限流）。
			r.Post("/auth/password/reset/code", h.sendPasswordResetCode)
			r.Post("/auth/password/reset", h.resetPassword)
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
				r.With(shareLimit).Post("/shares/{token}/access", h.shareAccess)
				r.Get("/announcements", h.listAnnouncements)
				r.Get("/pages/{slug}", h.getPage)
				r.Get("/plans", h.listPlans)
				r.Get("/payment-gateways", h.listPaymentGateways)
				r.Post("/payments/{provider}/callback", h.paymentCallback)
			})

			r.Group(func(r chi.Router) {
				r.Use(h.uploadAuthGuard)
				r.Use(uploadLimit)
				r.Use(h.guestEnsure)
				r.Post("/upload", h.uploadImage)
				r.Post("/upload/presign", h.presignUpload)
				r.Post("/upload/confirm", h.confirmUpload)
			})

			r.Group(func(r chi.Router) {
				r.Use(auth.RequireAuth)
				r.Get("/auth/me", h.me)
				r.Get("/auth/preferences", h.getPreferences)
				r.Put("/auth/preferences", h.updatePreferences)
				r.Get("/auth/security", h.securityInfo)
				r.Post("/auth/totp/setup", h.setupTOTP)
				r.Post("/auth/totp/enable", h.enableTOTP)
				r.Post("/auth/totp/disable", h.disableTOTP)
				r.Post("/auth/email/code", h.sendEmailCode)
				r.Post("/auth/email/verify", h.verifyEmail)
				r.Post("/auth/email/unbind", h.unbindEmail)
				r.Post("/auth/password", h.changePassword)
				r.Get("/auth/policies", h.authPolicies)
				r.Post("/tokens", h.createToken)
				r.Get("/tokens", h.listTokens)
				r.Delete("/tokens/{id}", h.deleteToken)
				r.Get("/images", h.listImages)
				r.Get("/images/{id}", h.getImage)
				r.Patch("/images/{id}", h.renameImage)
				r.Delete("/images/{id}", h.deleteImage)
				r.Post("/images/batch", h.batchImages)
				r.Post("/images/batch-delete", h.batchDeleteImages)
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

				// 管理员核销 manual 等非自动完成的订单。
				r.Post("/admin/orders/{id}/pay", h.payOrder)

				r.Get("/admin/tickets/{id}", h.getTicket)
				r.Patch("/admin/tickets/{id}", h.adminSetTicketStatus)
				r.Post("/admin/tickets/{id}/reply", h.replyTicket)

				r.Get("/admin/notify/channels", h.adminNotifyChannels)
				r.Post("/admin/notify/test", h.adminTestNotify)
				r.Get("/admin/notify/logs", h.adminNotifyLogs)
				r.Get("/admin/notify/smtp", h.adminGetSMTP)
				r.Put("/admin/notify/smtp", h.adminUpdateSMTP)
				r.Get("/admin/settings/{domain}", h.adminGetSettingDomain)
				r.Put("/admin/settings/{domain}", h.adminUpdateSettingDomain)
				r.Get("/admin/client-ip", h.adminClientIPInfo)
				r.Post("/admin/client-ip/preview", h.adminClientIPPreview)
				r.Get("/admin/auth", h.adminGetAuth)
				r.Put("/admin/auth", h.adminUpdateAuth)
				r.Get("/admin/moderation", h.adminGetModeration)
				r.Put("/admin/moderation", h.adminUpdateModeration)
				r.Get("/admin/payment", h.adminGetPayment)
				r.Put("/admin/payment", h.adminUpdatePayment)
				r.Get("/admin/security", h.adminSecurityInfo)
				r.Get("/admin/imaging/drivers", h.adminImagingDrivers)
				r.Get("/admin/runtime", h.adminRuntimeInfo)
				r.Get("/admin/process", h.adminProcessInfo)
			})
		})
	})

	if d.Static != nil {
		r.Handle("/*", d.Static)
	}

	return r
}

// currentAuth 返回当前生效的上传鉴权开关：优先读取可在后台热更新的设置，
// 未配置设置服务时回退到启动时的运行环境信息。
func (h *Handler) currentAuth() service.AuthConfig {
	if h.settings != nil {
		return h.settings.Auth()
	}
	return service.AuthConfig{
		RequireAuth:      h.runtime.RequireAuth,
		AllowGuestUpload: h.runtime.AllowGuestUpload,
	}
}

// uploadAuthGuard 在上传接口按当前开关决定是否强制登录。允许访客上传时放行，
// 以便匿名请求由 Authenticator 附加 Guest 身份后上传；该判断在每次请求时读取，
// 因此后台切换后无需重启即生效。
func (h *Handler) uploadAuthGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := h.currentAuth()
		if cfg.RequireAuth && !cfg.AllowGuestUpload {
			auth.RequireAuth(next).ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
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

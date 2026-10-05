// Command axmipic 运行 AXmiPic 图床服务。
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/AXmishell/axmipic/internal/api"
	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/config"
	"github.com/AXmishell/axmipic/internal/imaging"
	"github.com/AXmishell/axmipic/internal/notify"
	"github.com/AXmishell/axmipic/internal/secret"
	"github.com/AXmishell/axmipic/internal/security"
	"github.com/AXmishell/axmipic/internal/server"
	"github.com/AXmishell/axmipic/internal/service"
	"github.com/AXmishell/axmipic/internal/storage"
	"github.com/AXmishell/axmipic/internal/store"
	"github.com/AXmishell/axmipic/internal/webui"
)

// version 是构建时注入的版本号（例如 v1.0.0），默认值用于本地开发构建。
// 发布流水线通过 -ldflags "-X main.version=..." 覆盖它。
var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("axmipic exited with error", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "configs/config.yaml", "path to the YAML configuration file")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return nil
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	logger := newLogger(cfg.Logging.Level)
	slog.SetDefault(logger)
	logger.Info("AXmiPic starting",
		slog.String("version", version),
		slog.String("platform", runtime.GOOS+"/"+runtime.GOARCH),
	)

	installSvc := service.NewInstallService(cfg.Install.LockFile, cfg.Install.ConfigPath, cfg.Install.Disabled)
	installed := installSvc.IsInstalled()
	if !installed {
		logger.Warn("AXmiPic is not installed yet; visit /install to run the setup wizard")
	}

	repo, err := store.Open(cfg.Database.Driver, cfg.Database.DSN)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := repo.Close(); closeErr != nil {
			logger.Error("failed to close database", slog.Any("error", closeErr))
		}
	}()

	// 既有部署升级：数据库已有管理员但缺少锁文件时，自动写入锁文件。
	if !installed {
		if count, countErr := repo.CountAdmins(context.Background()); countErr == nil {
			if adopted, adoptErr := installSvc.AdoptExisting(count); adoptErr != nil {
				logger.Warn("failed to adopt existing installation", slog.Any("error", adoptErr))
			} else if adopted {
				logger.Info("detected an existing deployment; wrote install lock automatically")
				installed = true
			}
		}
	}

	jwtKey, err := sessionSecret(cfg, logger)
	if err != nil {
		return err
	}

	manager := storage.NewManager()
	cipherKey, err := cipherSecret(cfg, logger, jwtKey)
	if err != nil {
		return err
	}
	cipher, err := secret.New(cipherKey)
	if err != nil {
		return err
	}
	storageSvc := service.NewStorageService(repo, manager, cipher, cfg.Server.BaseURL, cfg.Storage)

	bootstrapCtx, cancelBootstrap := context.WithTimeout(context.Background(), 30*time.Second)
	if err := storageSvc.Bootstrap(bootstrapCtx); err != nil {
		cancelBootstrap()
		return err
	}
	cancelBootstrap()

	uploadSvc := service.NewUploadService(repo, manager, service.UploadPolicy{
		MaxSizeBytes:     int64(cfg.Upload.MaxSizeMB) << 20,
		AllowedMIMETypes: cfg.Upload.AllowedMIMETypes,
		PresignExpiry:    storagePresignExpiry(cfg),
	})

	processor, processorName, err := imaging.ResolveWithFallback(cfg.Processing.Driver)
	if err != nil {
		return err
	}
	capabilities := processor.Capabilities()
	logger.Info("imaging processor ready",
		slog.String("requested_driver", cfg.Processing.Driver),
		slog.String("processor", processorName),
		slog.Any("formats", capabilities.OutputFormats),
	)
	imagingSvc := service.NewImagingService(manager, processor, service.ProcessingPolicy{
		Enabled:        cfg.Processing.Enabled,
		MaxWidth:       cfg.Processing.MaxWidth,
		MaxHeight:      cfg.Processing.MaxHeight,
		DefaultQuality: cfg.Processing.DefaultQuality,
		AllowedFormats: processingFormats(cfg.Processing.AllowedFormats),
		AllowEnlarge:   cfg.Processing.AllowEnlarge,
		AllowEffects:   cfg.Processing.AllowEffects,
		AllowWatermark: cfg.Processing.AllowWatermark,
		WatermarkText:  cfg.Processing.WatermarkText,
	})

	issuer := auth.NewSessionIssuer(jwtKey, time.Duration(cfg.Auth.SessionTTLHours)*time.Hour)
	accounts := service.NewAccountService(repo, issuer, cfg.Auth.AllowRegistration, int64(cfg.Auth.DefaultQuotaMB)<<20)
	accounts.SetCipher(cipher)
	adminSvc := service.NewAdminService(repo, cfg.Storage.Driver, processor)
	albumSvc := service.NewAlbumService(repo)

	policies := service.NewPolicyService(repo, service.PolicyDefaults{
		QuotaBytes:       int64(cfg.Auth.DefaultQuotaMB) << 20,
		UploadMaxBytes:   int64(cfg.Upload.MaxSizeMB) << 20,
		AllowedMIMETypes: cfg.Upload.AllowedMIMETypes,
		Rate: service.RateSettings{
			UploadPerMinute: cfg.Limits.UploadPerMinute,
			UploadBurst:     cfg.Limits.UploadBurst,
			ImagePerMinute:  cfg.Limits.ImagePerMinute,
			ImageBurst:      cfg.Limits.ImageBurst,
		},
		Processing: service.ProcessingSettings{
			Enabled:        cfg.Processing.Enabled,
			MaxWidth:       cfg.Processing.MaxWidth,
			MaxHeight:      cfg.Processing.MaxHeight,
			DefaultQuality: cfg.Processing.DefaultQuality,
			AllowedFormats: cfg.Processing.AllowedFormats,
		},
	})

	bootstrapCtx, cancelBootstrap = context.WithTimeout(context.Background(), 30*time.Second)
	if installed {
		err = accounts.EnsureBootstrapAdmin(bootstrapCtx, cfg.Auth.BootstrapAdmin)
		if err == nil {
			err = policies.SeedDefaults(bootstrapCtx)
		}
		if err == nil {
			err = seedGuest(bootstrapCtx, policies, accounts, cfg)
		}
	}
	cancelBootstrap()
	if err != nil {
		return err
	}

	if installed {
		if adopted, adoptErr := policies.AdoptUnassigned(context.Background()); adoptErr != nil {
			logger.Warn("failed to assign existing users to the default role group", slog.Any("error", adoptErr))
		} else if adopted > 0 {
			logger.Info("assigned existing users to the default role group", slog.Int64("count", adopted))
		}
	}

	uploadSvc.SetPolicyResolver(policies)
	accounts.SetPolicyService(policies)
	adminSvc.SetPolicyService(policies)

	installSeed := func(ctx context.Context, target *store.Repository, in service.InstallInput) error {
		return runInstallSeed(ctx, target, policies, in, cfg, logger)
	}

	switch cfg.Security.Scanner {
	case "builtin":
		uploadSvc.SetScanner(security.NewBlockingScanner(cfg.Upload.AllowedMIMETypes))
	default:
		// none：不安装扫描器，直接放行。
	}

	// 通知渠道：短信与邮件。未配置服务商时回退到日志渠道。
	var smsSender notify.Sender = notify.NewLogSender("sms", logger)
	if cfg.SMS.Enabled {
		httpSender, err := notify.NewHTTPSSender(notify.HTTPOptions{
			Name:     cfg.SMS.Provider,
			Endpoint: cfg.SMS.Endpoint,
			Method:   cfg.SMS.Method,
		})
		if err != nil {
			return fmt.Errorf("main: sms sender: %w", err)
		}
		smsSender = httpSender
		logger.Info("sms notification channel enabled")
	}
	var emailSender notify.Sender = notify.NewLogSender("email", logger)
	if cfg.Email.Enabled {
		smtpSender, err := notify.NewSMTPSender(notify.SMTPOptions{
			Host:     cfg.Email.Host,
			Port:     cfg.Email.Port,
			Username: cfg.Email.Username,
			Password: cfg.Email.Password,
			From:     cfg.Email.From,
			UseTLS:   cfg.Email.UseTLS,
		})
		if err != nil {
			return fmt.Errorf("main: email sender: %w", err)
		}
		emailSender = smtpSender
		logger.Info("email notification channel enabled")
	}
	notifySvc := service.NewNotifyService(smsSender, emailSender)
	accounts.SetNotifyService(notifySvc)
	settingsSvc := service.NewSettingsService(repo, cipher, notifySvc, logger, service.SMTPConfig{
		Enabled:  cfg.Email.Enabled,
		Host:     cfg.Email.Host,
		Port:     cfg.Email.Port,
		Username: cfg.Email.Username,
		Password: cfg.Email.Password,
		From:     cfg.Email.From,
		UseTLS:   cfg.Email.UseTLS,
	})
	settingsCtx, cancelSettings := context.WithTimeout(context.Background(), 10*time.Second)
	err = settingsSvc.Bootstrap(settingsCtx)
	cancelSettings()
	if err != nil {
		return err
	}
	shareSvc := service.NewShareService(repo, cfg.Server.BaseURL)
	siteSvc := service.NewSiteService(repo)

	runtimeInfo := api.RuntimeInfo{
		SiteName:          "AXmiPic",
		Version:           version,
		BaseURL:           cfg.Server.BaseURL,
		DatabaseDriver:    normalizeDBDriver(cfg.Database.Driver),
		StorageDriver:     cfg.Storage.Driver,
		Processor:         processorName,
		Formats:           formatNames(capabilities.OutputFormats),
		AllowRegistration: cfg.Auth.AllowRegistration,
		RequireAuth:       cfg.Auth.RequireAuth,
		AllowGuestUpload:  cfg.Auth.AllowGuestUpload,
		GuestQuotaMB:      cfg.Auth.GuestQuotaMB,
		GuestUploadMaxMB:  cfg.Auth.GuestUploadMaxMB,
		DefaultQuotaMB:    cfg.Auth.DefaultQuotaMB,
		UploadMaxMB:       cfg.Upload.MaxSizeMB,
		TrustProxy:        cfg.Server.TrustProxy,
		SessionTTLHours:   cfg.Auth.SessionTTLHours,
		InstallLockFile:   cfg.Install.LockFile,
		GoVersion:         runtime.Version(),
		Platform:          runtime.GOOS + "/" + runtime.GOARCH,
	}

	gateways, err := service.BuildGateways(cfg.Server.BaseURL, cfg.Payment, logger)
	if err != nil {
		return fmt.Errorf("main: payment gateways: %w", err)
	}
	billingSvc := service.NewBillingService(repo, cfg.Server.BaseURL, gateways, cfg.Payment.DefaultGateway)
	billingSvc.SetPolicyService(policies)

	// 支付设置：以配置文件为兜底，首次启动写入数据库并支持后台热更新。
	settingsSvc.SetPaymentDefaults(cfg.Payment)
	settingsSvc.SetPaymentApplier(func(paymentCfg config.PaymentConfig) error {
		built, buildErr := service.BuildGateways(cfg.Server.BaseURL, paymentCfg, logger)
		if buildErr != nil {
			return buildErr
		}
		billingSvc.ApplyPaymentConfig(paymentCfg.DefaultGateway, built)
		return nil
	})
	paymentBootstrapCtx, cancelPayment := context.WithTimeout(context.Background(), 10*time.Second)
	err = settingsSvc.BootstrapPayment(paymentBootstrapCtx)
	cancelPayment()
	if err != nil {
		return err
	}
	if err := settingsSvc.ApplyStoredPayment(); err != nil {
		return fmt.Errorf("main: apply payment settings: %w", err)
	}

	authenticator := auth.NewAuthenticator(repo, issuer)
	// 允许访客上传时，为匿名请求附加内置 Guest 账户身份，使访客上传计入
	// Guest 角色的存储配额。
	if installed && cfg.Auth.AllowGuestUpload {
		if guestID := accounts.GuestID(context.Background()); guestID != "" {
			authenticator.SetGuestPrincipal(&auth.Principal{
				UserID:   guestID,
				Username: service.GuestUsername,
				Role:     auth.RoleUser,
				Guest:    true,
			})
		}
	}

	router := api.NewRouter(api.Deps{
		Upload:        uploadSvc,
		Imaging:       imagingSvc,
		Accounts:      accounts,
		Admin:         adminSvc,
		Albums:        albumSvc,
		Storage:       storageSvc,
		Policies:      policies,
		Shares:        shareSvc,
		Site:          siteSvc,
		Billing:       billingSvc,
		Notify:        notifySvc,
		Install:       installSvc,
		Settings:      settingsSvc,
		Runtime:       runtimeInfo,
		InstallRepo:   store.Open,
		InstallSeed:   installSeed,
		Authenticator: authenticator,
		// 上传与图片读取限流按角色组策略动态解析（PolicyLimiter + Policies）。
		PolicyLimiter:    auth.NewDynamicRateLimiter(),
		RequireAuth:      cfg.Auth.RequireAuth,
		AllowGuestUpload: cfg.Auth.AllowGuestUpload,
		TrustProxy:       cfg.Server.TrustProxy,
		MaxUploadMB:      cfg.Upload.MaxSizeMB,
		Static:           webui.Handler(),
		Logger:           logger,
	})
	srv := server.New(cfg, logger, router)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go runPendingUploadJanitor(ctx, uploadSvc, logger)
	go runExpiredPlanJanitor(ctx, billingSvc, logger)

	return srv.Run(ctx)
}

// sessionSecret 返回会话 JWT 的签名密钥。优先使用配置的 auth.jwt_secret；
// 未配置时读取或生成一个持久化密钥，使会话在重启后依然有效。
func sessionSecret(cfg config.Config, logger *slog.Logger) ([]byte, error) {
	if key := strings.TrimSpace(cfg.Auth.JWTSecret); key != "" {
		return []byte(key), nil
	}
	return persistedSecret(cfg, logger)
}

// cipherSecret 返回加密存储后端密钥所用的主密钥。优先使用独立的
// auth.encryption_key，其次回退到会话密钥（保持对既有部署的兼容）。
func cipherSecret(cfg config.Config, logger *slog.Logger, sessionKey []byte) ([]byte, error) {
	if key := strings.TrimSpace(cfg.Auth.EncryptionKey); key != "" {
		return []byte(key), nil
	}
	return sessionKey, nil
}

// persistedSecret 从磁盘读取一个稳定的自动生成主密钥；不存在时生成并写入。
// 它仅在 auth.jwt_secret 留空时使用，目的是避免每次重启都更换加密主密钥而
// 导致已加密入库的存储密钥无法解密。
func persistedSecret(cfg config.Config, logger *slog.Logger) ([]byte, error) {
	path := secretKeyPath(cfg)
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if key := strings.TrimSpace(string(data)); key != "" {
			return []byte(key), nil
		}
	case !errors.Is(err, os.ErrNotExist):
		return nil, fmt.Errorf("main: read secret key %q: %w", path, err)
	}

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("main: generate secret key: %w", err)
	}
	key := base64.RawURLEncoding.EncodeToString(buf)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("main: create secret key directory for %q: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(key), 0o600); err != nil {
		return nil, fmt.Errorf("main: persist secret key %q: %w", path, err)
	}
	logger.Warn("auth.jwt_secret is empty; generated and persisted a secret key",
		slog.String("path", path),
		slog.String("hint", "set auth.jwt_secret or auth.encryption_key for multi-instance deployments"),
	)
	return []byte(key), nil
}

// secretKeyPath 返回自动生成主密钥的落盘位置：SQLite 场景放在数据库同目录，
// 否则放在 ./data 下。
func secretKeyPath(cfg config.Config) string {
	dir := "data"
	dsn := strings.TrimSpace(cfg.Database.DSN)
	driver := strings.ToLower(strings.TrimSpace(cfg.Database.Driver))
	if (driver == "" || driver == "sqlite") && dsn != "" && dsn != ":memory:" && !strings.HasPrefix(dsn, "file:") {
		if d := filepath.Dir(dsn); d != "" {
			dir = d
		}
	}
	return filepath.Join(dir, ".axmipic-key")
}

// processingFormats 将配置的格式名称转换为 imaging 格式，跳过未知名称
// （这些名称已在配置校验阶段被拒绝）。
func processingFormats(names []string) []imaging.Format {
	formats := make([]imaging.Format, 0, len(names))
	for _, name := range names {
		if f, ok := imaging.ParseFormat(name); ok {
			formats = append(formats, f)
		}
	}
	return formats
}

// formatNames 将处理器能力中的格式转换为可读名称列表。
func formatNames(formats []imaging.Format) []string {
	names := make([]string, 0, len(formats))
	for _, f := range formats {
		names = append(names, string(f))
	}
	return names
}

// normalizeDBDriver 返回规范化的数据库驱动名称（空值视为 sqlite）。
func normalizeDBDriver(driver string) string {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "", "sqlite":
		return "sqlite"
	case "postgres", "postgresql", "pgx":
		return "postgres"
	default:
		return driver
	}
}

// seedGuest 确保 Guest 访客角色组与 Guest 账户存在，并按配置决定访客是否可上传。
func seedGuest(ctx context.Context, policies *service.PolicyService, accounts *service.AccountService, cfg config.Config) error {
	guestQuota := int64(cfg.Auth.GuestQuotaMB) << 20
	guestUpload := int64(cfg.Auth.GuestUploadMaxMB) << 20
	if guestUpload <= 0 {
		guestUpload = int64(cfg.Upload.MaxSizeMB) << 20
	}
	group, err := policies.SeedGuestRoleGroup(ctx, guestQuota, guestUpload)
	if err != nil {
		return err
	}
	if _, err := accounts.EnsureGuestAccount(ctx, group.ID, guestQuota); err != nil {
		return err
	}
	return nil
}

// runInstallSeed 在安装向导中针对新建仓库完成初始化：创建管理员、播种默认与
// Guest 角色组，并创建 Guest 账户。
func runInstallSeed(ctx context.Context, target *store.Repository, policies *service.PolicyService, in service.InstallInput, cfg config.Config, logger *slog.Logger) error {
	issuer, err := newInstallIssuer(cfg)
	if err != nil {
		return err
	}
	accounts := service.NewAccountService(target, issuer, in.AllowRegistration, int64(cfg.Auth.DefaultQuotaMB)<<20)
	accounts.SetPolicyService(policies)

	// 基于向导输入构造策略服务（指向同一仓库）。
	policySvc := service.NewPolicyService(target, policyDefaultsFor(cfg))
	if err := policySvc.SeedDefaults(ctx); err != nil {
		return err
	}

	if _, err := accounts.RegisterAdmin(ctx, in.AdminUsername, in.AdminPassword); err != nil {
		return err
	}

	guestUpload := int64(cfg.Auth.GuestUploadMaxMB) << 20
	if guestUpload <= 0 {
		guestUpload = int64(cfg.Upload.MaxSizeMB) << 20
	}
	group, err := policySvc.SeedGuestRoleGroup(ctx, int64(cfg.Auth.GuestQuotaMB)<<20, guestUpload)
	if err != nil {
		return err
	}
	if _, err := accounts.EnsureGuestAccount(ctx, group.ID, int64(cfg.Auth.GuestQuotaMB)<<20); err != nil {
		return err
	}
	logger.Info("installation initialized",
		slog.String("admin", in.AdminUsername),
		slog.String("guest_group", group.ID),
	)
	return nil
}

// newInstallIssuer 为安装过程构造一个临时的会话签发器（用于创建管理员账户）。
func newInstallIssuer(cfg config.Config) (*auth.SessionIssuer, error) {
	key := strings.TrimSpace(cfg.Auth.JWTSecret)
	if key == "" {
		key = "axmipic-install-temporary-secret"
	}
	return auth.NewSessionIssuer([]byte(key), time.Duration(cfg.Auth.SessionTTLHours)*time.Hour), nil
}

// policyDefaultsFor 从配置构造策略默认值。
func policyDefaultsFor(cfg config.Config) service.PolicyDefaults {
	return service.PolicyDefaults{
		QuotaBytes:       int64(cfg.Auth.DefaultQuotaMB) << 20,
		UploadMaxBytes:   int64(cfg.Upload.MaxSizeMB) << 20,
		AllowedMIMETypes: cfg.Upload.AllowedMIMETypes,
		Rate: service.RateSettings{
			UploadPerMinute: cfg.Limits.UploadPerMinute,
			UploadBurst:     cfg.Limits.UploadBurst,
			ImagePerMinute:  cfg.Limits.ImagePerMinute,
			ImageBurst:      cfg.Limits.ImageBurst,
		},
		Processing: service.ProcessingSettings{
			Enabled:        cfg.Processing.Enabled,
			MaxWidth:       cfg.Processing.MaxWidth,
			MaxHeight:      cfg.Processing.MaxHeight,
			DefaultQuality: cfg.Processing.DefaultQuality,
			AllowedFormats: cfg.Processing.AllowedFormats,
		},
	}
}

// runPendingUploadJanitor 定期删除过期的待处理上传及其遗留的孤立对象。
// 它运行直到 ctx 被取消。
func runPendingUploadJanitor(ctx context.Context, svc *service.UploadService, logger *slog.Logger) {
	const interval = time.Hour
	cleanup := func() {
		cleanupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		removed, err := svc.CleanupExpired(cleanupCtx, time.Now())
		if err != nil {
			logger.Warn("pending upload cleanup failed", slog.Any("error", err))
			return
		}
		if removed > 0 {
			logger.Info("cleaned up expired pending uploads", slog.Int("count", removed))
		}
	}

	cleanup()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanup()
		}
	}
}

// runExpiredPlanJanitor 定期把已到期套餐的客户回退到默认角色组与配额。
// 它运行直到 ctx 被取消。
func runExpiredPlanJanitor(ctx context.Context, svc *service.BillingService, logger *slog.Logger) {
	const interval = time.Hour
	cleanup := func() {
		cleanupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		handled, err := svc.ExpirePlans(cleanupCtx, time.Now())
		if err != nil {
			logger.Warn("expired plan cleanup failed", slog.Any("error", err))
			return
		}
		if handled > 0 {
			logger.Info("reverted expired plan customers", slog.Int("count", handled))
		}
	}

	cleanup()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanup()
		}
	}
}

// storagePresignExpiry 返回当前存储驱动的预签名上传有效期。
func storagePresignExpiry(cfg config.Config) time.Duration {
	seconds := 0
	switch cfg.Storage.Driver {
	case "s3":
		seconds = cfg.Storage.S3.PresignExpirySec
	case "qiniu":
		seconds = cfg.Storage.Qiniu.PresignExpirySec
	}
	if seconds < 1 {
		seconds = 900
	}
	return time.Duration(seconds) * time.Second
}

// newLogger 在配置的级别上构建结构化 JSON 日志器。
func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}

// Command axmipic 运行 AXmiPic 图床服务。
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AXmishell/axmipic/internal/api"
	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/config"
	"github.com/AXmishell/axmipic/internal/imaging"
	"github.com/AXmishell/axmipic/internal/server"
	"github.com/AXmishell/axmipic/internal/service"
	"github.com/AXmishell/axmipic/internal/storage"
	"github.com/AXmishell/axmipic/internal/store"
	"github.com/AXmishell/axmipic/internal/webui"
)

func main() {
	if err := run(); err != nil {
		slog.Error("axmipic exited with error", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "configs/config.yaml", "path to the YAML configuration file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	logger := newLogger(cfg.Logging.Level)
	slog.SetDefault(logger)

	repo, err := store.Open(cfg.Database.Driver, cfg.Database.DSN)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := repo.Close(); closeErr != nil {
			logger.Error("failed to close database", slog.Any("error", closeErr))
		}
	}()

	storeBackend, err := storage.NewFromConfig(cfg.Storage, cfg.Server.BaseURL)
	if err != nil {
		return err
	}

	uploadSvc := service.NewUploadService(repo, storeBackend, service.UploadPolicy{
		MaxSizeBytes:     int64(cfg.Upload.MaxSizeMB) << 20,
		AllowedMIMETypes: cfg.Upload.AllowedMIMETypes,
		PresignExpiry:    storagePresignExpiry(cfg),
	})

	processor := imaging.Default()
	capabilities := processor.Capabilities()
	logger.Info("imaging processor ready",
		slog.String("processor", capabilities.Name),
		slog.Any("formats", capabilities.OutputFormats),
	)
	imagingSvc := service.NewImagingService(storeBackend, processor, service.ProcessingPolicy{
		Enabled:        cfg.Processing.Enabled,
		MaxWidth:       cfg.Processing.MaxWidth,
		MaxHeight:      cfg.Processing.MaxHeight,
		DefaultQuality: cfg.Processing.DefaultQuality,
		AllowedFormats: processingFormats(cfg.Processing.AllowedFormats),
	})

	secret, err := jwtSecret(cfg.Auth.JWTSecret, logger)
	if err != nil {
		return err
	}
	issuer := auth.NewSessionIssuer(secret, time.Duration(cfg.Auth.SessionTTLHours)*time.Hour)
	accounts := service.NewAccountService(repo, issuer, cfg.Auth.AllowRegistration, int64(cfg.Auth.DefaultQuotaMB)<<20)
	adminSvc := service.NewAdminService(repo, cfg.Storage.Driver, processor)

	bootstrapCtx, cancelBootstrap := context.WithTimeout(context.Background(), 30*time.Second)
	err = accounts.EnsureBootstrapAdmin(bootstrapCtx, cfg.Auth.BootstrapAdmin)
	cancelBootstrap()
	if err != nil {
		return err
	}

	router := api.NewRouter(api.Deps{
		Upload:        uploadSvc,
		Imaging:       imagingSvc,
		Accounts:      accounts,
		Admin:         adminSvc,
		Storage:       storeBackend,
		Authenticator: auth.NewAuthenticator(repo, issuer),
		UploadLimiter: &auth.UploadLimiter{
			User:  auth.NewRateLimiter(cfg.Limits.UploadPerMinute, cfg.Limits.UploadBurst),
			Guest: auth.NewRateLimiter(cfg.Limits.GuestPerMinute, cfg.Limits.GuestBurst),
		},
		ImageLimiter: auth.NewRateLimiter(cfg.Limits.ImagePerMinute, cfg.Limits.ImageBurst),
		RequireAuth:  cfg.Auth.RequireAuth,
		TrustProxy:   cfg.Server.TrustProxy,
		MaxUploadMB:  cfg.Upload.MaxSizeMB,
		Static:       webui.Handler(),
		Logger:       logger,
	})
	srv := server.New(cfg, logger, router)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go runPendingUploadJanitor(ctx, uploadSvc, logger)

	return srv.Run(ctx)
}

// jwtSecret 返回配置的会话密钥，当未配置时生成一个随机密钥（并发出警告）。
// 随机密钥会使会话在重启后失效，不适用于多实例部署。
func jwtSecret(configured string, logger *slog.Logger) ([]byte, error) {
	if configured != "" {
		return []byte(configured), nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("main: generate jwt secret: %w", err)
	}
	logger.Warn("auth.jwt_secret is empty; generated a random secret (sessions will not survive restart)")
	return []byte(base64.RawURLEncoding.EncodeToString(buf)), nil
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

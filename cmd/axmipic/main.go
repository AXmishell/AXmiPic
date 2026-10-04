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
	"strings"
	"syscall"
	"time"

	"github.com/AXmishell/axmipic/internal/api"
	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/config"
	"github.com/AXmishell/axmipic/internal/imaging"
	"github.com/AXmishell/axmipic/internal/secret"
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

	processor := imaging.Default()
	capabilities := processor.Capabilities()
	logger.Info("imaging processor ready",
		slog.String("processor", capabilities.Name),
		slog.Any("formats", capabilities.OutputFormats),
	)
	imagingSvc := service.NewImagingService(manager, processor, service.ProcessingPolicy{
		Enabled:        cfg.Processing.Enabled,
		MaxWidth:       cfg.Processing.MaxWidth,
		MaxHeight:      cfg.Processing.MaxHeight,
		DefaultQuality: cfg.Processing.DefaultQuality,
		AllowedFormats: processingFormats(cfg.Processing.AllowedFormats),
	})

	issuer := auth.NewSessionIssuer(jwtKey, time.Duration(cfg.Auth.SessionTTLHours)*time.Hour)
	accounts := service.NewAccountService(repo, issuer, cfg.Auth.AllowRegistration, int64(cfg.Auth.DefaultQuotaMB)<<20)
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
	err = accounts.EnsureBootstrapAdmin(bootstrapCtx, cfg.Auth.BootstrapAdmin)
	if err == nil {
		err = policies.SeedDefaults(bootstrapCtx)
	}
	cancelBootstrap()
	if err != nil {
		return err
	}

	if adopted, adoptErr := policies.AdoptUnassigned(context.Background()); adoptErr != nil {
		logger.Warn("failed to assign existing users to the default role group", slog.Any("error", adoptErr))
	} else if adopted > 0 {
		logger.Info("assigned existing users to the default role group", slog.Int64("count", adopted))
	}

	uploadSvc.SetPolicyResolver(policies)
	accounts.SetPolicyService(policies)
	adminSvc.SetPolicyService(policies)
	shareSvc := service.NewShareService(repo, cfg.Server.BaseURL)
	siteSvc := service.NewSiteService(repo)

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

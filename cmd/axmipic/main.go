// Command axmipic runs the AXmiPic image hosting service.
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

	"github.com/axmipic/axmipic/internal/api"
	"github.com/axmipic/axmipic/internal/auth"
	"github.com/axmipic/axmipic/internal/config"
	"github.com/axmipic/axmipic/internal/imaging"
	"github.com/axmipic/axmipic/internal/server"
	"github.com/axmipic/axmipic/internal/service"
	"github.com/axmipic/axmipic/internal/storage"
	"github.com/axmipic/axmipic/internal/store"
	"github.com/axmipic/axmipic/internal/webui"
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

	repo, err := store.Open(cfg.Database.DSN)
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
		RequireAuth: cfg.Auth.RequireAuth,
		MaxUploadMB: cfg.Upload.MaxSizeMB,
		Static:      webui.Handler(),
		Logger:      logger,
	})
	srv := server.New(cfg, logger, router)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return srv.Run(ctx)
}

// jwtSecret returns the configured session secret, generating a random one (and
// warning) when none is configured. A random secret invalidates sessions on
// restart and is unsuitable for multi-instance deployments.
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

// processingFormats converts configured format names into imaging formats,
// skipping unknown names (already rejected during config validation).
func processingFormats(names []string) []imaging.Format {
	formats := make([]imaging.Format, 0, len(names))
	for _, name := range names {
		if f, ok := imaging.ParseFormat(name); ok {
			formats = append(formats, f)
		}
	}
	return formats
}

// storagePresignExpiry returns the presigned-upload lifetime for the active
// storage driver.
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

// newLogger builds a structured JSON logger at the configured level.
func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}

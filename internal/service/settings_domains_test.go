package service_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/AXmishell/axmipic/internal/secret"
	"github.com/AXmishell/axmipic/internal/service"
)

// TestSettingDomainBootstrapUpdateAndPersist 验证设置域的播种、热应用、校验与持久化。
func TestSettingDomainBootstrapUpdateAndPersist(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	cipher, err := secret.New([]byte("test-encryption-key-0123456789"))
	if err != nil {
		t.Fatalf("secret.New: %v", err)
	}
	notifySvc := service.NewNotifyService(nil, nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := service.NewSettingsService(repo, cipher, notifySvc, logger, service.SMTPConfig{})

	var applied []service.UploadSettings
	svc.RegisterDomain(service.NewSettingDomain(repo, "upload",
		service.UploadSettings{MaxSizeMB: 20, AllowedMIMETypes: []string{"image/png"}},
		service.ValidateUploadSettings,
		func(v service.UploadSettings) { applied = append(applied, v) },
	))

	if err := svc.BootstrapDomains(ctx); err != nil {
		t.Fatalf("BootstrapDomains: %v", err)
	}
	if len(applied) != 1 || applied[0].MaxSizeMB != 20 {
		t.Fatalf("applier not invoked with defaults: %+v", applied)
	}
	got, ok := service.DomainValue[service.UploadSettings](svc, "upload")
	if !ok || got.MaxSizeMB != 20 {
		t.Fatalf("default value = %+v, ok=%v", got, ok)
	}

	domain, _ := svc.Domain("upload")
	if _, err := domain.Update(ctx, []byte(`{"max_size_mb":5,"allowed_mime_types":["image/jpeg"]}`)); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ = service.DomainValue[service.UploadSettings](svc, "upload")
	if got.MaxSizeMB != 5 || len(got.AllowedMIMETypes) != 1 || got.AllowedMIMETypes[0] != "image/jpeg" {
		t.Fatalf("updated value = %+v", got)
	}
	if len(applied) != 2 {
		t.Fatalf("applier should run on update, got %d calls", len(applied))
	}

	// 非法值被拒绝。
	if _, err := domain.Update(ctx, []byte(`{"max_size_mb":0,"allowed_mime_types":[]}`)); !errors.Is(err, service.ErrSettingsConfig) {
		t.Fatalf("invalid update error = %v, want ErrSettingsConfig", err)
	}
	if _, err := domain.Update(ctx, []byte(`not-json`)); !errors.Is(err, service.ErrSettingsConfig) {
		t.Fatalf("invalid JSON error = %v, want ErrSettingsConfig", err)
	}

	// 新实例从同一个数据库加载（而非默认值）。
	svc2 := service.NewSettingsService(repo, cipher, notifySvc, logger, service.SMTPConfig{})
	svc2.RegisterDomain(service.NewSettingDomain(repo, "upload",
		service.UploadSettings{MaxSizeMB: 99}, nil, nil))
	if err := svc2.BootstrapDomains(ctx); err != nil {
		t.Fatalf("BootstrapDomains (reload): %v", err)
	}
	reloaded, _ := service.DomainValue[service.UploadSettings](svc2, "upload")
	if reloaded.MaxSizeMB != 5 {
		t.Fatalf("reloaded value = %+v, want persisted 5", reloaded)
	}
}

func TestValidateImagingSettingsRejectsUnknownFormat(t *testing.T) {
	err := service.ValidateImagingSettings(service.ImagingSettings{
		Enabled:        true,
		MaxWidth:       100,
		MaxHeight:      100,
		DefaultQuality: 80,
		AllowedFormats: []string{"bmp"},
	})
	if !errors.Is(err, service.ErrSettingsConfig) {
		t.Fatalf("error = %v, want ErrSettingsConfig", err)
	}
}

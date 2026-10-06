package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/AXmishell/axmipic/internal/config"
	"github.com/AXmishell/axmipic/internal/notify"
	"github.com/AXmishell/axmipic/internal/secret"
	"github.com/AXmishell/axmipic/internal/service"
)

func TestSMTPUpdateAndHotReload(t *testing.T) {
	repo := newRepo(t)
	cipher, err := secret.New([]byte("test-encryption-key"))
	if err != nil {
		t.Fatalf("secret.New: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	notifySvc := service.NewNotifyService(notify.NewMockSender("sms"), notify.NewLogSender("email", logger))
	svc := service.NewSettingsService(repo, cipher, notifySvc, logger, service.SMTPConfig{Enabled: false})

	ctx := context.Background()

	// 初始为未启用。
	if got := svc.SMTP(); got.Enabled || got.PasswordSet {
		t.Fatalf("initial smtp = %+v", got)
	}

	// 保存一份启用配置。
	updated, err := svc.UpdateSMTP(ctx, service.SMTPInput{
		Enabled:  true,
		Host:     "smtp.example.com",
		Port:     465,
		Username: "user@example.com",
		Password: "secret-pass",
		From:     "noreply@example.com",
		UseTLS:   true,
	})
	if err != nil {
		t.Fatalf("UpdateSMTP: %v", err)
	}
	if !updated.Enabled || !updated.PasswordSet {
		t.Fatalf("updated smtp = %+v", updated)
	}
	if updated.Port != 465 || updated.UseTLS != true {
		t.Fatalf("updated smtp fields = %+v", updated)
	}

	// 邮件渠道应即时切换为 smtp。
	if _, email := notifySvc.Channels(); email != "smtp" {
		t.Fatalf("email channel = %q, want smtp", email)
	}

	// 落库的密码必须是密文，不能出现明文。
	raw, err := repo.GetSetting(ctx, "smtp")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if strings.Contains(raw, "secret-pass") {
		t.Fatalf("stored smtp leaks plaintext password: %s", raw)
	}
	var stored map[string]any
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		t.Fatalf("unmarshal stored smtp: %v", err)
	}
	if stored["password"] == "" {
		t.Fatalf("stored smtp password ciphertext is empty")
	}

	// 新的服务实例应能从数据库恢复配置并解密密码。
	reloaded := service.NewSettingsService(repo, cipher, service.NewNotifyService(nil, nil), logger, service.SMTPConfig{})
	if err := reloaded.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if got := reloaded.SMTP(); !got.Enabled || !got.PasswordSet {
		t.Fatalf("reloaded smtp = %+v", got)
	}

	// 空密码表示保持不变。
	again, err := svc.UpdateSMTP(ctx, service.SMTPInput{
		Enabled:  true,
		Host:     "smtp.example.com",
		Port:     587,
		Username: "user@example.com",
		From:     "noreply@example.com",
	})
	if err != nil {
		t.Fatalf("UpdateSMTP without password: %v", err)
	}
	if !again.PasswordSet {
		t.Fatalf("password should be preserved: %+v", again)
	}
}

func TestSMTPValidation(t *testing.T) {
	repo := newRepo(t)
	cipher, _ := secret.New([]byte("test-encryption-key"))
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := service.NewSettingsService(repo, cipher, service.NewNotifyService(nil, nil), logger, service.SMTPConfig{})

	if _, err := svc.UpdateSMTP(context.Background(), service.SMTPInput{Enabled: true, Host: ""}); !errors.Is(err, service.ErrSettingsConfig) {
		t.Fatalf("missing host err = %v, want ErrSettingsConfig", err)
	}
}

func TestPaymentUpdateAndHotReload(t *testing.T) {
	repo := newRepo(t)
	cipher, _ := secret.New([]byte("test-encryption-key"))
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := service.NewSettingsService(repo, cipher, service.NewNotifyService(nil, nil), logger, service.SMTPConfig{})
	ctx := context.Background()

	var applied config.PaymentConfig
	svc.SetPaymentApplier(func(cfg config.PaymentConfig) error {
		applied = cfg
		return nil
	})
	if err := svc.BootstrapPayment(ctx); err != nil {
		t.Fatalf("BootstrapPayment: %v", err)
	}

	updated, err := svc.UpdatePayment(ctx, service.PaymentSettingsInput{
		DefaultGateway: "epay",
		Epay: service.EpaySettingsInput{
			Enabled: true, PID: "1001", Key: "secret-key", GatewayURL: "https://pay.example.com",
		},
	})
	if err != nil {
		t.Fatalf("UpdatePayment: %v", err)
	}
	if updated.DefaultGateway != "epay" || !updated.Epay.Enabled || !updated.Epay.KeySet {
		t.Fatalf("updated = %+v", updated)
	}
	if !applied.Epay.Enabled || applied.Epay.PID != "1001" {
		t.Fatalf("applier not applied with new config: %+v", applied)
	}

	// 落库的密钥必须是密文。
	raw, err := repo.GetSetting(ctx, "payment")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if strings.Contains(raw, "secret-key") {
		t.Fatalf("stored payment leaks plaintext key: %s", raw)
	}

	// 空密钥表示保持不变。
	again, err := svc.UpdatePayment(ctx, service.PaymentSettingsInput{
		DefaultGateway: "epay",
		Epay:           service.EpaySettingsInput{Enabled: true, PID: "1001", GatewayURL: "https://pay.example.com"},
	})
	if err != nil {
		t.Fatalf("UpdatePayment without key: %v", err)
	}
	if !again.Epay.KeySet {
		t.Fatalf("epay key should be preserved: %+v", again)
	}

	// 新的服务实例应能从数据库恢复配置。
	reloaded := service.NewSettingsService(repo, cipher, service.NewNotifyService(nil, nil), logger, service.SMTPConfig{})
	if err := reloaded.BootstrapPayment(ctx); err != nil {
		t.Fatalf("BootstrapPayment reload: %v", err)
	}
	if got := reloaded.Payment(); got.DefaultGateway != "epay" || !got.Epay.KeySet {
		t.Fatalf("reloaded payment = %+v", got)
	}

	// 启用但缺少凭据应被拒绝。
	if _, err := svc.UpdatePayment(ctx, service.PaymentSettingsInput{
		Epay: service.EpaySettingsInput{Enabled: true},
	}); !errors.Is(err, service.ErrSettingsConfig) {
		t.Fatalf("missing epay creds err = %v, want ErrSettingsConfig", err)
	}
}

func TestAuthUpdateAndHotReload(t *testing.T) {
	repo := newRepo(t)
	cipher, err := secret.New([]byte("test-encryption-key"))
	if err != nil {
		t.Fatalf("secret.New: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	svc := service.NewSettingsService(repo, cipher, service.NewNotifyService(nil, nil), logger, service.SMTPConfig{})
	var applied []service.AuthConfig
	svc.SetAuthDefaults(service.AuthConfig{AllowRegistration: true, RequireAuth: true})
	svc.SetAuthApplier(func(cfg service.AuthConfig) { applied = append(applied, cfg) })

	ctx := context.Background()
	if !svc.Auth().AllowRegistration || !svc.Auth().RequireAuth || svc.Auth().AllowGuestUpload {
		t.Fatalf("defaults = %+v", svc.Auth())
	}

	// 首次 Bootstrap 将配置兜底写入数据库并应用。
	if err := svc.BootstrapAuth(ctx); err != nil {
		t.Fatalf("BootstrapAuth: %v", err)
	}
	if !svc.Auth().AllowRegistration || !svc.Auth().RequireAuth {
		t.Fatalf("after bootstrap = %+v", svc.Auth())
	}

	// 在线切换应立即生效。
	updated, err := svc.UpdateAuth(ctx, service.AuthConfig{AllowRegistration: false, RequireAuth: false, AllowGuestUpload: true})
	if err != nil {
		t.Fatalf("UpdateAuth: %v", err)
	}
	if updated.AllowRegistration || updated.RequireAuth || !updated.AllowGuestUpload {
		t.Fatalf("updated = %+v", updated)
	}
	if len(applied) == 0 || applied[len(applied)-1].RequireAuth || !applied[len(applied)-1].AllowGuestUpload {
		t.Fatalf("applier did not receive updated config: %+v", applied)
	}

	// 新的服务实例应能从数据库恢复，而不是回退到配置默认。
	reloaded := service.NewSettingsService(repo, cipher, service.NewNotifyService(nil, nil), logger, service.SMTPConfig{})
	reloaded.SetAuthDefaults(service.AuthConfig{AllowRegistration: true, RequireAuth: true})
	if err := reloaded.BootstrapAuth(ctx); err != nil {
		t.Fatalf("BootstrapAuth reload: %v", err)
	}
	if got := reloaded.Auth(); got.AllowRegistration || got.RequireAuth || !got.AllowGuestUpload {
		t.Fatalf("reloaded = %+v, want persisted {false,false,true}", got)
	}
}

func TestAuthBootstrapMergesLegacyRecord(t *testing.T) {
	repo := newRepo(t)
	cipher, err := secret.New([]byte("test-encryption-key"))
	if err != nil {
		t.Fatalf("secret.New: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()

	// 模拟旧版本写入的记录：只有 allow_registration，缺少后来新增的字段。
	if err := repo.SetSetting(ctx, "auth", `{"allow_registration":false}`); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}

	svc := service.NewSettingsService(repo, cipher, service.NewNotifyService(nil, nil), logger, service.SMTPConfig{})
	svc.SetAuthDefaults(service.AuthConfig{AllowRegistration: true, RequireAuth: true})
	if err := svc.BootstrapAuth(ctx); err != nil {
		t.Fatalf("BootstrapAuth: %v", err)
	}
	if got := svc.Auth(); got.AllowRegistration || !got.RequireAuth || got.AllowGuestUpload {
		t.Fatalf("merged = %+v, want {false,true,false}", got)
	}

	// 启动时应把缺失字段补齐回写。
	raw, err := repo.GetSetting(ctx, "auth")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	var stored map[string]any
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		t.Fatalf("unmarshal stored auth: %v", err)
	}
	if _, ok := stored["require_auth"]; !ok {
		t.Fatalf("normalized record missing require_auth: %s", raw)
	}
	if _, ok := stored["allow_guest_upload"]; !ok {
		t.Fatalf("normalized record missing allow_guest_upload: %s", raw)
	}
}

func TestModerationUpdateAndHotReload(t *testing.T) {
	repo := newRepo(t)
	cipher, err := secret.New([]byte("test-encryption-key"))
	if err != nil {
		t.Fatalf("secret.New: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()

	svc := service.NewSettingsService(repo, cipher, service.NewNotifyService(nil, nil), logger, service.SMTPConfig{})
	var applied []config.ModerationConfig
	svc.SetModerationDefaults(config.ModerationConfig{
		Enabled: true, BaseURL: "https://api.openai.com/v1", APIKey: "sk-default",
		Model: "gpt-4o-mini", TimeoutSec: 30, MaxImageMB: 10,
	})
	svc.SetModerationApplier(func(cfg config.ModerationConfig) error {
		applied = append(applied, cfg)
		return nil
	})

	if err := svc.BootstrapModeration(ctx); err != nil {
		t.Fatalf("BootstrapModeration: %v", err)
	}
	if got := svc.Moderation(); !got.Enabled || !got.APIKeySet {
		t.Fatalf("after bootstrap = %+v", got)
	}
	if svc.Moderation().Prompt == "" {
		t.Fatalf("default moderation prompt should be surfaced to the admin UI")
	}

	// 更新时留空密钥应沿用原值。
	updated, err := svc.UpdateModeration(ctx, service.ModerationSettingsInput{
		Enabled: false, BaseURL: "http://localhost:11434/v1", Model: "llava", TimeoutSec: 20, MaxImageMB: 5,
	})
	if err != nil {
		t.Fatalf("UpdateModeration: %v", err)
	}
	if updated.Enabled || !updated.APIKeySet {
		t.Fatalf("updated = %+v, want disabled with preserved key", updated)
	}
	if len(applied) == 0 || applied[len(applied)-1].APIKey != "sk-default" {
		t.Fatalf("applier did not preserve key: %+v", applied)
	}

	// 提供新密钥。
	if _, err := svc.UpdateModeration(ctx, service.ModerationSettingsInput{
		Enabled: true, BaseURL: "http://example/v1", APIKey: "sk-new", Model: "m", TimeoutSec: 30, MaxImageMB: 10,
	}); err != nil {
		t.Fatalf("UpdateModeration new key: %v", err)
	}

	// 落库记录不得包含明文密钥。
	raw, err := repo.GetSetting(ctx, "moderation")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if strings.Contains(raw, "sk-new") {
		t.Fatalf("stored moderation leaks api key: %s", raw)
	}

	// 新实例应从数据库恢复。
	reloaded := service.NewSettingsService(repo, cipher, service.NewNotifyService(nil, nil), logger, service.SMTPConfig{})
	reloaded.SetModerationDefaults(config.ModerationConfig{})
	if err := reloaded.BootstrapModeration(ctx); err != nil {
		t.Fatalf("BootstrapModeration reload: %v", err)
	}
	got := reloaded.Moderation()
	if !got.Enabled || got.Model != "m" || got.BaseURL != "http://example/v1" || !got.APIKeySet {
		t.Fatalf("reloaded = %+v", got)
	}
}

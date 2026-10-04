package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

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

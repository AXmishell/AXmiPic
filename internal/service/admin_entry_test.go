package service_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/config"
	"github.com/AXmishell/axmipic/internal/secret"
	"github.com/AXmishell/axmipic/internal/service"
	"github.com/AXmishell/axmipic/internal/store"
)

func newAdminEntryService(t *testing.T, cfg config.AdminEntryConfig) (*service.AdminEntryService, *store.Repository) {
	t.Helper()
	repo := newRepo(t)
	cipher, err := secret.New([]byte("test-encryption-key"))
	if err != nil {
		t.Fatalf("secret.New: %v", err)
	}
	svc := service.NewAdminEntryService(
		repo, cipher,
		auth.NewAdminEntryIssuer([]byte("test-secret"), 10*time.Minute),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		cfg,
	)
	if err := svc.Bootstrap(context.Background()); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	return svc, repo
}

func TestAdminEntryLifecycle(t *testing.T) {
	svc, _ := newAdminEntryService(t, config.AdminEntryConfig{})
	ctx := context.Background()

	if svc.Enabled() {
		t.Fatal("admin entry should be disabled by default")
	}
	token, err := svc.SetEnabled(ctx, true)
	if err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if len(token) != 43 {
		t.Fatalf("token length = %d, want 43", len(token))
	}
	if !svc.Enabled() || !svc.ValidateToken(token) {
		t.Fatal("enabled admin entry should validate its token")
	}
	if svc.ValidateToken("wrong") {
		t.Fatal("wrong token must be rejected")
	}
	status := svc.Status("https://example.com/")
	if status.URL != "https://example.com/admin/entry/"+token {
		t.Fatalf("status URL = %q", status.URL)
	}

	// 入口 Cookie 只在对应版本内有效。
	rec := httptest.NewRecorder()
	if err := svc.SetEntryCookie(rec, false); err != nil {
		t.Fatalf("SetEntryCookie: %v", err)
	}
	req := httptest.NewRequest("GET", "/admin/login", nil)
	for _, cookie := range rec.Result().Cookies() {
		req.AddCookie(cookie)
	}
	if !svc.Unlocked(req) {
		t.Fatal("entry cookie should unlock admin login")
	}

	newToken, err := svc.Rotate(ctx)
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if newToken == token || svc.ValidateToken(token) || !svc.ValidateToken(newToken) {
		t.Fatal("rotation should invalidate the old token")
	}
	if svc.Unlocked(req) {
		t.Fatal("rotation should invalidate old entry cookies")
	}

	if _, err := svc.SetEnabled(ctx, false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if svc.Enabled() || !svc.Unlocked(httptest.NewRequest("GET", "/", nil)) {
		t.Fatal("disabled admin entry should not gate requests")
	}
}

func TestAdminEntryPersistsAndConfigOverride(t *testing.T) {
	svc, repo := newAdminEntryService(t, config.AdminEntryConfig{})
	token, err := svc.SetEnabled(context.Background(), true)
	if err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}

	// 用同一仓库重新 Bootstrap 应恢复入口。
	cipher, err := secret.New([]byte("test-encryption-key"))
	if err != nil {
		t.Fatalf("secret.New: %v", err)
	}
	reloaded := service.NewAdminEntryService(
		repo, cipher,
		auth.NewAdminEntryIssuer([]byte("test-secret"), 10*time.Minute),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		config.AdminEntryConfig{},
	)
	if err := reloaded.Bootstrap(context.Background()); err != nil {
		t.Fatalf("reload Bootstrap: %v", err)
	}
	if !reloaded.Enabled() || !reloaded.ValidateToken(token) {
		t.Fatal("admin entry should be restored from the database")
	}

	overridden, _ := newAdminEntryService(t, config.AdminEntryConfig{
		Override: true,
		Enabled:  true,
		Token:    token,
	})
	if !overridden.Enabled() || !overridden.ValidateToken(token) {
		t.Fatal("config override should force the configured token")
	}
	if _, err := overridden.SetEnabled(context.Background(), false); !errors.Is(err, service.ErrAdminEntryManaged) {
		t.Fatalf("SetEnabled under override err = %v, want ErrAdminEntryManaged", err)
	}

	disabled, _ := newAdminEntryService(t, config.AdminEntryConfig{Override: true, Enabled: false})
	if disabled.Enabled() {
		t.Fatal("config override should be able to disable admin entry")
	}
}

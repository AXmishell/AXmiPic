package api_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AXmishell/axmipic/internal/api"
	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/service"
	"github.com/AXmishell/axmipic/internal/store"
)

// TestInstallWithoutToken 验证初始化接口不再要求初始化令牌：未携带令牌时
// 请求会直接进入请求体校验。
func TestInstallWithoutToken(t *testing.T) {
	repo, err := store.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	issuer := auth.NewSessionIssuer([]byte("test-secret"), time.Hour)
	installSvc := service.NewInstallService(filepath.Join(t.TempDir(), "missing.lock"), "", false)
	router := api.NewRouter(api.Deps{
		Install:       installSvc,
		Authenticator: auth.NewAuthenticator(repo, issuer),
		InstallRepo: func(driver, dsn string) (*store.Repository, error) {
			return store.Open("sqlite", filepath.Join(t.TempDir(), "install.db"))
		},
		InstallSeed: func(_ context.Context, _ *store.Repository, _ service.InstallInput) error { return nil },
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/install", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed body status = %d, want 400", rec.Code)
	}
}

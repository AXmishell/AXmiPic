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

// TestInstallTokenRequired 验证未安装时 POST /install 必须携带正确的安装令牌。
func TestInstallTokenRequired(t *testing.T) {
	repo, err := store.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	issuer := auth.NewSessionIssuer([]byte("test-secret"), time.Hour)
	installSvc := service.NewInstallService(filepath.Join(t.TempDir(), "missing.lock"), "", false, "setup-token")
	router := api.NewRouter(api.Deps{
		Install:       installSvc,
		Authenticator: auth.NewAuthenticator(repo, issuer),
		InstallRepo: func(driver, dsn string) (*store.Repository, error) {
			return store.Open("sqlite", filepath.Join(t.TempDir(), "install.db"))
		},
		InstallSeed: func(_ context.Context, _ *store.Repository, _ service.InstallInput) error { return nil },
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	post := func(token, body string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/install", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("X-Install-Token", token)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec.Code
	}

	if status := post("", `{}`); status != http.StatusForbidden {
		t.Fatalf("missing token status = %d, want 403", status)
	}
	if status := post("wrong", `{}`); status != http.StatusForbidden {
		t.Fatalf("wrong token status = %d, want 403", status)
	}
	// 正确令牌通过授权后进入请求体校验。
	if status := post("setup-token", "not-json"); status != http.StatusBadRequest {
		t.Fatalf("authorized malformed body status = %d, want 400", status)
	}
}

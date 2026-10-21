package api_test

import (
	"context"
	"encoding/json"
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

// TestInstallThenAdminLoginWithoutRestart 验证切换到新数据库后，同一进程可立即
// 使用初始化向导创建的管理员登录，无需重启。
func TestInstallThenAdminLoginWithoutRestart(t *testing.T) {
	live, err := store.Open("sqlite", filepath.Join(t.TempDir(), "live.db"))
	if err != nil {
		t.Fatalf("store.Open(live): %v", err)
	}
	t.Cleanup(func() { _ = live.Close() })

	issuer := auth.NewSessionIssuer([]byte("test-secret"), time.Hour)
	accounts := service.NewAccountService(live, issuer, false, 0)
	installSvc := service.NewInstallService(filepath.Join(t.TempDir(), "install.lock"), "", false)
	installSvc.SetActivator(live.Adopt)

	router := api.NewRouter(api.Deps{
		Accounts:      accounts,
		Install:       installSvc,
		Authenticator: auth.NewAuthenticator(live, issuer),
		InstallRepo: func(_, _ string) (*store.Repository, error) {
			return store.Open("sqlite", filepath.Join(t.TempDir(), "target.db"))
		},
		InstallSeed: func(ctx context.Context, repo *store.Repository, in service.InstallInput) error {
			seedAccounts := service.NewAccountService(repo, issuer, false, 0)
			_, err := seedAccounts.RegisterAdmin(ctx, in.AdminUsername, in.AdminPassword)
			return err
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	installBody := `{
		"site_name": "AXmiPic",
		"base_url": "http://example.test",
		"database_driver": "sqlite",
		"database_dsn": "target.db",
		"admin_username": "123456@qq.com",
		"admin_password": "Abcdefghij1234567890!@#$%^&*()",
		"allow_registration": true,
		"allow_guest_upload": true
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/install", strings.NewReader(installBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("install status = %d, body = %s", rec.Code, rec.Body.String())
	}

	loginBody := `{
		"username": "123456@qq.com",
		"password": "Abcdefghij1234567890!@#$%^&*()"
	}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/login", strings.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin login after install status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if resp.Data.Token == "" {
		t.Fatal("admin login after install returned no token")
	}
}

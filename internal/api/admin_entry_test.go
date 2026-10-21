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
	"github.com/AXmishell/axmipic/internal/config"
	"github.com/AXmishell/axmipic/internal/secret"
	"github.com/AXmishell/axmipic/internal/service"
	"github.com/AXmishell/axmipic/internal/store"
)

func TestAdminEntryGatesLogin(t *testing.T) {
	repo, err := store.Open("sqlite", filepath.Join(t.TempDir(), "entry.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	cipher, err := secret.New([]byte("test-encryption-key"))
	if err != nil {
		t.Fatalf("secret.New: %v", err)
	}
	entrySvc := service.NewAdminEntryService(
		repo, cipher,
		auth.NewAdminEntryIssuer([]byte("test-secret"), 10*time.Minute),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		config.AdminEntryConfig{},
	)
	if err := entrySvc.Bootstrap(context.Background()); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if _, err := entrySvc.SetEnabled(context.Background(), true); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}

	issuer := auth.NewSessionIssuer([]byte("test-secret"), time.Hour)
	accounts := service.NewAccountService(repo, issuer, false, 0)
	admin, err := accounts.RegisterAdmin(context.Background(), "admin", "password123")
	if err != nil {
		t.Fatalf("RegisterAdmin: %v", err)
	}
	adminToken, _, err := issuer.Issue(admin.ID, string(auth.RoleAdmin))
	if err != nil {
		t.Fatalf("Issue admin token: %v", err)
	}
	static := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "spa")
	})
	router := api.NewRouter(api.Deps{
		Accounts:      accounts,
		AdminEntry:    entrySvc,
		Authenticator: auth.NewAuthenticator(repo, issuer),
		Static:        static,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	// 没有入口 Cookie 时，登录页与登录接口都按 404 处理。
	req := httptest.NewRequest(http.MethodGet, "/admin/login", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("login page without entry status = %d, want 404", rec.Code)
	}

	body := `{"username":"admin","password":"password123"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("login API without entry status = %d, want 404", rec.Code)
	}

	// 模拟访问安全入口后获得的 Cookie。
	cookieRec := httptest.NewRecorder()
	if err := entrySvc.SetEntryCookie(cookieRec, false); err != nil {
		t.Fatalf("SetEntryCookie: %v", err)
	}
	cookies := cookieRec.Result().Cookies()

	req = httptest.NewRequest(http.MethodGet, "/admin/login", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "spa" {
		t.Fatalf("login page with entry = %d %q", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login with entry status = %d, body = %s", rec.Code, rec.Body.String())
	}
	cleared := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.AdminEntryCookieName && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("successful admin login should clear the admin entry cookie")
	}

	// 修改安全入口需要当前密码。
	update := `{"enabled":false}`
	req = httptest.NewRequest(http.MethodPut, "/api/v1/admin/security/admin-entry", strings.NewReader(update))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("update entry without password status = %d, want 400", rec.Code)
	}

	update = `{"enabled":false,"current_password":"password123"}`
	req = httptest.NewRequest(http.MethodPut, "/api/v1/admin/security/admin-entry", strings.NewReader(update))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("disable entry status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if entrySvc.Enabled() {
		t.Fatal("admin entry should be disabled")
	}
}

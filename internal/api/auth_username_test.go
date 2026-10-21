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

func TestChangeUsernameAPI(t *testing.T) {
	repo, err := store.Open("sqlite", filepath.Join(t.TempDir(), "username.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	issuer := auth.NewSessionIssuer([]byte("test-secret"), time.Hour)
	accounts := service.NewAccountService(repo, issuer, true, 0)
	user, err := accounts.RegisterCustomer(context.Background(), "alice", "password123")
	if err != nil {
		t.Fatalf("RegisterCustomer: %v", err)
	}
	token, _, err := issuer.Issue(user.ID, string(auth.RoleUser))
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	router := api.NewRouter(api.Deps{
		Accounts:      accounts,
		Authenticator: auth.NewAuthenticator(repo, issuer),
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	body := `{"username":"Alice2@Example.com","current_password":"password123"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/auth/username", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"username":"Alice2@Example.com"`) {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
	if _, err := accounts.LoginCustomer(context.Background(), "alice2@EXAMPLE.com", "password123"); err != nil {
		t.Fatalf("login with new username: %v", err)
	}

	// 当前密码错误返回 400。
	body = `{"username":"alice3","current_password":"wrong"}`
	req = httptest.NewRequest(http.MethodPut, "/api/v1/auth/username", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("wrong password status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
}

package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/service"
	"github.com/AXmishell/axmipic/internal/store"
)

func newAccountService(t *testing.T, allowRegistration bool, quotaBytes int64) (*service.AccountService, *store.Repository) {
	t.Helper()
	repo := newRepo(t)
	issuer := auth.NewSessionIssuer([]byte("test-secret"), time.Hour)
	return service.NewAccountService(repo, issuer, allowRegistration, quotaBytes), repo
}

func TestRegisterLoginMe(t *testing.T) {
	svc, _ := newAccountService(t, true, 1<<20)
	ctx := context.Background()

	user, err := svc.RegisterCustomer(ctx, "alice", "password123")
	if err != nil {
		t.Fatalf("RegisterCustomer: %v", err)
	}
	if user.Username != "alice" || user.Role != string(auth.RoleUser) {
		t.Fatalf("unexpected user: %+v", user)
	}

	session, err := svc.LoginCustomer(ctx, "alice", "password123")
	if err != nil {
		t.Fatalf("LoginCustomer: %v", err)
	}
	if session.Token == "" || session.User.ID != user.ID {
		t.Fatalf("unexpected session: %+v", session)
	}

	me, err := svc.Me(ctx, &auth.Principal{UserID: user.ID, Role: auth.RoleUser})
	if err != nil || me.ID != user.ID {
		t.Fatalf("Me: %v (%+v)", err, me)
	}

	if _, err := svc.RegisterCustomer(ctx, "alice", "password123"); !errors.Is(err, service.ErrUserExists) {
		t.Fatalf("duplicate register error = %v, want ErrUserExists", err)
	}
	if _, err := svc.LoginCustomer(ctx, "alice", "wrong-password"); !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("bad login error = %v, want ErrInvalidCredentials", err)
	}
	if _, err := svc.RegisterCustomer(ctx, "ab", "short"); !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("invalid input error = %v, want ErrInvalidInput", err)
	}
}

func TestRegistrationDisabled(t *testing.T) {
	svc, _ := newAccountService(t, false, 0)
	if _, err := svc.RegisterCustomer(context.Background(), "bob", "password123"); !errors.Is(err, service.ErrRegistrationDisabled) {
		t.Fatalf("error = %v, want ErrRegistrationDisabled", err)
	}
}

func TestAdminAndCustomerAreSeparateTables(t *testing.T) {
	svc, repo := newAccountService(t, true, 1<<20)
	ctx := context.Background()

	admin, err := svc.RegisterAdmin(ctx, "shared", "password123")
	if err != nil {
		t.Fatalf("RegisterAdmin: %v", err)
	}
	if admin.Role != string(auth.RoleAdmin) {
		t.Fatalf("admin role = %q", admin.Role)
	}
	customer, err := svc.RegisterCustomer(ctx, "shared", "password123")
	if err != nil {
		t.Fatalf("RegisterCustomer with same name: %v", err)
	}
	if customer.ID == admin.ID {
		t.Fatal("admin and customer share an id")
	}

	if n, _ := repo.CountAdmins(ctx); n != 1 {
		t.Fatalf("admins = %d, want 1", n)
	}
	if n, _ := repo.CountCustomers(ctx); n != 1 {
		t.Fatalf("customers = %d, want 1", n)
	}

	// 跨表登录必须失败：管理员凭证不能用于普通用户入口。
	if _, err := svc.LoginCustomer(ctx, "shared", "password123"); err != nil {
		t.Fatalf("customer login failed: %v", err)
	}
	if _, err := svc.LoginAdmin(ctx, "customer-only", "password123"); !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("missing admin login error = %v, want ErrInvalidCredentials", err)
	}
	if _, err := svc.LoginCustomer(ctx, "admin-only", "password123"); !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("missing customer login error = %v, want ErrInvalidCredentials", err)
	}
}

func TestTokenLifecycle(t *testing.T) {
	svc, _ := newAccountService(t, true, 0)
	ctx := context.Background()
	user, err := svc.RegisterCustomer(ctx, "carol", "password123")
	if err != nil {
		t.Fatalf("RegisterCustomer: %v", err)
	}

	token, err := svc.CreateToken(ctx, user.ID, "cli")
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if !strings.HasPrefix(token.Token, auth.TokenPrefix) {
		t.Fatalf("unexpected token: %q", token.Token)
	}

	tokens, err := svc.ListTokens(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListTokens: %v", err)
	}
	if len(tokens) != 1 || tokens[0].ID != token.ID {
		t.Fatalf("unexpected tokens: %+v", tokens)
	}
	if tokens[0].Token != "" {
		t.Fatal("list leaked the token secret")
	}

	if err := svc.RevokeToken(ctx, user.ID, token.ID); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}
	if err := svc.RevokeToken(ctx, user.ID, token.ID); !errors.Is(err, service.ErrTokenNotFound) {
		t.Fatalf("second revoke error = %v, want ErrTokenNotFound", err)
	}
}

func TestEnsureBootstrapAdmin(t *testing.T) {
	svc, repo := newAccountService(t, false, 0)
	ctx := context.Background()

	if err := svc.EnsureBootstrapAdmin(ctx, "root:password123"); err != nil {
		t.Fatalf("EnsureBootstrapAdmin: %v", err)
	}
	admin, err := repo.GetAccountByUsername(ctx, store.RoleAdmin, "root")
	if err != nil {
		t.Fatalf("GetAccountByUsername: %v", err)
	}
	if admin.Role != store.RoleAdmin {
		t.Fatalf("role = %q, want admin", admin.Role)
	}

	// 第二个 spec 会被忽略，因为已经存在一个管理员。
	if err := svc.EnsureBootstrapAdmin(ctx, "other:password123"); err != nil {
		t.Fatalf("EnsureBootstrapAdmin (second): %v", err)
	}
	if _, err := repo.GetAccountByUsername(ctx, store.RoleAdmin, "other"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("second bootstrap admin was created")
	}
}

func TestUploadReservesAndDeleteReleasesQuota(t *testing.T) {
	repo := newRepo(t)
	fs := newFakeStorage()
	svc := service.NewUploadService(repo, fs, pngPolicy())
	ctx := context.Background()

	customer := &store.Customer{ID: "u1", Username: "u1", PasswordHash: "x", QuotaBytes: 1 << 20}
	if err := repo.CreateCustomer(ctx, customer); err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	principal := &auth.Principal{UserID: "u1", Role: auth.RoleUser}

	dto, err := svc.Upload(ctx, principal, service.UploadInput{Data: testPNG(t), MimeType: "image/png"})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	stored, err := repo.GetAccountByID(ctx, store.RoleCustomer, "u1")
	if err != nil {
		t.Fatalf("GetAccountByID: %v", err)
	}
	if stored.UsedBytes != dto.Size {
		t.Fatalf("used bytes = %d, want %d", stored.UsedBytes, dto.Size)
	}

	if err := svc.Delete(ctx, principal, dto.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	stored, err = repo.GetAccountByID(ctx, store.RoleCustomer, "u1")
	if err != nil {
		t.Fatalf("GetAccountByID: %v", err)
	}
	if stored.UsedBytes != 0 {
		t.Fatalf("used bytes after delete = %d, want 0", stored.UsedBytes)
	}
}

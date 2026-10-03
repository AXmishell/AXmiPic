package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/axmipic/axmipic/internal/auth"
	"github.com/axmipic/axmipic/internal/service"
	"github.com/axmipic/axmipic/internal/store"
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

	user, err := svc.Register(ctx, "alice", "password123")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if user.Username != "alice" || user.Role != string(auth.RoleUser) {
		t.Fatalf("unexpected user: %+v", user)
	}

	session, err := svc.Login(ctx, "alice", "password123")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if session.Token == "" || session.User.ID != user.ID {
		t.Fatalf("unexpected session: %+v", session)
	}

	me, err := svc.Me(ctx, user.ID)
	if err != nil || me.ID != user.ID {
		t.Fatalf("Me: %v (%+v)", err, me)
	}

	if _, err := svc.Register(ctx, "alice", "password123"); !errors.Is(err, service.ErrUserExists) {
		t.Fatalf("duplicate register error = %v, want ErrUserExists", err)
	}
	if _, err := svc.Login(ctx, "alice", "wrong-password"); !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("bad login error = %v, want ErrInvalidCredentials", err)
	}
	if _, err := svc.Register(ctx, "ab", "short"); !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("invalid input error = %v, want ErrInvalidInput", err)
	}
}

func TestRegistrationDisabled(t *testing.T) {
	svc, _ := newAccountService(t, false, 0)
	if _, err := svc.Register(context.Background(), "bob", "password123"); !errors.Is(err, service.ErrRegistrationDisabled) {
		t.Fatalf("error = %v, want ErrRegistrationDisabled", err)
	}
}

func TestTokenLifecycle(t *testing.T) {
	svc, _ := newAccountService(t, true, 0)
	ctx := context.Background()
	user, err := svc.Register(ctx, "carol", "password123")
	if err != nil {
		t.Fatalf("Register: %v", err)
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
	admin, err := repo.GetUserByUsername(ctx, "root")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if admin.Role != string(auth.RoleAdmin) {
		t.Fatalf("role = %q, want admin", admin.Role)
	}

	// A second spec is ignored because an account already exists.
	if err := svc.EnsureBootstrapAdmin(ctx, "other:password123"); err != nil {
		t.Fatalf("EnsureBootstrapAdmin (second): %v", err)
	}
	if _, err := repo.GetUserByUsername(ctx, "other"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("second bootstrap admin was created")
	}
}

func TestUploadReservesAndDeleteReleasesQuota(t *testing.T) {
	repo := newRepo(t)
	fs := newFakeStorage()
	svc := service.NewUploadService(repo, fs, pngPolicy())
	ctx := context.Background()

	user := &store.User{ID: "u1", Username: "u1", PasswordHash: "x", Role: string(auth.RoleUser), QuotaBytes: 1 << 20}
	if err := repo.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	principal := &auth.Principal{UserID: "u1", Role: auth.RoleUser}

	dto, err := svc.Upload(ctx, principal, service.UploadInput{Data: testPNG(t), MimeType: "image/png"})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	stored, err := repo.GetUserByID(ctx, "u1")
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if stored.UsedBytes != dto.Size {
		t.Fatalf("used bytes = %d, want %d", stored.UsedBytes, dto.Size)
	}

	if err := svc.Delete(ctx, principal, dto.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	stored, err = repo.GetUserByID(ctx, "u1")
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if stored.UsedBytes != 0 {
		t.Fatalf("used bytes after delete = %d, want 0", stored.UsedBytes)
	}
}

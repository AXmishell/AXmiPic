package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/AXmishell/axmipic/internal/api"
	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/config"
	"github.com/AXmishell/axmipic/internal/imaging"
	"github.com/AXmishell/axmipic/internal/secret"
	"github.com/AXmishell/axmipic/internal/service"
	"github.com/AXmishell/axmipic/internal/storage"
	"github.com/AXmishell/axmipic/internal/store"
)

// newPolicyTestEnv 构造一个启用了角色策略的测试环境，用于验证服务端功能
// 开关校验。
func newPolicyTestEnv(t *testing.T) (*testEnv, *service.PolicyService) {
	t.Helper()
	repo, err := store.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	local, err := storage.NewLocal(t.TempDir(), "http://localhost:8080")
	if err != nil {
		t.Fatalf("storage.NewLocal: %v", err)
	}
	manager := storage.NewManager()
	manager.SetFallback(local)
	cipher, err := secret.New([]byte("test-encryption-key"))
	if err != nil {
		t.Fatalf("secret.New: %v", err)
	}
	storageSvc := service.NewStorageService(repo, manager, cipher, "http://localhost:8080", config.StorageConfig{
		Driver: "local",
		Local:  config.LocalStorageConfig{Root: t.TempDir()},
	})
	uploadSvc := service.NewUploadService(repo, manager, service.UploadPolicy{
		MaxSizeBytes:     1 << 20,
		AllowedMIMETypes: []string{"image/png"},
		PresignExpiry:    10,
	})
	imagingSvc := service.NewImagingService(manager, imaging.Default(), service.ProcessingPolicy{
		Enabled:        true,
		MaxWidth:       64,
		MaxHeight:      64,
		DefaultQuality: 82,
		AllowedFormats: []imaging.Format{imaging.FormatPNG, imaging.FormatJPEG},
	})
	ctx := context.Background()
	policies := service.NewPolicyService(repo, service.PolicyDefaults{
		QuotaBytes:       1 << 20,
		UploadMaxBytes:   1 << 20,
		AllowedMIMETypes: []string{"image/png"},
		Rate: service.RateSettings{
			UploadPerMinute: 10000, UploadBurst: 1000,
			ImagePerMinute: 10000, ImageBurst: 1000,
		},
		Processing: service.ProcessingSettings{
			Enabled: true, MaxWidth: 64, MaxHeight: 64, DefaultQuality: 82,
			AllowedFormats: []string{"png", "jpeg"},
		},
	})
	if err := policies.SeedDefaults(ctx); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	issuer := auth.NewSessionIssuer([]byte("test-secret"), time.Hour)
	accounts := service.NewAccountService(repo, issuer, true, 1<<20)
	accounts.SetPolicyService(policies)
	router := api.NewRouter(api.Deps{
		Upload:        uploadSvc,
		Imaging:       imagingSvc,
		Accounts:      accounts,
		Admin:         service.NewAdminService(repo, "local", imaging.Default()),
		Albums:        service.NewAlbumService(repo),
		Storage:       storageSvc,
		Policies:      policies,
		Shares:        service.NewShareService(repo, "http://localhost:8080"),
		Authenticator: auth.NewAuthenticator(repo, issuer),
		UploadLimiter: &auth.UploadLimiter{
			User:  auth.NewRateLimiter(10000, 1000),
			Guest: auth.NewRateLimiter(10000, 1000),
		},
		ShareLimiter: auth.NewRateLimiter(1, 1),
		RequireAuth:  true,
		MaxUploadMB:  1,
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return &testEnv{router: router, repo: repo, accounts: accounts}, policies
}

// restrictFeatures 为 userID 新建一个仅启用给定功能点的角色组并分配给它。
func restrictFeatures(t *testing.T, policies *service.PolicyService, repo *store.Repository, userID string, features ...string) {
	t.Helper()
	ctx := context.Background()
	suffix := uuid.NewString()[:8]
	group, err := policies.CreateRoleGroup(ctx, service.RoleGroupInput{Name: "受限-" + suffix})
	if err != nil {
		t.Fatalf("CreateRoleGroup: %v", err)
	}
	settings, err := json.Marshal(map[string]any{"features": features})
	if err != nil {
		t.Fatalf("marshal features: %v", err)
	}
	policy, err := policies.CreatePolicy(ctx, service.PolicyInput{
		Name:     "受限功能-" + suffix,
		Type:     store.PolicyTypeFeature,
		Enabled:  true,
		Settings: settings,
	})
	if err != nil {
		t.Fatalf("CreatePolicy: %v", err)
	}
	if _, err := policies.AttachPolicy(ctx, group.ID, policy.ID); err != nil {
		t.Fatalf("AttachPolicy: %v", err)
	}
	groupID := group.ID
	if _, err := repo.UpdateCustomer(ctx, userID, store.UserUpdate{RoleGroupID: &groupID}); err != nil {
		t.Fatalf("UpdateCustomer: %v", err)
	}
}

// TestFeatureSwitchesEnforcedServerSide 验证功能开关在服务端被强制执行，
// 而不是仅在前端隐藏入口。
func TestFeatureSwitchesEnforcedServerSide(t *testing.T) {
	env, policies := newPolicyTestEnv(t)

	user, err := env.accounts.RegisterCustomer(context.Background(), "gated", "password123")
	if err != nil {
		t.Fatalf("RegisterCustomer: %v", err)
	}
	token, err := env.accounts.CreateToken(context.Background(), user.ID, "test")
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	// 默认角色组包含全部功能点。
	if status, body := do(t, env.router, http.MethodPost, "/api/v1/tokens", `{"name":"cli"}`, token.Token); status != http.StatusCreated {
		t.Fatalf("default tokens status = %d (%s), want 201", status, body)
	}

	// 收紧为不启用任何功能点。
	restrictFeatures(t, policies, env.repo, user.ID)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"api_tokens", http.MethodPost, "/api/v1/tokens", `{"name":"cli"}`},
		{"albums", http.MethodPost, "/api/v1/albums", `{"name":"a"}`},
		{"albums-list", http.MethodGet, "/api/v1/albums", ""},
		{"batch_upload", http.MethodPost, "/api/v1/images/batch", `{"ids":["x"],"permission":"public"}`},
	}
	for _, tc := range cases {
		status, body := do(t, env.router, tc.method, tc.path, tc.body, token.Token)
		if status != http.StatusForbidden {
			t.Fatalf("%s status = %d (%s), want 403", tc.name, status, body)
		}
	}

	// 恢复全部功能点后应再次放行。
	restrictFeatures(t, policies, env.repo, user.ID, service.KnownFeatures()...)
	if status, body := do(t, env.router, http.MethodPost, "/api/v1/albums", `{"name":"b"}`, token.Token); status != http.StatusCreated {
		t.Fatalf("restored albums status = %d (%s), want 201", status, body)
	}
}

// TestAdminBypassesFeatureSwitches 验证管理员不受功能开关限制。
func TestAdminBypassesFeatureSwitches(t *testing.T) {
	env, _ := newPolicyTestEnv(t)
	adminToken, _ := env.adminToken(t, "root")
	if status, body := do(t, env.router, http.MethodPost, "/api/v1/tokens", `{"name":"cli"}`, adminToken); status != http.StatusCreated {
		t.Fatalf("admin tokens status = %d (%s), want 201", status, body)
	}
}

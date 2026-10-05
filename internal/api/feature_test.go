package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
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
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	settingsSvc := service.NewSettingsService(repo, cipher, service.NewNotifyService(nil, nil), logger, service.SMTPConfig{})
	settingsSvc.SetAuthDefaults(service.AuthConfig{AllowRegistration: true, RequireAuth: true})
	router := api.NewRouter(api.Deps{
		Upload:        uploadSvc,
		Imaging:       imagingSvc,
		Accounts:      accounts,
		Admin:         service.NewAdminService(repo, "local", imaging.Default()),
		Albums:        service.NewAlbumService(repo),
		Storage:       storageSvc,
		Policies:      policies,
		Shares:        service.NewShareService(repo, "http://localhost:8080"),
		Settings:      settingsSvc,
		Authenticator: auth.NewAuthenticator(repo, issuer),
		GuestSigner:   auth.NewGuestSigner([]byte("test-guest-secret")),
		UploadLimiter: &auth.UploadLimiter{
			User:  auth.NewRateLimiter(10000, 1000),
			Guest: auth.NewRateLimiter(10000, 1000),
		},
		ShareLimiter: auth.NewRateLimiter(1, 1),
		MaxUploadMB:  1,
		Logger:       logger,
	})
	return &testEnv{router: router, repo: repo, accounts: accounts, settings: settingsSvc}, policies
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

// TestRuntimeGuestLimitsReflectPolicies 验证运行环境信息里的访客配额与单文件
// 上限取自 Guest 角色组策略，而不是配置文件的 auth.guest_* 兜底值。
func TestRuntimeGuestLimitsReflectPolicies(t *testing.T) {
	env, policies := newPolicyTestEnv(t)
	adminToken, _ := env.adminToken(t, "root")
	ctx := context.Background()

	if _, err := policies.SeedGuestRoleGroup(ctx, 64<<20, 5<<20); err != nil {
		t.Fatalf("SeedGuestRoleGroup: %v", err)
	}

	status, body := do(t, env.router, http.MethodGet, "/api/v1/admin/runtime", "", adminToken)
	if status != http.StatusOK {
		t.Fatalf("runtime status = %d (%s)", status, body)
	}
	var got struct {
		Data struct {
			GuestQuotaMB     int `json:"guest_quota_mb"`
			GuestUploadMaxMB int `json:"guest_upload_max_mb"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode runtime: %v", err)
	}
	if got.Data.GuestQuotaMB != 64 || got.Data.GuestUploadMaxMB != 5 {
		t.Fatalf("guest limits = %d/%d, want 64/5 (%s)", got.Data.GuestQuotaMB, got.Data.GuestUploadMaxMB, body)
	}
}

// TestGuestUploadPerVisitorAccounts 验证开启访客上传后，每个匿名访客（按签名
// cookie）会获得独立的 Guest 账户，且这些账户不出现在用户管理中。
func TestGuestUploadPerVisitorAccounts(t *testing.T) {
	env, _ := newPolicyTestEnv(t)
	ctx := context.Background()
	adminToken, _ := env.adminToken(t, "root")

	status, body := do(t, env.router, http.MethodPut, "/api/v1/admin/auth",
		`{"allow_registration":true,"require_auth":true,"allow_guest_upload":true}`, adminToken)
	if status != http.StatusOK {
		t.Fatalf("enable guest upload status = %d (%s)", status, body)
	}

	data := testPNG(t, 8)
	doGuestUpload := func(cookie *http.Cookie) (int, *http.Cookie) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		part, err := mw.CreateFormFile("file", "guest.png")
		if err != nil {
			t.Fatalf("CreateFormFile: %v", err)
		}
		if _, err := part.Write(data); err != nil {
			t.Fatalf("write part: %v", err)
		}
		if err := mw.Close(); err != nil {
			t.Fatalf("close writer: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/upload", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		if cookie != nil {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		env.router.ServeHTTP(rec, req)
		var issued *http.Cookie
		for _, c := range rec.Result().Cookies() {
			if c.Name == auth.GuestCookieName {
				issued = c
			}
		}
		return rec.Code, issued
	}

	// 首次匿名上传：新建访客账户并下发签名 cookie。
	status, cookie1 := doGuestUpload(nil)
	if status != http.StatusOK || cookie1 == nil {
		t.Fatalf("first guest upload status = %d, cookie = %v", status, cookie1)
	}
	// 携带同一 cookie：复用同一访客账户。
	if status, _ := doGuestUpload(cookie1); status != http.StatusOK {
		t.Fatalf("cookie guest upload status = %d", status)
	}
	// 不带 cookie：应创建独立的新访客账户。
	status, cookie2 := doGuestUpload(nil)
	if status != http.StatusOK || cookie2 == nil {
		t.Fatalf("second visitor upload status = %d, cookie = %v", status, cookie2)
	}
	if cookie1.Value == cookie2.Value {
		t.Fatal("two visitors received the same guest cookie")
	}

	guests, err := env.repo.CountGuestAccounts(ctx)
	if err != nil {
		t.Fatalf("CountGuestAccounts: %v", err)
	}
	if guests != 2 {
		t.Fatalf("guest accounts = %d, want 2", guests)
	}

	status, body = do(t, env.router, http.MethodGet, "/api/v1/admin/customers", "", adminToken)
	if status != http.StatusOK {
		t.Fatalf("customers status = %d (%s)", status, body)
	}
	if bytes.Contains(body, []byte("guest_")) {
		t.Fatalf("admin customers should exclude guest accounts: %s", body)
	}
}

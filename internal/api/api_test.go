package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/AXmishell/axmipic/internal/api"
	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/config"
	"github.com/AXmishell/axmipic/internal/imaging"
	"github.com/AXmishell/axmipic/internal/notify"
	"github.com/AXmishell/axmipic/internal/secret"
	"github.com/AXmishell/axmipic/internal/service"
	"github.com/AXmishell/axmipic/internal/storage"
	"github.com/AXmishell/axmipic/internal/store"
)

type testEnv struct {
	router   http.Handler
	repo     *store.Repository
	accounts *service.AccountService
	mail     *notify.MockSender
}

func newTestEnv(t *testing.T, requireAuth bool, quotaBytes int64) *testEnv {
	t.Helper()
	repo, err := store.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() {
		_ = repo.Close()
	})
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
	issuer := auth.NewSessionIssuer([]byte("test-secret"), time.Hour)
	accounts := service.NewAccountService(repo, issuer, true, quotaBytes)
	mail := notify.NewMockSender("email")
	accounts.SetNotifyService(service.NewNotifyService(nil, mail))
	router := api.NewRouter(api.Deps{
		Upload:        uploadSvc,
		Imaging:       imagingSvc,
		Accounts:      accounts,
		Admin:         service.NewAdminService(repo, "local", imaging.Default()),
		Albums:        service.NewAlbumService(repo),
		Storage:       storageSvc,
		Authenticator: auth.NewAuthenticator(repo, issuer),
		UploadLimiter: &auth.UploadLimiter{
			User:  auth.NewRateLimiter(10000, 1000),
			Guest: auth.NewRateLimiter(10000, 1000),
		},
		RequireAuth: requireAuth,
		MaxUploadMB: 1,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return &testEnv{router: router, repo: repo, accounts: accounts, mail: mail}
}

// lastEmailCode 从最近一封邮件正文中提取 6 位验证码。
var emailCodePattern = regexp.MustCompile(`\b(\d{6})\b`)

func (e *testEnv) lastEmailCode(t *testing.T) string {
	t.Helper()
	if len(e.mail.Sent) == 0 {
		t.Fatal("no email was sent")
	}
	match := emailCodePattern.FindStringSubmatch(e.mail.Sent[len(e.mail.Sent)-1].Body)
	if len(match) < 2 {
		t.Fatalf("no code in email body: %q", e.mail.Sent[len(e.mail.Sent)-1].Body)
	}
	return match[1]
}

func (e *testEnv) token(t *testing.T, username string) string {
	t.Helper()
	user, err := e.accounts.RegisterCustomer(context.Background(), username, "password123")
	if err != nil {
		t.Fatalf("RegisterCustomer: %v", err)
	}
	token, err := e.accounts.CreateToken(context.Background(), user.ID, "test")
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	return token.Token
}

func (e *testEnv) adminToken(t *testing.T, username string) (string, string) {
	t.Helper()
	user, err := e.accounts.RegisterAdmin(context.Background(), username, "password123")
	if err != nil {
		t.Fatalf("RegisterAdmin: %v", err)
	}
	token, err := e.accounts.CreateToken(context.Background(), user.ID, "admin")
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	return token.Token, user.ID
}

func testPNG(t *testing.T, size int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

func do(t *testing.T, router http.Handler, method, path, body, token string) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

func uploadPNG(t *testing.T, router http.Handler, data []byte, token string) (int, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "test.png")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var env struct {
		Data struct {
			Key string `json:"key"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return rec.Code, env.Data.Key
}

func TestHealthz(t *testing.T) {
	env := newTestEnv(t, true, 1<<20)
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestUploadRequiresAuth(t *testing.T) {
	env := newTestEnv(t, true, 1<<20)
	status, _ := uploadPNG(t, env.router, testPNG(t, 8), "")
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", status)
	}
}

func TestAuthFlowAndOwnership(t *testing.T) {
	env := newTestEnv(t, true, 1<<20)

	// 注册前先请求邮箱验证码。
	status, body := do(t, env.router, http.MethodPost, "/api/v1/auth/register/code",
		`{"email":"alice@example.com"}`, "")
	if status != http.StatusOK {
		t.Fatalf("register code status = %d (%s)", status, body)
	}
	code := env.lastEmailCode(t)

	status, body = do(t, env.router, http.MethodPost, "/api/v1/auth/register",
		`{"username":"alice","email":"alice@example.com","code":"`+code+`","password":"password123"}`, "")
	if status != http.StatusCreated {
		t.Fatalf("register status = %d (%s)", status, body)
	}

	status, body = do(t, env.router, http.MethodPost, "/api/v1/auth/login",
		`{"username":"alice","password":"password123"}`, "")
	if status != http.StatusOK {
		t.Fatalf("login status = %d (%s)", status, body)
	}

	// 邮箱也可用于登录。
	status, body = do(t, env.router, http.MethodPost, "/api/v1/auth/login",
		`{"username":"alice@example.com","password":"password123"}`, "")
	if status != http.StatusOK {
		t.Fatalf("email login status = %d (%s)", status, body)
	}

	var session struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &session); err != nil || session.Data.Token == "" {
		t.Fatalf("bad login response: %s", body)
	}

	status, body = do(t, env.router, http.MethodPost, "/api/v1/tokens",
		`{"name":"cli"}`, session.Data.Token)
	if status != http.StatusCreated {
		t.Fatalf("create token status = %d (%s)", status, body)
	}
	var tokenResp struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil || tokenResp.Data.Token == "" {
		t.Fatalf("bad token response: %s", body)
	}
	apiToken := tokenResp.Data.Token

	status, key := uploadPNG(t, env.router, testPNG(t, 8), apiToken)
	if status != http.StatusOK || key == "" {
		t.Fatalf("upload status = %d key = %q", status, key)
	}

	status, body = do(t, env.router, http.MethodGet, "/api/v1/images", "", apiToken)
	if status != http.StatusOK {
		t.Fatalf("list status = %d (%s)", status, body)
	}
	var list struct {
		Data struct {
			Total int64 `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &list); err != nil || list.Data.Total != 1 {
		t.Fatalf("list total = %d (%s)", list.Data.Total, body)
	}

	other := env.token(t, "bob")
	status, body = do(t, env.router, http.MethodGet, "/api/v1/images", "", other)
	if status != http.StatusOK {
		t.Fatalf("bob list status = %d (%s)", status, body)
	}
	if err := json.Unmarshal(body, &list); err != nil || list.Data.Total != 0 {
		t.Fatalf("bob should see 0 images, got %d", list.Data.Total)
	}
}

func TestQuotaExceeded(t *testing.T) {
	env := newTestEnv(t, true, 1)
	token := env.token(t, "carol")
	status, _ := uploadPNG(t, env.router, testPNG(t, 16), token)
	if status != http.StatusInsufficientStorage {
		t.Fatalf("status = %d, want 507", status)
	}
}

func TestPresignReturnsNotImplementedForLocalStorage(t *testing.T) {
	env := newTestEnv(t, false, 1<<20)
	status, body := do(t, env.router, http.MethodPost, "/api/v1/upload/presign",
		`{"mime_type":"image/png","size":128}`, "")
	if status != http.StatusNotImplemented {
		t.Fatalf("status = %d (%s), want 501", status, body)
	}
}

func TestServeAppliesResizeAndCachesWithETag(t *testing.T) {
	env := newTestEnv(t, false, 1<<20)
	status, key := uploadPNG(t, env.router, testPNG(t, 8), "")
	if status != http.StatusOK {
		t.Fatalf("upload status = %d", status)
	}

	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/i/"+key+"?w=4&f=png", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("transform status = %d (%s)", rec.Code, rec.Body.String())
	}
	img, _, err := image.Decode(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("decode transformed image: %v", err)
	}
	if got := img.Bounds().Dx(); got != 4 {
		t.Fatalf("width = %d, want 4", got)
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}
	conditional := httptest.NewRequest(http.MethodGet, "/i/"+key+"?w=4&f=png", nil)
	conditional.Header.Set("If-None-Match", etag)
	notModified := httptest.NewRecorder()
	env.router.ServeHTTP(notModified, conditional)
	if notModified.Code != http.StatusNotModified {
		t.Fatalf("conditional status = %d, want 304", notModified.Code)
	}
}

func TestServeRejectsUnsupportedTransform(t *testing.T) {
	env := newTestEnv(t, false, 1<<20)
	status, key := uploadPNG(t, env.router, testPNG(t, 8), "")
	if status != http.StatusOK {
		t.Fatalf("upload status = %d", status)
	}
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/i/"+key+"?f=webp", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d (%s), want 400", rec.Code, rec.Body.String())
	}
}

func TestAdminRequiresAdminRole(t *testing.T) {
	env := newTestEnv(t, true, 1<<20)
	userToken := env.token(t, "dave")
	status, body := do(t, env.router, http.MethodGet, "/api/v1/admin/stats", "", userToken)
	if status != http.StatusForbidden {
		t.Fatalf("status = %d (%s), want 403", status, body)
	}
}

func TestAdminStatsUsersAndDisable(t *testing.T) {
	env := newTestEnv(t, true, 1<<20)
	adminToken, _ := env.adminToken(t, "root")
	victim, err := env.accounts.RegisterCustomer(context.Background(), "eve", "password123")
	if err != nil {
		t.Fatalf("RegisterCustomer victim: %v", err)
	}
	victimToken, err := env.accounts.CreateToken(context.Background(), victim.ID, "v")
	if err != nil {
		t.Fatalf("CreateToken victim: %v", err)
	}

	status, body := do(t, env.router, http.MethodGet, "/api/v1/admin/stats", "", adminToken)
	if status != http.StatusOK {
		t.Fatalf("stats status = %d (%s)", status, body)
	}
	var stats struct {
		Data struct {
			Admins        int64  `json:"admins"`
			Customers     int64  `json:"customers"`
			Users         int64  `json:"users"`
			StorageDriver string `json:"storage_driver"`
			Processor     string `json:"processor"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &stats); err != nil {
		t.Fatalf("stats decode: %v", err)
	}
	if stats.Data.Admins != 1 || stats.Data.Customers != 1 || stats.Data.Users != 2 ||
		stats.Data.StorageDriver != "local" || stats.Data.Processor == "" {
		t.Fatalf("unexpected stats: %s", body)
	}

	status, body = do(t, env.router, http.MethodGet, "/api/v1/admin/customers", "", adminToken)
	if status != http.StatusOK {
		t.Fatalf("customers status = %d (%s)", status, body)
	}
	status, body = do(t, env.router, http.MethodGet, "/api/v1/admin/admins", "", adminToken)
	if status != http.StatusOK {
		t.Fatalf("admins status = %d (%s)", status, body)
	}

	status, body = do(t, env.router, http.MethodPatch, "/api/v1/admin/customers/"+victim.ID, `{"disabled":true}`, adminToken)
	if status != http.StatusOK {
		t.Fatalf("disable status = %d (%s)", status, body)
	}
	var updated struct {
		Data struct {
			Disabled bool `json:"disabled"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &updated); err != nil || !updated.Data.Disabled {
		t.Fatalf("disable response: %s", body)
	}

	status, _ = do(t, env.router, http.MethodGet, "/api/v1/auth/me", "", victimToken.Token)
	if status != http.StatusUnauthorized {
		t.Fatalf("disabled token status = %d, want 401", status)
	}
}

func TestAdminCannotDisableSelf(t *testing.T) {
	env := newTestEnv(t, true, 1<<20)
	adminToken, adminID := env.adminToken(t, "root")
	status, body := do(t, env.router, http.MethodPatch, "/api/v1/admin/admins/"+adminID, `{"disabled":true}`, adminToken)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d (%s), want 400", status, body)
	}
}

func TestServeSupportsHead(t *testing.T) {
	env := newTestEnv(t, false, 1<<20)
	status, key := uploadPNG(t, env.router, testPNG(t, 8), "")
	if status != http.StatusOK {
		t.Fatalf("upload status = %d", status)
	}
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/i/"+key, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("HEAD status = %d, want 200", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("HEAD body length = %d, want 0", rec.Body.Len())
	}
	if rec.Header().Get("Content-Length") == "" {
		t.Fatal("missing Content-Length on HEAD")
	}
}

func TestServeRejectsCoverWithoutDimensions(t *testing.T) {
	env := newTestEnv(t, false, 1<<20)
	status, key := uploadPNG(t, env.router, testPNG(t, 8), "")
	if status != http.StatusOK {
		t.Fatalf("upload status = %d", status)
	}
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/i/"+key+"?fit=cover&w=4", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d (%s), want 400", rec.Code, rec.Body.String())
	}
}

// TestAlbumPlazaAndBatchFlow 覆盖相册 CRUD、批量设置可见性/相册以及图片广场。
func TestAlbumPlazaAndBatchFlow(t *testing.T) {
	env := newTestEnv(t, true, 1<<20)
	token := env.token(t, "frank")

	// 新建相册。
	status, body := do(t, env.router, http.MethodPost, "/api/v1/albums", `{"name":"旅行","intro":"2026"}`, token)
	if status != http.StatusCreated {
		t.Fatalf("create album status = %d (%s)", status, body)
	}
	var album struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &album); err != nil || album.Data.ID == "" {
		t.Fatalf("bad album response: %s", body)
	}

	// 上传图片并取回其 id。
	status, _ = uploadPNG(t, env.router, testPNG(t, 8), token)
	if status != http.StatusOK {
		t.Fatalf("upload status = %d", status)
	}
	status, body = do(t, env.router, http.MethodGet, "/api/v1/images", "", token)
	if status != http.StatusOK {
		t.Fatalf("list images status = %d (%s)", status, body)
	}
	var list struct {
		Data struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &list); err != nil || len(list.Data.Items) != 1 {
		t.Fatalf("bad image list: %s", body)
	}
	imageID := list.Data.Items[0].ID

	// 批量设为公开并归入相册。
	status, body = do(t, env.router, http.MethodPost, "/api/v1/images/batch",
		`{"ids":["`+imageID+`"],"permission":"public","album_id":"`+album.Data.ID+`"}`, token)
	if status != http.StatusOK {
		t.Fatalf("batch status = %d (%s)", status, body)
	}

	// 图片广场应包含该公开图片。
	status, body = do(t, env.router, http.MethodGet, "/api/v1/plaza", "", token)
	if status != http.StatusOK {
		t.Fatalf("plaza status = %d (%s)", status, body)
	}
	var plaza struct {
		Data struct {
			Total int64 `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &plaza); err != nil || plaza.Data.Total != 1 {
		t.Fatalf("plaza total = %d (%s)", plaza.Data.Total, body)
	}

	// 按相册过滤应只返回这一张。
	status, body = do(t, env.router, http.MethodGet, "/api/v1/images?album_id="+album.Data.ID, "", token)
	if status != http.StatusOK {
		t.Fatalf("album filter status = %d (%s)", status, body)
	}
	var filtered struct {
		Data struct {
			Total int64 `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &filtered); err != nil || filtered.Data.Total != 1 {
		t.Fatalf("filtered total = %d (%s)", filtered.Data.Total, body)
	}

	// 删除相册后图片仍存在，仅移出相册。
	status, body = do(t, env.router, http.MethodDelete, "/api/v1/albums/"+album.Data.ID, "", token)
	if status != http.StatusOK {
		t.Fatalf("delete album status = %d (%s)", status, body)
	}
	status, body = do(t, env.router, http.MethodGet, "/api/v1/images/"+imageID, "", token)
	if status != http.StatusOK {
		t.Fatalf("image gone after album delete: %d (%s)", status, body)
	}
}

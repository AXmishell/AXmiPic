package api_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axmipic/axmipic/internal/api"
	"github.com/axmipic/axmipic/internal/service"
	"github.com/axmipic/axmipic/internal/storage"
	"github.com/axmipic/axmipic/internal/store"
)

func newTestRouter(t *testing.T) (http.Handler, *storage.Local) {
	t.Helper()
	repo, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
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
	svc := service.NewUploadService(repo, local, service.UploadPolicy{
		MaxSizeBytes:     1 << 20,
		AllowedMIMETypes: []string{"image/png"},
		PresignExpiry:    10,
	})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return api.NewRouter(api.NewHandler(svc, local, 20, logger)), local
}

func TestHealthz(t *testing.T) {
	router, _ := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestPresignReturnsNotImplementedForLocalStorage(t *testing.T) {
	router, _ := newTestRouter(t)
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"mime_type":"image/png","size":128}`)
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/upload/presign", body))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d (%s), want 501", rec.Code, rec.Body.String())
	}
}

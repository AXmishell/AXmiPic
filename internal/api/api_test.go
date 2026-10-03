package api_test

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axmipic/axmipic/internal/api"
	"github.com/axmipic/axmipic/internal/imaging"
	"github.com/axmipic/axmipic/internal/service"
	"github.com/axmipic/axmipic/internal/storage"
	"github.com/axmipic/axmipic/internal/store"
)

func newTestRouter(t *testing.T) http.Handler {
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
	uploadSvc := service.NewUploadService(repo, local, service.UploadPolicy{
		MaxSizeBytes:     1 << 20,
		AllowedMIMETypes: []string{"image/png"},
		PresignExpiry:    10,
	})
	imagingSvc := service.NewImagingService(local, imaging.Default(), service.ProcessingPolicy{
		Enabled:        true,
		MaxWidth:       64,
		MaxHeight:      64,
		DefaultQuality: 82,
		AllowedFormats: []imaging.Format{imaging.FormatPNG, imaging.FormatJPEG},
	})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return api.NewRouter(api.NewHandler(uploadSvc, imagingSvc, local, 20, logger))
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

func uploadPNG(t *testing.T, router http.Handler, data []byte) string {
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
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload status = %d (%s)", rec.Code, rec.Body.String())
	}
	var env struct {
		Data struct {
			Key string `json:"key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	return env.Data.Key
}

func TestHealthz(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestPresignReturnsNotImplementedForLocalStorage(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"mime_type":"image/png","size":128}`)
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/upload/presign", body))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d (%s), want 501", rec.Code, rec.Body.String())
	}
}

func TestServeAppliesResizeAndCachesWithETag(t *testing.T) {
	router := newTestRouter(t)
	key := uploadPNG(t, router, testPNG(t, 8))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/i/"+key+"?w=4&f=png", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("transform status = %d (%s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("content-type = %q, want image/png", ct)
	}
	img, format, err := image.Decode(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("decode transformed image: %v", err)
	}
	if format != "png" {
		t.Fatalf("format = %q, want png", format)
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
	router.ServeHTTP(notModified, conditional)
	if notModified.Code != http.StatusNotModified {
		t.Fatalf("conditional status = %d, want 304", notModified.Code)
	}
}

func TestServeRejectsUnsupportedTransform(t *testing.T) {
	router := newTestRouter(t)
	key := uploadPNG(t, router, testPNG(t, 8))

	// webp cannot be encoded by the pure-Go processor, so the request is rejected.
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/i/"+key+"?f=webp", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d (%s), want 400", rec.Code, rec.Body.String())
	}
}

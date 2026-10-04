package service_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/AXmishell/axmipic/internal/service"
	"github.com/AXmishell/axmipic/internal/storage"
	"github.com/AXmishell/axmipic/internal/store"
)

type storedObject struct {
	data        []byte
	contentType string
}

// fakeStorage 是用于驱动 service 层测试的内存 Storage。
type fakeStorage struct {
	objects map[string]storedObject
}

func newFakeStorage() *fakeStorage {
	return &fakeStorage{objects: map[string]storedObject{}}
}

func (f *fakeStorage) Put(_ context.Context, key string, r io.Reader, _ int64, contentType string) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.objects[key] = storedObject{data: data, contentType: contentType}
	return nil
}

func (f *fakeStorage) Get(_ context.Context, key string) (io.ReadCloser, error) {
	obj, ok := f.objects[key]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(obj.data)), nil
}

func (f *fakeStorage) Delete(_ context.Context, key string) error {
	delete(f.objects, key)
	return nil
}

func (f *fakeStorage) Exists(_ context.Context, key string) (bool, error) {
	_, ok := f.objects[key]
	return ok, nil
}

func (f *fakeStorage) URL(key string) string {
	return "https://cdn.example.com/" + key
}

func (f *fakeStorage) Stat(_ context.Context, key string) (*storage.ObjectInfo, error) {
	obj, ok := f.objects[key]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return &storage.ObjectInfo{Key: key, Size: int64(len(obj.data)), ContentType: obj.contentType}, nil
}

// fakePresignStorage 额外支持预签名直传。
type fakePresignStorage struct {
	*fakeStorage
}

func (f *fakePresignStorage) PresignPut(_ context.Context, key string, opts storage.PresignOptions) (*storage.PresignedRequest, error) {
	return &storage.PresignedRequest{
		URL:       "https://upload.example.com",
		Method:    "POST",
		Fields:    map[string]string{"token": "tok", "key": key},
		ExpiresAt: time.Now().Add(opts.Expires),
	}, nil
}

func newRepo(t *testing.T) *store.Repository {
	t.Helper()
	repo, err := store.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() {
		_ = repo.Close()
	})
	return repo
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

func pngPolicy() service.UploadPolicy {
	return service.UploadPolicy{
		MaxSizeBytes:     1 << 20,
		AllowedMIMETypes: []string{"image/png"},
		PresignExpiry:    time.Hour,
	}
}

func TestUploadDeduplicatesIdenticalContent(t *testing.T) {
	repo := newRepo(t)
	svc := service.NewUploadService(repo, newFakeStorage(), pngPolicy())
	ctx := context.Background()
	data := testPNG(t)

	first, err := svc.Upload(ctx, nil, service.UploadInput{Data: data, MimeType: "image/png"})
	if err != nil {
		t.Fatalf("first Upload: %v", err)
	}
	second, err := svc.Upload(ctx, nil, service.UploadInput{Data: data, MimeType: "image/png"})
	if err != nil {
		t.Fatalf("second Upload: %v", err)
	}
	if first.ID != second.ID || first.Key != second.Key {
		t.Fatalf("expected dedup, got %s/%s and %s/%s", first.ID, first.Key, second.ID, second.Key)
	}
	if first.Width != 8 || first.Height != 8 {
		t.Fatalf("dimensions = %dx%d, want 8x8", first.Width, first.Height)
	}
}

func TestPresignUnsupportedByNonPresigningStorage(t *testing.T) {
	repo := newRepo(t)
	svc := service.NewUploadService(repo, newFakeStorage(), pngPolicy())
	_, err := svc.Presign(context.Background(), nil, service.PresignInput{MimeType: "image/png", Size: 10})
	if !errors.Is(err, service.ErrPresignUnsupported) {
		t.Fatalf("error = %v, want ErrPresignUnsupported", err)
	}
}

func TestPresignRecordsPendingUpload(t *testing.T) {
	repo := newRepo(t)
	svc := service.NewUploadService(repo, &fakePresignStorage{newFakeStorage()}, pngPolicy())
	ctx := context.Background()

	result, err := svc.Presign(ctx, nil, service.PresignInput{MimeType: "image/png", Size: 128})
	if err != nil {
		t.Fatalf("Presign: %v", err)
	}
	if result.Key == "" || result.UploadURL == "" {
		t.Fatalf("incomplete presign result: %+v", result)
	}
	if _, err := repo.GetPendingUpload(ctx, result.Key); err != nil {
		t.Fatalf("pending upload not recorded: %v", err)
	}
}

func TestConfirmRecordsMetadataAndClearsPending(t *testing.T) {
	repo := newRepo(t)
	fs := newFakeStorage()
	const key = "2026/01/01/abc.png"
	fs.objects[key] = storedObject{data: testPNG(t), contentType: "image/png"}
	ctx := context.Background()
	if err := repo.CreatePendingUpload(ctx, &store.PendingUpload{
		Key:       key,
		MimeType:  "image/png",
		MaxSize:   1 << 20,
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("CreatePendingUpload: %v", err)
	}
	svc := service.NewUploadService(repo, fs, pngPolicy())

	dto, err := svc.Confirm(ctx, nil, key)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if dto.Key != key || dto.MimeType != "image/png" {
		t.Fatalf("unexpected dto: %+v", dto)
	}
	if dto.Width != 8 || dto.Height != 8 {
		t.Fatalf("dimensions = %dx%d, want 8x8", dto.Width, dto.Height)
	}
	if _, err := repo.GetPendingUpload(ctx, key); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("pending upload not cleared: %v", err)
	}

	again, err := svc.Confirm(ctx, nil, key)
	if err != nil {
		t.Fatalf("Confirm (idempotent): %v", err)
	}
	if again.ID != dto.ID {
		t.Fatalf("confirm not idempotent: %s != %s", again.ID, dto.ID)
	}
}

func TestConfirmRejectsUnissuedKey(t *testing.T) {
	repo := newRepo(t)
	svc := service.NewUploadService(repo, newFakeStorage(), pngPolicy())
	_, err := svc.Confirm(context.Background(), nil, "2026/01/01/never-issued.png")
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestConfirmRejectsDisallowedContentType(t *testing.T) {
	repo := newRepo(t)
	fs := newFakeStorage()
	const key = "2026/01/01/evil.png"
	fs.objects[key] = storedObject{data: []byte("not really an image"), contentType: "application/octet-stream"}
	ctx := context.Background()
	if err := repo.CreatePendingUpload(ctx, &store.PendingUpload{
		Key:       key,
		MimeType:  "image/png",
		MaxSize:   1 << 20,
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("CreatePendingUpload: %v", err)
	}
	svc := service.NewUploadService(repo, fs, pngPolicy())

	_, err := svc.Confirm(ctx, nil, key)
	if !errors.Is(err, service.ErrUnsupportedType) {
		t.Fatalf("error = %v, want ErrUnsupportedType", err)
	}
	if _, ok := fs.objects[key]; ok {
		t.Fatal("rejected object was not removed from storage")
	}
}

func TestConfirmRejectsSpoofedContent(t *testing.T) {
	repo := newRepo(t)
	fs := newFakeStorage()
	const key = "2026/01/01/spoof.png"
	// 声明的类型是允许的，但这些字节并不是图片。
	fs.objects[key] = storedObject{data: []byte("this is not an image"), contentType: "image/png"}
	ctx := context.Background()
	if err := repo.CreatePendingUpload(ctx, &store.PendingUpload{
		Key:       key,
		MimeType:  "image/png",
		MaxSize:   1 << 20,
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("CreatePendingUpload: %v", err)
	}
	svc := service.NewUploadService(repo, fs, pngPolicy())

	_, err := svc.Confirm(ctx, nil, key)
	if !errors.Is(err, service.ErrUnsupportedType) {
		t.Fatalf("error = %v, want ErrUnsupportedType", err)
	}
	if _, ok := fs.objects[key]; ok {
		t.Fatal("spoofed object was not removed from storage")
	}
}

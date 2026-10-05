package service_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/png"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/moderation"
	"github.com/AXmishell/axmipic/internal/service"
	"github.com/AXmishell/axmipic/internal/storage"
	"github.com/AXmishell/axmipic/internal/store"
)

// contentHashForTest 复算 sha256，便于在测试中断言。
func contentHashForTest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// testPNGSize 生成指定边长的 PNG，用于构造彼此不同的内容。
func testPNGSize(t *testing.T, size int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

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

// managerWithFallback 用一个兜底后端构建 Manager，模拟配置文件的存储。
func managerWithFallback(t *testing.T, store storage.Storage) *storage.Manager {
	t.Helper()
	m := storage.NewManager()
	m.SetFallback(store)
	return m
}

func TestUploadDeduplicatesIdenticalContent(t *testing.T) {
	repo := newRepo(t)
	svc := service.NewUploadService(repo, managerWithFallback(t, newFakeStorage()), pngPolicy())
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

func TestUploadDedupIsPerOwner(t *testing.T) {
	repo := newRepo(t)
	svc := service.NewUploadService(repo, managerWithFallback(t, newFakeStorage()), pngPolicy())
	ctx := context.Background()
	newCustomer(t, repo, "u1")
	newCustomer(t, repo, "u2")
	u1 := &auth.Principal{UserID: "u1", Username: "u1", Role: auth.RoleUser}
	u2 := &auth.Principal{UserID: "u2", Username: "u2", Role: auth.RoleUser}
	data := testPNG(t)

	first, err := svc.Upload(ctx, u1, service.UploadInput{Data: data, MimeType: "image/png"})
	if err != nil {
		t.Fatalf("u1 first Upload: %v", err)
	}
	// 同一所有者重复上传相同内容应去重。
	again, err := svc.Upload(ctx, u1, service.UploadInput{Data: data, MimeType: "image/png"})
	if err != nil {
		t.Fatalf("u1 second Upload: %v", err)
	}
	if again.ID != first.ID || again.Key != first.Key {
		t.Fatalf("same-owner dedup failed: %s vs %s", again.ID, first.ID)
	}
	// 不同所有者上传相同内容应各自持有独立记录与对象，不得共享。
	second, err := svc.Upload(ctx, u2, service.UploadInput{Data: data, MimeType: "image/png"})
	if err != nil {
		t.Fatalf("u2 Upload: %v", err)
	}
	if second.ID == first.ID || second.Key == first.Key {
		t.Fatalf("cross-owner content should not be shared: %s/%s vs %s/%s", first.ID, first.Key, second.ID, second.Key)
	}
	if second.Hash != first.Hash {
		t.Fatalf("hash = %q, want %q", second.Hash, first.Hash)
	}
}

func TestUploadRenamesAndRecordsOriginalNameAndHash(t *testing.T) {
	repo := newRepo(t)
	svc := service.NewUploadService(repo, managerWithFallback(t, newFakeStorage()), pngPolicy())
	ctx := context.Background()
	data := testPNG(t)

	dto, err := svc.Upload(ctx, nil, service.UploadInput{
		Data:         data,
		MimeType:     "image/png",
		OriginalName: "我的 照片.PNG",
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	// 存储键与文件名应为随机命名，而非原始名或内容哈希。
	wantHash := contentHashForTest(data)
	if dto.Hash != wantHash {
		t.Fatalf("hash = %q, want %q", dto.Hash, wantHash)
	}
	if !strings.HasSuffix(dto.Filename, ".png") || strings.Contains(dto.Filename, wantHash) {
		t.Fatalf("filename = %q, want a random .png name", dto.Filename)
	}
	if !strings.HasSuffix(dto.Key, "/"+dto.Filename) {
		t.Fatalf("key = %q should end with filename %q", dto.Key, dto.Filename)
	}
	if strings.Contains(dto.Key, wantHash) || strings.Contains(dto.Key, "我的") {
		t.Fatalf("key %q should be random and not leak content hash or original name", dto.Key)
	}
	// 原文件名应被保留。
	if dto.OriginalName != "我的 照片.PNG" {
		t.Fatalf("original_name = %q", dto.OriginalName)
	}
}

func TestSanitizeOriginalNameStripsPaths(t *testing.T) {
	repo := newRepo(t)
	svc := service.NewUploadService(repo, managerWithFallback(t, newFakeStorage()), pngPolicy())
	ctx := context.Background()

	cases := []struct {
		in   string
		want string
	}{
		{"../../etc/passwd", "passwd"},
		{"C:\\Users\\a\\pic.png", "pic.png"},
		{"  spaced.png  ", "spaced.png"},
		{"", ""},
	}
	for i, tc := range cases {
		// 每例使用不同尺寸的图片，避免内容去重命中同一条记录。
		dto, err := svc.Upload(ctx, nil, service.UploadInput{
			Data:         testPNGSize(t, 8+i),
			MimeType:     "image/png",
			OriginalName: tc.in,
		})
		if err != nil {
			t.Fatalf("Upload(%q): %v", tc.in, err)
		}
		if dto.OriginalName != tc.want {
			t.Fatalf("sanitize(%q) = %q, want %q", tc.in, dto.OriginalName, tc.want)
		}
	}
}

func TestRenameUpdatesOriginalNameOnly(t *testing.T) {
	repo := newRepo(t)
	svc := service.NewUploadService(repo, managerWithFallback(t, newFakeStorage()), pngPolicy())
	ctx := context.Background()

	dto, err := svc.Upload(ctx, nil, service.UploadInput{
		Data:         testPNG(t),
		MimeType:     "image/png",
		OriginalName: "before.png",
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	renamed, err := svc.Rename(ctx, nil, dto.ID, "重命名后.png")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if renamed.OriginalName != "重命名后.png" {
		t.Fatalf("original_name = %q", renamed.OriginalName)
	}
	// 存储键、存储文件名与哈希必须保持不变。
	if renamed.Key != dto.Key || renamed.Filename != dto.Filename || renamed.Hash != dto.Hash {
		t.Fatalf("storage identity changed: %+v vs %+v", renamed, dto)
	}

	if _, err := svc.Rename(ctx, nil, dto.ID, "   "); !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("empty name error = %v, want ErrInvalidInput", err)
	}
}

func TestListOrderAndKeyword(t *testing.T) {
	repo := newRepo(t)
	svc := service.NewUploadService(repo, managerWithFallback(t, newFakeStorage()), pngPolicy())
	ctx := context.Background()

	small, err := svc.Upload(ctx, nil, service.UploadInput{Data: testPNGSize(t, 8), MimeType: "image/png", OriginalName: "small.png"})
	if err != nil {
		t.Fatalf("upload small: %v", err)
	}
	large, err := svc.Upload(ctx, nil, service.UploadInput{Data: testPNGSize(t, 32), MimeType: "image/png", OriginalName: "large.png"})
	if err != nil {
		t.Fatalf("upload large: %v", err)
	}

	largest, err := svc.List(ctx, &auth.Principal{Role: auth.RoleAdmin}, 1, 20, service.ImageFilter{Order: "largest"})
	if err != nil {
		t.Fatalf("List largest: %v", err)
	}
	if len(largest.Items) != 2 || largest.Items[0].ID != large.ID {
		t.Fatalf("largest order wrong: %+v", largest.Items)
	}

	smallest, err := svc.List(ctx, &auth.Principal{Role: auth.RoleAdmin}, 1, 20, service.ImageFilter{Order: "smallest"})
	if err != nil {
		t.Fatalf("List smallest: %v", err)
	}
	if smallest.Items[0].ID != small.ID {
		t.Fatalf("smallest order wrong")
	}

	found, err := svc.List(ctx, &auth.Principal{Role: auth.RoleAdmin}, 1, 20, service.ImageFilter{Keyword: "large"})
	if err != nil {
		t.Fatalf("List keyword: %v", err)
	}
	if found.Total != 1 || found.Items[0].ID != large.ID {
		t.Fatalf("keyword search wrong: total=%d", found.Total)
	}
}

func TestPresignUnsupportedByNonPresigningStorage(t *testing.T) {
	repo := newRepo(t)
	svc := service.NewUploadService(repo, managerWithFallback(t, newFakeStorage()), pngPolicy())
	_, err := svc.Presign(context.Background(), nil, service.PresignInput{MimeType: "image/png", Size: 10})
	if !errors.Is(err, service.ErrPresignUnsupported) {
		t.Fatalf("error = %v, want ErrPresignUnsupported", err)
	}
}

func TestPresignRecordsPendingUpload(t *testing.T) {
	repo := newRepo(t)
	svc := service.NewUploadService(repo, managerWithFallback(t, &fakePresignStorage{newFakeStorage()}), pngPolicy())
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
	svc := service.NewUploadService(repo, managerWithFallback(t, fs), pngPolicy())

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
	svc := service.NewUploadService(repo, managerWithFallback(t, newFakeStorage()), pngPolicy())
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
	svc := service.NewUploadService(repo, managerWithFallback(t, fs), pngPolicy())

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
	svc := service.NewUploadService(repo, managerWithFallback(t, fs), pngPolicy())

	_, err := svc.Confirm(ctx, nil, key)
	if !errors.Is(err, service.ErrUnsupportedType) {
		t.Fatalf("error = %v, want ErrUnsupportedType", err)
	}
	if _, ok := fs.objects[key]; ok {
		t.Fatal("spoofed object was not removed from storage")
	}
}

// TestUploadGuestIPQuota 验证匿名访客按 IP 的窗口配额：同一 IP 超出后拒绝，
// 其他 IP 不受影响，非访客主体不受该限制。
func TestUploadGuestIPQuota(t *testing.T) {
	repo := newRepo(t)
	svc := service.NewUploadService(repo, managerWithFallback(t, newFakeStorage()), pngPolicy())

	data := testPNGSize(t, 8)
	other := testPNGSize(t, 9)
	svc.SetGuestIPQuota(int64(len(data)), time.Hour)

	guest := &auth.Principal{Guest: true}
	ipA := service.WithClientIP(context.Background(), "203.0.113.9")

	if _, err := svc.Upload(ipA, guest, service.UploadInput{Data: data, MimeType: "image/png"}); err != nil {
		t.Fatalf("first upload: %v", err)
	}
	if _, err := svc.Upload(ipA, guest, service.UploadInput{Data: other, MimeType: "image/png"}); !errors.Is(err, service.ErrGuestQuotaExceeded) {
		t.Fatalf("second upload error = %v, want ErrGuestQuotaExceeded", err)
	}

	// 另一个 IP 仍有独立额度。
	ipB := service.WithClientIP(context.Background(), "203.0.113.10")
	if _, err := svc.Upload(ipB, guest, service.UploadInput{Data: data, MimeType: "image/png"}); err != nil {
		t.Fatalf("upload from another ip: %v", err)
	}

	// 非访客主体不受 IP 配额限制。
	user := &auth.Principal{Role: auth.RoleUser}
	if _, err := svc.Upload(ipA, user, service.UploadInput{Data: other, MimeType: "image/png"}); err != nil {
		t.Fatalf("non-guest upload: %v", err)
	}
}

// stubModerator 是测试用的审查器，按回调返回结论。
type stubModerator struct {
	fn func(data []byte) (moderation.Decision, error)
}

func (s stubModerator) Review(_ context.Context, data []byte, _ string) (moderation.Decision, error) {
	return s.fn(data)
}

// TestSetPermissionModeration 验证设为公开时逐张审查：不通过的保持私有，通过的
// 才公开。
func TestSetPermissionModeration(t *testing.T) {
	repo := newRepo(t)
	svc := service.NewUploadService(repo, managerWithFallback(t, newFakeStorage()), pngPolicy())
	ctx := context.Background()
	newCustomer(t, repo, "u1")
	u1 := &auth.Principal{UserID: "u1", Username: "u1", Role: auth.RoleUser}

	good := testPNGSize(t, 8)
	bad := testPNGSize(t, 9)
	g1, err := svc.Upload(ctx, u1, service.UploadInput{Data: good, MimeType: "image/png"})
	if err != nil {
		t.Fatalf("upload good: %v", err)
	}
	b1, err := svc.Upload(ctx, u1, service.UploadInput{Data: bad, MimeType: "image/png"})
	if err != nil {
		t.Fatalf("upload bad: %v", err)
	}

	svc.SetModerator(stubModerator{fn: func(data []byte) (moderation.Decision, error) {
		if len(data) == len(bad) {
			return moderation.Decision{Allowed: false, Reason: "违规"}, nil
		}
		return moderation.Decision{Allowed: true, Reason: "SAFE"}, nil
	}}, 0)

	res, err := svc.SetPermission(ctx, u1, []string{g1.ID, b1.ID}, store.PermissionPublic)
	if err != nil {
		t.Fatalf("SetPermission: %v", err)
	}
	if res.Published != 1 || len(res.Blocked) != 1 || res.Blocked[0] != b1.ID {
		t.Fatalf("result = %+v, want published 1 and blocked %s", res, b1.ID)
	}

	images, err := repo.ListImagesByIDs(ctx, []string{g1.ID, b1.ID})
	if err != nil {
		t.Fatalf("ListImagesByIDs: %v", err)
	}
	for i := range images {
		want := store.PermissionPrivate
		if images[i].ID == g1.ID {
			want = store.PermissionPublic
		}
		if images[i].Permission != want {
			t.Fatalf("image %s permission = %q, want %q", images[i].ID, images[i].Permission, want)
		}
	}
}

// TestSetPermissionModerationFailClosed 验证审查调用失败（无返回）时按违规处理。
func TestSetPermissionModerationFailClosed(t *testing.T) {
	repo := newRepo(t)
	svc := service.NewUploadService(repo, managerWithFallback(t, newFakeStorage()), pngPolicy())
	ctx := context.Background()
	newCustomer(t, repo, "u1")
	u1 := &auth.Principal{UserID: "u1", Username: "u1", Role: auth.RoleUser}

	dto, err := svc.Upload(ctx, u1, service.UploadInput{Data: testPNGSize(t, 8), MimeType: "image/png"})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	svc.SetModerator(stubModerator{fn: func([]byte) (moderation.Decision, error) {
		return moderation.Decision{}, errors.New("boom")
	}}, 0)

	res, err := svc.SetPermission(ctx, u1, []string{dto.ID}, store.PermissionPublic)
	if err != nil {
		t.Fatalf("SetPermission: %v", err)
	}
	if res.Published != 0 || len(res.Blocked) != 1 {
		t.Fatalf("result = %+v, want blocked", res)
	}
	img, err := repo.GetByKey(ctx, dto.Key)
	if err != nil {
		t.Fatalf("GetByKey: %v", err)
	}
	if img.Permission != store.PermissionPrivate {
		t.Fatalf("permission = %q, want private (fail-closed)", img.Permission)
	}
}

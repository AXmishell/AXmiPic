package service_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/imaging"
	"github.com/AXmishell/axmipic/internal/service"
	"github.com/AXmishell/axmipic/internal/store"
)

// TestThumbnailURLAndCursorPagination 验证 DTO 携带经本实例处理的缩略图 URL，
// 且 keyset 游标翻页不漏不重。
func TestThumbnailURLAndCursorPagination(t *testing.T) {
	repo := newRepo(t)
	svc := service.NewUploadService(repo, managerWithFallback(t, newFakeStorage()), pngPolicy())
	svc.SetMediaBaseURL("http://localhost:8080/")
	ctx := context.Background()
	principal := &auth.Principal{Role: auth.RoleAdmin}

	for i := 0; i < 3; i++ {
		dto, err := svc.Upload(ctx, principal, service.UploadInput{
			Data:         testPNGSize(t, 8+i*4),
			MimeType:     "image/png",
			OriginalName: fmt.Sprintf("img-%d.png", i),
		})
		if err != nil {
			t.Fatalf("upload %d: %v", i, err)
		}
		if !strings.HasPrefix(dto.Thumbnail, "http://localhost:8080/i/") ||
			!strings.Contains(dto.Thumbnail, "w=480") {
			t.Fatalf("thumbnail url = %q", dto.Thumbnail)
		}
	}

	first, err := svc.List(ctx, principal, 1, 2, service.ImageFilter{Order: "newest"})
	if err != nil {
		t.Fatalf("List page 1: %v", err)
	}
	if len(first.Items) != 2 {
		t.Fatalf("page 1 items = %d, want 2", len(first.Items))
	}
	if first.NextCursor == "" {
		t.Fatalf("expected a next cursor for a full page")
	}

	second, err := svc.List(ctx, principal, 1, 2, service.ImageFilter{Order: "newest", Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("List page 2: %v", err)
	}
	if len(second.Items) != 1 {
		t.Fatalf("page 2 items = %d, want 1", len(second.Items))
	}

	seen := map[string]bool{}
	for _, item := range append(append([]service.ImageDTO{}, first.Items...), second.Items...) {
		if seen[item.ID] {
			t.Fatalf("duplicate image %s across cursor pages", item.ID)
		}
		seen[item.ID] = true
	}
	if len(seen) != 3 {
		t.Fatalf("unique images across pages = %d, want 3", len(seen))
	}
}

// TestDeleteBatchChecksOwnership 验证批量删除会校验所有权并跳过未知 id。
func TestDeleteBatchChecksOwnership(t *testing.T) {
	repo := newRepo(t)
	svc := service.NewUploadService(repo, managerWithFallback(t, newFakeStorage()), pngPolicy())
	ctx := context.Background()
	newCustomer(t, repo, "u1")
	newCustomer(t, repo, "u2")
	u1 := &auth.Principal{UserID: "u1", Username: "u1", Role: auth.RoleUser}
	u2 := &auth.Principal{UserID: "u2", Username: "u2", Role: auth.RoleUser}

	first, err := svc.Upload(ctx, u1, service.UploadInput{Data: testPNGSize(t, 8), MimeType: "image/png"})
	if err != nil {
		t.Fatalf("upload 1: %v", err)
	}
	second, err := svc.Upload(ctx, u1, service.UploadInput{Data: testPNGSize(t, 16), MimeType: "image/png"})
	if err != nil {
		t.Fatalf("upload 2: %v", err)
	}

	deleted, failed, err := svc.DeleteBatch(ctx, u1, []string{first.ID, second.ID, "does-not-exist"})
	if err != nil {
		t.Fatalf("DeleteBatch: %v", err)
	}
	if deleted != 2 || failed != 0 {
		t.Fatalf("deleted=%d failed=%d, want 2/0", deleted, failed)
	}

	// 另一个用户不能删除 u1 的图片。
	third, err := svc.Upload(ctx, u1, service.UploadInput{Data: testPNGSize(t, 24), MimeType: "image/png"})
	if err != nil {
		t.Fatalf("upload 3: %v", err)
	}
	if _, _, err := svc.DeleteBatch(ctx, u2, []string{third.ID}); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("cross-owner DeleteBatch error = %v, want ErrForbidden", err)
	}
}

// TestDeleteCustomerPurgesContent 验证删除客户会级联清理其图片、对象与相册。
func TestDeleteCustomerPurgesContent(t *testing.T) {
	repo := newRepo(t)
	fake := newFakeStorage()
	upload := service.NewUploadService(repo, managerWithFallback(t, fake), pngPolicy())
	admin := service.NewAdminService(repo, "local", imaging.Default())
	admin.SetImagePurger(upload)
	ctx := context.Background()
	newCustomer(t, repo, "u1")
	u1 := &auth.Principal{UserID: "u1", Username: "u1", Role: auth.RoleUser}

	dto, err := upload.Upload(ctx, u1, service.UploadInput{Data: testPNG(t), MimeType: "image/png"})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	owner := "u1"
	if err := repo.CreateAlbum(ctx, &store.Album{ID: "a1", UserID: &owner, Name: "A"}); err != nil {
		t.Fatalf("CreateAlbum: %v", err)
	}

	if err := admin.DeleteCustomer(ctx, "u1"); err != nil {
		t.Fatalf("DeleteCustomer: %v", err)
	}
	if _, ok := fake.objects[dto.Key]; ok {
		t.Fatalf("storage object %q was not purged", dto.Key)
	}
	if _, err := repo.GetByID(ctx, dto.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("image record still present: %v", err)
	}
	if _, err := repo.GetAccountByID(ctx, store.RoleCustomer, "u1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("customer record still present: %v", err)
	}
	albums, err := repo.ListAlbums(ctx, "u1")
	if err != nil {
		t.Fatalf("ListAlbums: %v", err)
	}
	if len(albums) != 0 {
		t.Fatalf("albums after purge = %d, want 0", len(albums))
	}
}

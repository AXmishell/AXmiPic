package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/service"
	"github.com/AXmishell/axmipic/internal/store"
)

// newCustomer 插入一个普通账户，使配额相关逻辑可以正常执行。
func newCustomer(t *testing.T, repo *store.Repository, id string) {
	t.Helper()
	if err := repo.CreateCustomer(context.Background(), &store.Customer{
		ID:           id,
		Username:     id,
		PasswordHash: "x",
	}); err != nil {
		t.Fatalf("CreateCustomer(%q): %v", id, err)
	}
}

func TestAlbumCRUDAndOwnership(t *testing.T) {
	repo := newRepo(t)
	svc := service.NewAlbumService(repo)
	ctx := context.Background()
	u1 := &auth.Principal{UserID: "u1", Role: auth.RoleUser}
	u2 := &auth.Principal{UserID: "u2", Role: auth.RoleUser}

	album, err := svc.Create(ctx, u1, service.AlbumInput{Name: "  旅行  ", Intro: " 2026 "})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if album.Name != "旅行" || album.Intro != "2026" {
		t.Fatalf("input not normalized: %+v", album)
	}

	// 用户只能看到自己的相册。
	if list, err := svc.List(ctx, u1); err != nil || len(list) != 1 {
		t.Fatalf("u1 list = %+v, err = %v", list, err)
	}
	if list, err := svc.List(ctx, u2); err != nil || len(list) != 0 {
		t.Fatalf("u2 list = %+v, err = %v", list, err)
	}

	// u2 不能读取/修改/删除 u1 的相册。
	if _, err := svc.Get(ctx, u2, album.ID); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("u2 Get err = %v, want ErrForbidden", err)
	}
	if _, err := svc.Update(ctx, u2, album.ID, service.AlbumInput{Name: "x"}); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("u2 Update err = %v, want ErrForbidden", err)
	}
	if err := svc.Delete(ctx, u2, album.ID); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("u2 Delete err = %v, want ErrForbidden", err)
	}

	// 校验失败：空名称。
	if _, err := svc.Create(ctx, u1, service.AlbumInput{Name: "   "}); !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("empty name err = %v, want ErrInvalidInput", err)
	}

	// 有效的更新。
	updated, err := svc.Update(ctx, u1, album.ID, service.AlbumInput{Name: "旅行计划", Intro: "下次"})
	if err != nil || updated.Name != "旅行计划" || updated.Intro != "下次" {
		t.Fatalf("Update = %+v, err = %v", updated, err)
	}

	// 删除后不可再访问。
	if err := svc.Delete(ctx, u1, album.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(ctx, u1, album.ID); !errors.Is(err, service.ErrAlbumNotFound) {
		t.Fatalf("Get after delete err = %v, want ErrAlbumNotFound", err)
	}
}

func TestImagePermissionAndAlbumAssignment(t *testing.T) {
	repo := newRepo(t)
	newCustomer(t, repo, "u1")
	newCustomer(t, repo, "u2")
	upload := service.NewUploadService(repo, managerWithFallback(t, newFakeStorage()), pngPolicy())
	albums := service.NewAlbumService(repo)
	ctx := context.Background()
	u1 := &auth.Principal{UserID: "u1", Role: auth.RoleUser}
	u2 := &auth.Principal{UserID: "u2", Role: auth.RoleUser}

	img, err := upload.Upload(ctx, u1, service.UploadInput{Data: testPNGSize(t, 8), MimeType: "image/png"})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if img.Permission != store.PermissionPrivate {
		t.Fatalf("default permission = %q, want private", img.Permission)
	}

	if err := upload.SetPermission(ctx, u1, []string{img.ID}, store.PermissionPublic); err != nil {
		t.Fatalf("SetPermission: %v", err)
	}
	got, err := upload.Get(ctx, u1, img.ID)
	if err != nil || got.Permission != store.PermissionPublic {
		t.Fatalf("permission = %q, err = %v", got.Permission, err)
	}

	if err := upload.SetPermission(ctx, u1, []string{img.ID}, "hidden"); !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("bad permission err = %v, want ErrInvalidInput", err)
	}
	if err := upload.SetPermission(ctx, u2, []string{img.ID}, store.PermissionPrivate); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("cross-user SetPermission err = %v, want ErrForbidden", err)
	}

	album, err := albums.Create(ctx, u1, service.AlbumInput{Name: "A"})
	if err != nil {
		t.Fatalf("Create album: %v", err)
	}
	if err := upload.SetAlbum(ctx, u1, []string{img.ID}, &album.ID); err != nil {
		t.Fatalf("SetAlbum: %v", err)
	}
	got, err = upload.Get(ctx, u1, img.ID)
	if err != nil || got.AlbumID != album.ID {
		t.Fatalf("album id = %q, err = %v", got.AlbumID, err)
	}

	// 移出相册。
	if err := upload.SetAlbum(ctx, u1, []string{img.ID}, nil); err != nil {
		t.Fatalf("clear album: %v", err)
	}
	if got, _ = upload.Get(ctx, u1, img.ID); got.AlbumID != "" {
		t.Fatalf("album id after clear = %q, want empty", got.AlbumID)
	}

	// 不能把图片放进别人的相册。
	other, err := albums.Create(ctx, u2, service.AlbumInput{Name: "B"})
	if err != nil {
		t.Fatalf("Create other album: %v", err)
	}
	if err := upload.SetAlbum(ctx, u1, []string{img.ID}, &other.ID); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("cross-user SetAlbum err = %v, want ErrForbidden", err)
	}
}

func TestListPlazaOnlyPublic(t *testing.T) {
	repo := newRepo(t)
	newCustomer(t, repo, "u1")
	upload := service.NewUploadService(repo, managerWithFallback(t, newFakeStorage()), pngPolicy())
	ctx := context.Background()
	u1 := &auth.Principal{UserID: "u1", Role: auth.RoleUser}

	public, err := upload.Upload(ctx, u1, service.UploadInput{Data: testPNGSize(t, 8), MimeType: "image/png"})
	if err != nil {
		t.Fatalf("Upload public: %v", err)
	}
	if _, err := upload.Upload(ctx, u1, service.UploadInput{Data: testPNGSize(t, 16), MimeType: "image/png"}); err != nil {
		t.Fatalf("Upload private: %v", err)
	}
	if err := upload.SetPermission(ctx, u1, []string{public.ID}, store.PermissionPublic); err != nil {
		t.Fatalf("SetPermission: %v", err)
	}

	plaza, err := upload.ListPlaza(ctx, 1, 20, service.ImageFilter{})
	if err != nil {
		t.Fatalf("ListPlaza: %v", err)
	}
	if plaza.Total != 1 || len(plaza.Items) != 1 || plaza.Items[0].ID != public.ID {
		t.Fatalf("plaza = %+v, want only the public image", plaza)
	}
}

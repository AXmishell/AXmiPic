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

func TestPublicAlbumVisibilityAndProfile(t *testing.T) {
	repo := newRepo(t)
	svc := service.NewAlbumService(repo)
	ctx := context.Background()
	newCustomer(t, repo, "u1")
	u1 := &auth.Principal{UserID: "u1", Username: "u1", Role: auth.RoleUser}
	guest := &auth.Principal{Guest: true}

	privateAlbum, err := svc.Create(ctx, u1, service.AlbumInput{Name: "私密"})
	if err != nil {
		t.Fatalf("Create private: %v", err)
	}
	if privateAlbum.Permission != store.PermissionPrivate {
		t.Fatalf("default permission = %q, want private", privateAlbum.Permission)
	}
	if _, err := svc.Get(ctx, guest, privateAlbum.ID); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("guest Get private err = %v, want ErrForbidden", err)
	}

	publicAlbum, err := svc.Create(ctx, u1, service.AlbumInput{Name: "公开", Permission: store.PermissionPublic})
	if err != nil {
		t.Fatalf("Create public: %v", err)
	}
	if _, err := svc.Get(ctx, guest, publicAlbum.ID); err != nil {
		t.Fatalf("guest Get public: %v", err)
	}

	list, err := svc.ListPublic(ctx, "")
	if err != nil {
		t.Fatalf("ListPublic: %v", err)
	}
	if len(list) != 1 || list[0].ID != publicAlbum.ID || list[0].OwnerUsername != "u1" {
		t.Fatalf("public albums = %+v", list)
	}

	profile, err := svc.PublicProfile(ctx, "u1")
	if err != nil {
		t.Fatalf("PublicProfile: %v", err)
	}
	if profile.Username != "u1" || len(profile.PublicAlbums) != 1 {
		t.Fatalf("profile = %+v", profile)
	}
	if _, err := svc.PublicProfile(ctx, "missing"); !errors.Is(err, service.ErrUserNotFound) {
		t.Fatalf("missing profile err = %v, want ErrUserNotFound", err)
	}
}

func TestPlazaOwnerAndUserFilter(t *testing.T) {
	repo := newRepo(t)
	newCustomer(t, repo, "u1")
	newCustomer(t, repo, "u2")
	upload := service.NewUploadService(repo, managerWithFallback(t, newFakeStorage()), pngPolicy())
	ctx := context.Background()
	u1 := &auth.Principal{UserID: "u1", Username: "u1", Role: auth.RoleUser}
	u2 := &auth.Principal{UserID: "u2", Username: "u2", Role: auth.RoleUser}

	img1, err := upload.Upload(ctx, u1, service.UploadInput{Data: testPNGSize(t, 8), MimeType: "image/png"})
	if err != nil {
		t.Fatalf("Upload u1: %v", err)
	}
	img2, err := upload.Upload(ctx, u2, service.UploadInput{Data: testPNGSize(t, 16), MimeType: "image/png"})
	if err != nil {
		t.Fatalf("Upload u2: %v", err)
	}
	if err := upload.SetPermission(ctx, u1, []string{img1.ID}, store.PermissionPublic); err != nil {
		t.Fatalf("SetPermission u1: %v", err)
	}
	if err := upload.SetPermission(ctx, u2, []string{img2.ID}, store.PermissionPublic); err != nil {
		t.Fatalf("SetPermission u2: %v", err)
	}

	plaza, err := upload.ListPlaza(ctx, 1, 20, service.ImageFilter{})
	if err != nil {
		t.Fatalf("ListPlaza: %v", err)
	}
	if plaza.Total != 2 {
		t.Fatalf("plaza total = %d, want 2", plaza.Total)
	}
	for _, item := range plaza.Items {
		if item.OwnerUsername == "" {
			t.Fatalf("plaza item missing owner username: %+v", item)
		}
	}

	filtered, err := upload.ListPlaza(ctx, 1, 20, service.ImageFilter{UserID: "u1"})
	if err != nil {
		t.Fatalf("ListPlaza filtered: %v", err)
	}
	if filtered.Total != 1 || filtered.Items[0].OwnerUsername != "u1" || filtered.Items[0].ID != img1.ID {
		t.Fatalf("filtered plaza = %+v", filtered)
	}

	profile, err := service.NewAlbumService(repo).PublicProfile(ctx, "u1")
	if err != nil {
		t.Fatalf("PublicProfile: %v", err)
	}
	if profile.PublicImageCount != 1 {
		t.Fatalf("public image count = %d, want 1", profile.PublicImageCount)
	}
}

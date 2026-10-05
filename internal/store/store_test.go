package store_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/AXmishell/axmipic/internal/store"
)

func newRepo(t *testing.T) *store.Repository {
	t.Helper()
	repo, err := store.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

// seedImage 插入一张图片记录。
func seedImage(t *testing.T, repo *store.Repository, id, owner, albumID, permission string) {
	t.Helper()
	img := &store.Image{
		ID:         id,
		Key:        "key-" + id,
		UserID:     &owner,
		URL:        "http://localhost/i/" + id,
		Size:       1,
		MimeType:   "image/png",
		Permission: permission,
	}
	if albumID != "" {
		img.AlbumID = &albumID
	}
	if err := repo.Create(context.Background(), img); err != nil {
		t.Fatalf("Create image %q: %v", id, err)
	}
}

func TestListAlbumsCountsInSingleQuery(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	owner := "u1"

	for _, album := range []*store.Album{
		{ID: "a1", UserID: &owner, Name: "A"},
		{ID: "a2", UserID: &owner, Name: "B"},
	} {
		if err := repo.CreateAlbum(ctx, album); err != nil {
			t.Fatalf("CreateAlbum: %v", err)
		}
	}
	seedImage(t, repo, "i1", owner, "a1", store.PermissionPrivate)
	seedImage(t, repo, "i2", owner, "a1", store.PermissionPublic)
	seedImage(t, repo, "i3", owner, "", store.PermissionPrivate)

	albums, err := repo.ListAlbums(ctx, owner)
	if err != nil {
		t.Fatalf("ListAlbums: %v", err)
	}
	if len(albums) != 2 {
		t.Fatalf("album count = %d, want 2", len(albums))
	}
	counts := map[string]int64{}
	for _, a := range albums {
		counts[a.Album.ID] = a.ImageCount
	}
	if counts["a1"] != 2 || counts["a2"] != 0 {
		t.Fatalf("unexpected counts: %+v", counts)
	}
}

func TestDeleteAlbumDetachesImages(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	owner := "u1"
	if err := repo.CreateAlbum(ctx, &store.Album{ID: "a1", UserID: &owner, Name: "A"}); err != nil {
		t.Fatalf("CreateAlbum: %v", err)
	}
	seedImage(t, repo, "i1", owner, "a1", store.PermissionPrivate)

	if err := repo.DeleteAlbum(ctx, "a1"); err != nil {
		t.Fatalf("DeleteAlbum: %v", err)
	}
	img, err := repo.GetByID(ctx, "i1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if img.AlbumID != nil {
		t.Fatalf("image still attached to album: %q", *img.AlbumID)
	}
}

func TestSetImagePermissionAndAlbum(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	owner := "u1"
	seedImage(t, repo, "i1", owner, "", store.PermissionPrivate)
	seedImage(t, repo, "i2", owner, "", store.PermissionPrivate)

	if err := repo.SetImagePermission(ctx, []string{"i1", "i2"}, store.PermissionPublic); err != nil {
		t.Fatalf("SetImagePermission: %v", err)
	}
	if err := repo.SetImageAlbum(ctx, []string{"i1"}, &owner); err != nil {
		t.Fatalf("SetImageAlbum: %v", err)
	}

	img1, _ := repo.GetByID(ctx, "i1")
	img2, _ := repo.GetByID(ctx, "i2")
	if img1.Permission != store.PermissionPublic || img2.Permission != store.PermissionPublic {
		t.Fatalf("permission not updated: %q %q", img1.Permission, img2.Permission)
	}
	if img1.AlbumID == nil || *img1.AlbumID != owner {
		t.Fatalf("album not set on i1: %+v", img1.AlbumID)
	}
	if img2.AlbumID != nil {
		t.Fatalf("album unexpectedly set on i2: %+v", img2.AlbumID)
	}
}

func TestListImagesByIDs(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	owner := "u1"
	seedImage(t, repo, "i1", owner, "", store.PermissionPrivate)
	seedImage(t, repo, "i2", owner, "", store.PermissionPrivate)

	images, err := repo.ListImagesByIDs(ctx, []string{"i1", "missing", "i2"})
	if err != nil {
		t.Fatalf("ListImagesByIDs: %v", err)
	}
	if len(images) != 2 {
		t.Fatalf("got %d images, want 2", len(images))
	}
}

func TestQuotaReserveAndRelease(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	if err := repo.CreateCustomer(ctx, &store.Customer{
		ID: "c1", Username: "c1", PasswordHash: "x", QuotaBytes: 10,
	}); err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}

	ok, err := repo.ReserveQuota(ctx, "c1", 6)
	if err != nil || !ok {
		t.Fatalf("first reserve ok=%v err=%v", ok, err)
	}
	ok, err = repo.ReserveQuota(ctx, "c1", 6)
	if err != nil || ok {
		t.Fatalf("over-quota reserve ok=%v err=%v, want false", ok, err)
	}
	if err := repo.ReleaseQuota(ctx, "c1", 6); err != nil {
		t.Fatalf("ReleaseQuota: %v", err)
	}
	ok, err = repo.ReserveQuota(ctx, "c1", 6)
	if err != nil || !ok {
		t.Fatalf("reserve after release ok=%v err=%v", ok, err)
	}
}

func TestEmailCodeLifecycle(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()

	expires := time.Now().Add(time.Minute)
	if err := repo.UpsertEmailCode(ctx, "reset:a@b.com", "a@b.com", "123456", expires); err != nil {
		t.Fatalf("UpsertEmailCode: %v", err)
	}
	got, err := repo.EmailCode(ctx, "reset:a@b.com")
	if err != nil || got.Code != "123456" || got.Email != "a@b.com" {
		t.Fatalf("EmailCode = %+v err=%v", got, err)
	}

	// 错误的验证码不应删除记录。
	if ok, err := repo.DeleteEmailCodeIfMatches(ctx, "reset:a@b.com", "000000"); err != nil || ok {
		t.Fatalf("delete mismatch ok=%v err=%v, want false", ok, err)
	}
	if _, err := repo.EmailCode(ctx, "reset:a@b.com"); err != nil {
		t.Fatalf("record should survive mismatch: %v", err)
	}
	// 正确验证码原子消费。
	if ok, err := repo.DeleteEmailCodeIfMatches(ctx, "reset:a@b.com", "123456"); err != nil || !ok {
		t.Fatalf("delete match ok=%v err=%v, want true", ok, err)
	}
	if _, err := repo.EmailCode(ctx, "reset:a@b.com"); err == nil {
		t.Fatal("record should be consumed")
	}

	// 每日计数原子累加。
	day := "2026-01-01"
	if n, err := repo.IncrementEmailDaily(ctx, "reset:a@b.com", day); err != nil || n != 1 {
		t.Fatalf("first increment n=%d err=%v", n, err)
	}
	if n, err := repo.IncrementEmailDaily(ctx, "reset:a@b.com", day); err != nil || n != 2 {
		t.Fatalf("second increment n=%d err=%v", n, err)
	}
	if n, err := repo.EmailDailyCount(ctx, "reset:a@b.com", "2026-01-02"); err != nil || n != 0 {
		t.Fatalf("other day count n=%d err=%v", n, err)
	}
}

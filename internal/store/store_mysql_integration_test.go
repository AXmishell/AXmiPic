package store_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/AXmishell/axmipic/internal/store"
)

// TestMySQLIntegration 在设置了 AXMIPIC_TEST_MYSQL_DSN 时针对真实 MySQL 运行，
// 覆盖方言敏感路径：key 保留字、LIKE、ON CONFLICT、keyset 游标等。
func TestMySQLIntegration(t *testing.T) {
	dsn := os.Getenv("AXMIPIC_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("AXMIPIC_TEST_MYSQL_DSN not set")
	}
	repo, err := store.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("store.Open(mysql): %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	ctx := context.Background()

	owner := "u-mysql"
	_, _ = repo.DeleteImagesByUser(ctx, owner)

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		img := &store.Image{
			ID:         "img-mysql-" + string(rune('a'+i)),
			Key:        "2026/01/01/mysql-key-" + string(rune('a'+i)) + ".png",
			UserID:     &owner,
			URL:        "http://example/i/x.png",
			Size:       int64(10 + i),
			MimeType:   "image/png",
			Permission: store.PermissionPublic,
			CreatedAt:  base.Add(time.Duration(i) * time.Second),
		}
		if err := repo.Create(ctx, img); err != nil {
			t.Fatalf("Create image %d: %v", i, err)
		}
	}

	// `key = ?`（保留字）
	got, err := repo.GetByKey(ctx, "2026/01/01/mysql-key-b.png")
	if err != nil || got.ID != "img-mysql-b" {
		t.Fatalf("GetByKey: %v / %v", got, err)
	}

	// `images.key LIKE ?`（保留字 + LIKE）
	res, total, err := repo.ListImages(ctx, store.ImageListOptions{Keyword: "mysql-key", Limit: 10, Order: "newest"})
	if err != nil {
		t.Fatalf("ListImages keyword: %v", err)
	}
	if total != 3 || len(res) != 3 {
		t.Fatalf("keyword search total=%d len=%d, want 3/3", total, len(res))
	}

	// keyset 游标（行值比较）
	page1, _, err := repo.ListImages(ctx, store.ImageListOptions{Order: "newest", Limit: 2})
	if err != nil {
		t.Fatalf("ListImages page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1 len=%d, want 2", len(page1))
	}
	last := page1[len(page1)-1]
	page2, _, err := repo.ListImages(ctx, store.ImageListOptions{
		Order:  "newest",
		Limit:  2,
		Cursor: &store.ImageCursor{Order: "newest", CreatedAt: last.CreatedAt, ID: last.ID},
	})
	if err != nil {
		t.Fatalf("ListImages page2: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("page2 len=%d, want 1", len(page2))
	}

	// `key = ?` + ON CONFLICT（设置）
	if err := repo.SetSetting(ctx, "mysql.test", "v1"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	if v, err := repo.GetSetting(ctx, "mysql.test"); err != nil || v != "v1" {
		t.Fatalf("GetSetting: %q / %v", v, err)
	}

	// 邮箱验证码：key 列 + ON CONFLICT
	if err := repo.UpsertEmailCode(ctx, "mysql.test", "a@b.c", "123456", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("UpsertEmailCode: %v", err)
	}
	if code, err := repo.EmailCode(ctx, "mysql.test"); err != nil || code.Code != "123456" {
		t.Fatalf("EmailCode: %v / %v", code, err)
	}

	// 待确认上传：key = ? / DELETE
	pend := &store.PendingUpload{Key: "2026/01/01/pending.png", MimeType: "image/png", MaxSize: 100, ExpiresAt: time.Now().Add(time.Hour)}
	if err := repo.CreatePendingUpload(ctx, pend); err != nil {
		t.Fatalf("CreatePendingUpload: %v", err)
	}
	if _, err := repo.GetPendingUpload(ctx, pend.Key); err != nil {
		t.Fatalf("GetPendingUpload: %v", err)
	}
	if err := repo.DeletePendingUpload(ctx, pend.Key); err != nil {
		t.Fatalf("DeletePendingUpload: %v", err)
	}

	// `key IN ?` + Pluck
	existing, err := repo.ExistingImageKeys(ctx, []string{"2026/01/01/mysql-key-a.png", "nope"})
	if err != nil {
		t.Fatalf("ExistingImageKeys: %v", err)
	}
	if _, ok := existing["2026/01/01/mysql-key-a.png"]; !ok {
		t.Fatalf("ExistingImageKeys missing key: %+v", existing)
	}

	_, _ = repo.DeleteImagesByUser(ctx, owner)
}

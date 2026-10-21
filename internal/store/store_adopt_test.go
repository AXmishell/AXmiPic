package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/AXmishell/axmipic/internal/store"
)

// TestRepositoryAdopt 验证初始化期间切换数据库后，原仓库立即读写新库，
// 且新仓库句柄失效后不会误关已接管的连接。
func TestRepositoryAdopt(t *testing.T) {
	ctx := context.Background()
	live, err := store.Open("sqlite", filepath.Join(t.TempDir(), "live.db"))
	if err != nil {
		t.Fatalf("store.Open(live): %v", err)
	}
	t.Cleanup(func() { _ = live.Close() })

	target, err := store.Open("sqlite", filepath.Join(t.TempDir(), "target.db"))
	if err != nil {
		t.Fatalf("store.Open(target): %v", err)
	}
	t.Cleanup(func() { _ = target.Close() })

	if err := live.CreateAdmin(ctx, &store.Admin{ID: "old", Username: "old", PasswordHash: "x"}); err != nil {
		t.Fatalf("create admin in live db: %v", err)
	}
	if err := target.CreateAdmin(ctx, &store.Admin{ID: "new", Username: "new", PasswordHash: "x"}); err != nil {
		t.Fatalf("create admin in target db: %v", err)
	}

	if err := live.Adopt(target); err != nil {
		t.Fatalf("Adopt: %v", err)
	}
	if _, err := live.GetAccountByUsername(ctx, store.RoleAdmin, "new"); err != nil {
		t.Fatalf("query adopted database: %v", err)
	}
	if _, err := live.GetAccountByUsername(ctx, store.RoleAdmin, "old"); err == nil {
		t.Fatal("live repository still reads the old database after Adopt")
	}

	// target 已交出连接，关闭它不应影响 live。
	if err := target.Close(); err != nil {
		t.Fatalf("close adopted target: %v", err)
	}
	if err := live.Ping(ctx); err != nil {
		t.Fatalf("live ping after target close: %v", err)
	}
}

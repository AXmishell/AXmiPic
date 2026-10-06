package service_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AXmishell/axmipic/internal/config"
	"github.com/AXmishell/axmipic/internal/service"
	"github.com/AXmishell/axmipic/internal/storage"
	"github.com/AXmishell/axmipic/internal/store"
)

// TestOrphanReconcilerRemovesStaleOrphans 验证对账任务只删除「早于保留期且
// 数据库无记录」的日期分区对象，保留有记录的对象与过新的对象。
func TestOrphanReconcilerRemovesStaleOrphans(t *testing.T) {
	root := t.TempDir()
	repo := newRepo(t)
	manager := storage.NewManager()
	if _, err := manager.Register("local", "Local", "http://localhost:8080", config.StorageConfig{
		Driver: "local",
		Local:  config.LocalStorageConfig{Root: root},
	}); err != nil {
		t.Fatalf("manager.Register: %v", err)
	}

	writeObject := func(key string, mod time.Time) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(key))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if err := os.Chtimes(full, mod, mod); err != nil {
			t.Fatalf("Chtimes: %v", err)
		}
	}

	old := time.Now().Add(-30 * 24 * time.Hour)
	writeObject("2020/01/01/orphan.jpg", old)
	writeObject("2020/01/01/kept.jpg", old)
	writeObject("2999/01/01/future.jpg", time.Now())

	owner := "u1"
	if err := repo.Create(context.Background(), &store.Image{
		ID: "i1", Key: "2020/01/01/kept.jpg", UserID: &owner,
		URL: "http://localhost/i/2020/01/01/kept.jpg", Size: 1, MimeType: "image/png",
	}); err != nil {
		t.Fatalf("Create image: %v", err)
	}

	reconciler := service.NewOrphanReconciler(repo, manager, time.Hour)
	removed, err := reconciler.Reconcile(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if _, err := os.Stat(filepath.Join(root, "2020/01/01/orphan.jpg")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("orphan object still present: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "2020/01/01/kept.jpg")); err != nil {
		t.Fatalf("recorded object was removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "2999/01/01/future.jpg")); err != nil {
		t.Fatalf("future object was removed: %v", err)
	}
}

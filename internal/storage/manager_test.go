package storage_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/AXmishell/axmipic/internal/config"
	"github.com/AXmishell/axmipic/internal/storage"
)

func TestManagerRegisterAndResolve(t *testing.T) {
	m := storage.NewManager()
	first, err := m.Register("id-a", "本地A", "", config.StorageConfig{
		Driver: "local",
		Local:  config.LocalStorageConfig{Root: t.TempDir()},
	})
	if err != nil {
		t.Fatalf("Register A: %v", err)
	}
	second, err := m.Register("id-b", "本地B", "", config.StorageConfig{
		Driver: "local",
		Local:  config.LocalStorageConfig{Root: t.TempDir()},
	})
	if err != nil {
		t.Fatalf("Register B: %v", err)
	}

	// 未设置默认时，Resolve 回退到兜底（此处为 nil）。
	if err := m.SetCurrent("id-a"); err != nil {
		t.Fatalf("SetCurrent: %v", err)
	}
	if m.Resolve("id-a") != first {
		t.Fatal("Resolve(id-a) did not return the first backend")
	}
	if m.Resolve("id-b") != second {
		t.Fatal("Resolve(id-b) did not return the second backend")
	}
	// 未知 id 回退到当前默认。
	if m.Resolve("missing") != first {
		t.Fatal("Resolve should fall back to current")
	}

	// 热切换默认后端。
	if err := m.SetCurrent("id-b"); err != nil {
		t.Fatalf("SetCurrent id-b: %v", err)
	}
	if m.Current() != second {
		t.Fatal("Current did not switch to second backend")
	}
}

func TestManagerHotSwitchCurrent(t *testing.T) {
	m := storage.NewManager()
	dirA, dirB := t.TempDir(), t.TempDir()
	_, _ = m.Register("a", "A", "", config.StorageConfig{Driver: "local", Local: config.LocalStorageConfig{Root: dirA}})
	_, _ = m.Register("b", "B", "", config.StorageConfig{Driver: "local", Local: config.LocalStorageConfig{Root: dirB}})
	_ = m.SetCurrent("a")

	ctx := context.Background()
	payload := []byte("hello")
	if err := m.Current().Put(ctx, "obj.txt", bytes.NewReader(payload), int64(len(payload)), "text/plain"); err != nil {
		t.Fatalf("Put on A: %v", err)
	}
	if !fileExists(filepath.Join(dirA, "obj.txt")) {
		t.Fatal("object was not written to backend A")
	}

	// 热切换到 B 后，新写入应落到 B。
	_ = m.SetCurrent("b")
	if err := m.Current().Put(ctx, "obj.txt", bytes.NewReader(payload), int64(len(payload)), "text/plain"); err != nil {
		t.Fatalf("Put on B: %v", err)
	}
	if !fileExists(filepath.Join(dirB, "obj.txt")) {
		t.Fatal("object was not written to backend B after switch")
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestManagerRemoveClearsCurrent(t *testing.T) {
	m := storage.NewManager()
	_, _ = m.Register("a", "A", "", config.StorageConfig{Driver: "local", Local: config.LocalStorageConfig{Root: t.TempDir()}})
	_ = m.SetCurrent("a")
	m.Remove("a")
	if m.CurrentID() != "" {
		t.Fatalf("CurrentID = %q, want empty after removal", m.CurrentID())
	}
	if _, ok := m.DriverOf("a"); ok {
		t.Fatal("removed backend still reported")
	}
}

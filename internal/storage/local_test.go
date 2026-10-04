package storage_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/AXmishell/axmipic/internal/storage"
)

func TestLocalStatSniffsExtensionlessKey(t *testing.T) {
	local, err := storage.NewLocal(t.TempDir(), "http://localhost")
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}
	ctx := context.Background()

	// The PNG signature is enough for http.DetectContentType.
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x00")
	if err := local.Put(ctx, "no-extension", bytes.NewReader(png), int64(len(png)), "image/png"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	info, err := local.Stat(ctx, "no-extension")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.ContentType != "image/png" {
		t.Fatalf("content type = %q, want image/png", info.ContentType)
	}
}

func TestLocalStatPrefersExtension(t *testing.T) {
	local, err := storage.NewLocal(t.TempDir(), "http://localhost")
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}
	ctx := context.Background()

	if err := local.Put(ctx, "photo.png", bytes.NewReader([]byte("anything")), 8, "image/png"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	info, err := local.Stat(ctx, "photo.png")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.ContentType != "image/png" {
		t.Fatalf("content type = %q, want image/png", info.ContentType)
	}
}

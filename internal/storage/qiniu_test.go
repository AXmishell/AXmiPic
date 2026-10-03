package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/axmipic/axmipic/internal/config"
	"github.com/axmipic/axmipic/internal/storage"
)

func TestQiniuPresignPutIsGeneratedOffline(t *testing.T) {
	driver, err := storage.NewQiniu(config.QiniuConfig{
		AccessKey: "test-access-key",
		SecretKey: "test-secret-key",
		Bucket:    "axmipic",
		Domain:    "https://cdn.example.com",
		UseHTTPS:  true,
	})
	if err != nil {
		t.Fatalf("NewQiniu: %v", err)
	}

	const key = "2026/01/01/abc.png"
	req, err := driver.PresignPut(context.Background(), key, storage.PresignOptions{
		ContentType:         "image/png",
		AllowedContentTypes: []string{"image/png", "image/jpeg"},
		MaxSize:             1 << 20,
		Expires:             time.Hour,
	})
	if err != nil {
		t.Fatalf("PresignPut: %v", err)
	}
	if req.Method != "POST" {
		t.Fatalf("method = %q, want POST", req.Method)
	}
	if req.Fields["key"] != key {
		t.Fatalf("key field = %q, want %q", req.Fields["key"], key)
	}
	if req.Fields["token"] == "" {
		t.Fatal("empty upload token")
	}
	if req.URL == "" {
		t.Fatal("empty upload host")
	}
}

func TestQiniuURL(t *testing.T) {
	driver, err := storage.NewQiniu(config.QiniuConfig{
		AccessKey: "ak",
		SecretKey: "sk",
		Bucket:    "axmipic",
		Domain:    "https://cdn.example.com/",
		UseHTTPS:  true,
	})
	if err != nil {
		t.Fatalf("NewQiniu: %v", err)
	}
	if got, want := driver.URL("a/b.png"), "https://cdn.example.com/a/b.png"; got != want {
		t.Fatalf("URL = %q, want %q", got, want)
	}
}

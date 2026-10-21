package storage_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/AXmishell/axmipic/internal/config"
	"github.com/AXmishell/axmipic/internal/storage"
)

func TestCOSPresignPutIsGeneratedOffline(t *testing.T) {
	driver, err := storage.NewCOS(config.COSConfig{
		Bucket:    "axmipic-1250000000",
		Region:    "ap-guangzhou",
		SecretID:  "test-secret-id",
		SecretKey: "test-secret-key",
		UseHTTPS:  true,
	})
	if err != nil {
		t.Fatalf("NewCOS: %v", err)
	}

	const key = "2026/01/01/abc.png"
	req, err := driver.PresignPut(context.Background(), key, storage.PresignOptions{
		ContentType: "image/png",
		MaxSize:     1 << 20,
		Expires:     15 * time.Minute,
	})
	if err != nil {
		t.Fatalf("PresignPut: %v", err)
	}
	if req.Method != "PUT" {
		t.Fatalf("method = %q, want PUT", req.Method)
	}
	if !strings.Contains(req.URL, "axmipic-1250000000.cos.ap-guangzhou.myqcloud.com") {
		t.Fatalf("unexpected presigned URL: %s", req.URL)
	}
	if !strings.Contains(req.URL, "q-signature=") {
		t.Fatalf("missing signature in URL: %s", req.URL)
	}
	if req.Headers["Content-Type"] != "image/png" {
		t.Fatalf("Content-Type header = %q", req.Headers["Content-Type"])
	}
	if req.ExpiresAt.IsZero() {
		t.Fatal("empty expiry")
	}
}

func TestCOSURL(t *testing.T) {
	driver, err := storage.NewCOS(config.COSConfig{
		Bucket:    "axmipic-1250000000",
		Region:    "ap-chengdu",
		SecretID:  "id",
		SecretKey: "key",
		Domain:    "https://cdn.example.com/",
		UseHTTPS:  true,
	})
	if err != nil {
		t.Fatalf("NewCOS: %v", err)
	}
	if got, want := driver.URL("a/b.png"), "https://cdn.example.com/a/b.png"; got != want {
		t.Fatalf("URL = %q, want %q", got, want)
	}
}

func TestCOSDefaultEndpointURL(t *testing.T) {
	driver, err := storage.NewCOS(config.COSConfig{
		Bucket:    "axmipic-1250000000",
		Region:    "ap-shanghai",
		SecretID:  "id",
		SecretKey: "key",
		UseHTTPS:  true,
	})
	if err != nil {
		t.Fatalf("NewCOS: %v", err)
	}
	if got, want := driver.URL("a.png"), "https://axmipic-1250000000.cos.ap-shanghai.myqcloud.com/a.png"; got != want {
		t.Fatalf("URL = %q, want %q", got, want)
	}
}

func TestCOSRequiresRegion(t *testing.T) {
	if _, err := storage.NewCOS(config.COSConfig{
		Bucket:    "axmipic-1250000000",
		SecretID:  "id",
		SecretKey: "key",
	}); err == nil {
		t.Fatal("expected an error when region is empty")
	}
}

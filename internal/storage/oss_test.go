package storage_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/AXmishell/axmipic/internal/config"
	"github.com/AXmishell/axmipic/internal/storage"
)

func TestOSSPresignPutIsGeneratedOffline(t *testing.T) {
	driver, err := storage.NewOSS(config.OSSConfig{
		Region:          "oss-cn-hangzhou",
		Bucket:          "axmipic",
		AccessKeyID:     "test-access-key-id",
		AccessKeySecret: "test-access-key-secret",
		UseHTTPS:        true,
	})
	if err != nil {
		t.Fatalf("NewOSS: %v", err)
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
	if !strings.Contains(req.URL, "axmipic.oss-cn-hangzhou.aliyuncs.com") {
		t.Fatalf("unexpected presigned URL: %s", req.URL)
	}
	if !strings.Contains(req.URL, "OSSAccessKeyId=") || !strings.Contains(req.URL, "Signature=") {
		t.Fatalf("missing signature parameters: %s", req.URL)
	}
	if req.Headers["Content-Type"] != "image/png" {
		t.Fatalf("Content-Type header = %q", req.Headers["Content-Type"])
	}
	if req.ExpiresAt.IsZero() {
		t.Fatal("empty expiry")
	}
}

func TestOSSURL(t *testing.T) {
	driver, err := storage.NewOSS(config.OSSConfig{
		Region:          "oss-cn-shanghai",
		Bucket:          "axmipic",
		AccessKeyID:     "ak",
		AccessKeySecret: "sk",
		Domain:          "https://cdn.example.com/",
		UseHTTPS:        true,
	})
	if err != nil {
		t.Fatalf("NewOSS: %v", err)
	}
	if got, want := driver.URL("a/b.png"), "https://cdn.example.com/a/b.png"; got != want {
		t.Fatalf("URL = %q, want %q", got, want)
	}
}

func TestOSSDefaultEndpointURL(t *testing.T) {
	driver, err := storage.NewOSS(config.OSSConfig{
		Region:          "oss-cn-beijing",
		Bucket:          "axmipic",
		AccessKeyID:     "ak",
		AccessKeySecret: "sk",
		UseHTTPS:        true,
	})
	if err != nil {
		t.Fatalf("NewOSS: %v", err)
	}
	if got, want := driver.URL("a.png"), "https://axmipic.oss-cn-beijing.aliyuncs.com/a.png"; got != want {
		t.Fatalf("URL = %q, want %q", got, want)
	}
}

func TestOSSRequiresRegionOrEndpoint(t *testing.T) {
	if _, err := storage.NewOSS(config.OSSConfig{
		Bucket:          "axmipic",
		AccessKeyID:     "ak",
		AccessKeySecret: "sk",
	}); err == nil {
		t.Fatal("expected an error when both region and endpoint are empty")
	}
}

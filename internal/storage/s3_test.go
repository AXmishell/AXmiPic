package storage_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/AXmishell/axmipic/internal/config"
	"github.com/AXmishell/axmipic/internal/storage"
)

func TestS3PresignPutIsGeneratedOffline(t *testing.T) {
	driver, err := storage.NewS3(config.S3Config{
		Endpoint:        "https://s3.us-east-1.amazonaws.com",
		Region:          "us-east-1",
		Bucket:          "axmipic",
		AccessKeyID:     "AKIAEXAMPLE",
		SecretAccessKey: "secret",
		Secure:          true,
	})
	if err != nil {
		t.Fatalf("NewS3: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	const key = "2026/01/01/abc.png"
	req, err := driver.PresignPut(ctx, key, storage.PresignOptions{
		ContentType: "image/png",
		MaxSize:     1 << 20,
		Expires:     15 * time.Minute,
	})
	if err != nil {
		t.Fatalf("PresignPut: %v", err)
	}
	if req.Method != "POST" {
		t.Fatalf("method = %q, want POST", req.Method)
	}
	if req.URL == "" {
		t.Fatal("empty presigned URL")
	}
	if req.Fields["key"] != key {
		t.Fatalf("form key = %q, want %q", req.Fields["key"], key)
	}
	if req.Fields["policy"] == "" {
		t.Fatalf("missing signed policy field: %v", req.Fields)
	}
	hasSignature := false
	for name := range req.Fields {
		if strings.Contains(strings.ToLower(name), "signature") {
			hasSignature = true
			break
		}
	}
	if !hasSignature {
		t.Fatalf("missing signature field: %v", req.Fields)
	}
	if req.Fields["Content-Type"] != "image/png" {
		t.Fatalf("Content-Type field = %q", req.Fields["Content-Type"])
	}
	if req.ExpiresAt.IsZero() {
		t.Fatal("zero expiry")
	}
}

func TestS3URL(t *testing.T) {
	pathStyle, err := storage.NewS3(config.S3Config{
		Endpoint:     "https://minio.local:9000",
		Bucket:       "images",
		Secure:       true,
		UsePathStyle: true,
	})
	if err != nil {
		t.Fatalf("NewS3: %v", err)
	}
	if got, want := pathStyle.URL("a/b.png"), "https://minio.local:9000/images/a/b.png"; got != want {
		t.Fatalf("URL = %q, want %q", got, want)
	}

	cdn, err := storage.NewS3(config.S3Config{
		Endpoint:      "https://s3.amazonaws.com",
		Bucket:        "images",
		Secure:        true,
		PublicBaseURL: "https://cdn.example.com/",
	})
	if err != nil {
		t.Fatalf("NewS3: %v", err)
	}
	if got, want := cdn.URL("a/b.png"), "https://cdn.example.com/a/b.png"; got != want {
		t.Fatalf("URL = %q, want %q", got, want)
	}
}

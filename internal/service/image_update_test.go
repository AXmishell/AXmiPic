package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/service"
)

func TestUpdateImageMetadataOnlyAdmin(t *testing.T) {
	repo := newRepo(t)
	svc := service.NewUploadService(repo, managerWithFallback(t, newFakeStorage()), pngPolicy())
	ctx := context.Background()
	newCustomer(t, repo, "u1")
	owner := &auth.Principal{UserID: "u1", Username: "u1", Role: auth.RoleUser}
	admin := &auth.Principal{Role: auth.RoleAdmin}

	image, err := svc.Upload(ctx, owner, service.UploadInput{Data: testPNG(t), MimeType: "image/png"})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	name := "author"
	if _, err := svc.UpdateImage(ctx, owner, image.ID, service.ImageUpdate{AuthorName: &name}); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("owner metadata update: expected ErrForbidden, got %v", err)
	}

	handle := "@mooNli9ht_4tune"
	source := "https://x.com/mooNli9ht_4tune/status/2112521500301607286"
	updated, err := svc.UpdateImage(ctx, admin, image.ID, service.ImageUpdate{
		AuthorName:   &name,
		AuthorHandle: &handle,
		SourceURL:    &source,
	})
	if err != nil {
		t.Fatalf("admin metadata update: %v", err)
	}
	if updated.AuthorName != "author" || updated.AuthorHandle != "mooNli9ht_4tune" {
		t.Fatalf("author = %q/@%q", updated.AuthorName, updated.AuthorHandle)
	}
	if updated.AuthorURL != "https://x.com/mooNli9ht_4tune" {
		t.Fatalf("author url = %q", updated.AuthorURL)
	}
	if updated.SourceURL != source || updated.SourcePostID != "2112521500301607286" {
		t.Fatalf("source = %q / %q", updated.SourceURL, updated.SourcePostID)
	}
}

func TestUpdateImageNameByOwner(t *testing.T) {
	repo := newRepo(t)
	svc := service.NewUploadService(repo, managerWithFallback(t, newFakeStorage()), pngPolicy())
	ctx := context.Background()
	newCustomer(t, repo, "u1")
	owner := &auth.Principal{UserID: "u1", Username: "u1", Role: auth.RoleUser}

	image, err := svc.Upload(ctx, owner, service.UploadInput{Data: testPNG(t), MimeType: "image/png", OriginalName: "old.png"})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	name := "new.png"
	updated, err := svc.UpdateImage(ctx, owner, image.ID, service.ImageUpdate{OriginalName: &name})
	if err != nil {
		t.Fatalf("UpdateImage: %v", err)
	}
	if updated.OriginalName != "new.png" {
		t.Fatalf("original name = %q", updated.OriginalName)
	}
	if updated.Key != image.Key || updated.Hash != image.Hash {
		t.Fatalf("storage identity changed: %+v vs %+v", updated, image)
	}
}

func TestClearImageSourceKeepsAuthor(t *testing.T) {
	repo := newRepo(t)
	svc := service.NewUploadService(repo, managerWithFallback(t, newFakeStorage()), pngPolicy())
	ctx := context.Background()
	admin := &auth.Principal{Role: auth.RoleAdmin}

	image, err := svc.Upload(ctx, admin, service.UploadInput{Data: testPNG(t), MimeType: "image/png", Meta: service.ImageMeta{
		AuthorName:   "author",
		AuthorHandle: "mooNli9ht_4tune",
		SourceURL:    "https://x.com/mooNli9ht_4tune/status/2112521500301607286",
	}})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	empty := ""
	updated, err := svc.UpdateImage(ctx, admin, image.ID, service.ImageUpdate{SourceURL: &empty})
	if err != nil {
		t.Fatalf("UpdateImage: %v", err)
	}
	if updated.SourceURL != "" || updated.SourcePostID != "" {
		t.Fatalf("source not cleared: %+v", updated)
	}
	if updated.AuthorName != "author" || updated.AuthorHandle != "mooNli9ht_4tune" {
		t.Fatalf("author unexpectedly cleared: %+v", updated)
	}
}

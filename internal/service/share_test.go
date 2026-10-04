package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/service"
	"github.com/AXmishell/axmipic/internal/store"
)

func newShareSetup(t *testing.T) (*service.ShareService, *service.UploadService, *service.AlbumService, *store.Repository) {
	t.Helper()
	repo := newRepo(t)
	newCustomer(t, repo, "u1")
	newCustomer(t, repo, "u2")
	upload := service.NewUploadService(repo, managerWithFallback(t, newFakeStorage()), pngPolicy())
	shares := service.NewShareService(repo, "http://example.test")
	albums := service.NewAlbumService(repo)
	return shares, upload, albums, repo
}

func TestImageShareWithPassword(t *testing.T) {
	shares, upload, _, _ := newShareSetup(t)
	ctx := context.Background()
	u1 := &auth.Principal{UserID: "u1", Username: "u1", Role: auth.RoleUser}
	u2 := &auth.Principal{UserID: "u2", Username: "u2", Role: auth.RoleUser}

	img, err := upload.Upload(ctx, u1, service.UploadInput{Data: testPNGSize(t, 8), MimeType: "image/png"})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	// 跨用户创建分享被拒绝。
	if _, err := shares.Create(ctx, u2, service.ShareInput{TargetType: store.ShareTargetImage, TargetID: img.ID}); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("cross-user create err = %v, want ErrForbidden", err)
	}

	share, err := shares.Create(ctx, u1, service.ShareInput{
		TargetType: store.ShareTargetImage,
		TargetID:   img.ID,
		Password:   "s3cret",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !share.HasPassword || share.URL == "" {
		t.Fatalf("share = %+v", share)
	}

	info, err := shares.Info(ctx, share.Token)
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if !info.HasPassword {
		t.Fatalf("info has_password = false, want true")
	}

	if _, err := shares.Access(ctx, share.Token, ""); !errors.Is(err, service.ErrSharePasswordRequired) {
		t.Fatalf("empty password err = %v, want ErrSharePasswordRequired", err)
	}
	if _, err := shares.Access(ctx, share.Token, "wrong"); !errors.Is(err, service.ErrShareInvalidPassword) {
		t.Fatalf("wrong password err = %v, want ErrShareInvalidPassword", err)
	}

	payload, err := shares.Access(ctx, share.Token, "s3cret")
	if err != nil {
		t.Fatalf("Access: %v", err)
	}
	if payload.Image == nil || payload.Image.ID != img.ID {
		t.Fatalf("payload image = %+v", payload.Image)
	}

	// 撤销后不可再访问。
	if err := shares.Revoke(ctx, u1, share.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, err := shares.Info(ctx, share.Token); !errors.Is(err, service.ErrShareNotFound) {
		t.Fatalf("Info after revoke err = %v, want ErrShareNotFound", err)
	}
}

func TestShareMaxViewsAndAlbum(t *testing.T) {
	shares, upload, albums, _ := newShareSetup(t)
	ctx := context.Background()
	u1 := &auth.Principal{UserID: "u1", Username: "u1", Role: auth.RoleUser}

	img, err := upload.Upload(ctx, u1, service.UploadInput{Data: testPNGSize(t, 8), MimeType: "image/png"})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	album, err := albums.Create(ctx, u1, service.AlbumInput{Name: "相册"})
	if err != nil {
		t.Fatalf("Create album: %v", err)
	}
	if err := upload.SetAlbum(ctx, u1, []string{img.ID}, &album.ID); err != nil {
		t.Fatalf("SetAlbum: %v", err)
	}

	share, err := shares.Create(ctx, u1, service.ShareInput{
		TargetType: store.ShareTargetAlbum,
		TargetID:   album.ID,
		MaxViews:   1,
	})
	if err != nil {
		t.Fatalf("Create album share: %v", err)
	}
	payload, err := shares.Access(ctx, share.Token, "")
	if err != nil {
		t.Fatalf("Access album share: %v", err)
	}
	if payload.Album == nil || payload.Album.ID != album.ID {
		t.Fatalf("payload album = %+v", payload.Album)
	}
	if len(payload.Images) != 1 || payload.Images[0].ID != img.ID {
		t.Fatalf("payload images = %+v", payload.Images)
	}
	// 达到访问上限后不可再访问。
	if _, err := shares.Access(ctx, share.Token, ""); !errors.Is(err, service.ErrShareUnavailable) {
		t.Fatalf("second access err = %v, want ErrShareUnavailable", err)
	}
}

func TestShareInvalidTarget(t *testing.T) {
	shares, _, _, _ := newShareSetup(t)
	ctx := context.Background()
	u1 := &auth.Principal{UserID: "u1", Username: "u1", Role: auth.RoleUser}
	if _, err := shares.Create(ctx, u1, service.ShareInput{TargetType: "nope", TargetID: "x"}); !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("invalid target type err = %v, want ErrInvalidInput", err)
	}
	if _, err := shares.Create(ctx, u1, service.ShareInput{TargetType: store.ShareTargetImage, TargetID: "missing"}); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("missing image err = %v, want ErrNotFound", err)
	}
}

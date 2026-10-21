package service_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/config"
	"github.com/AXmishell/axmipic/internal/secret"
	"github.com/AXmishell/axmipic/internal/service"
	"github.com/AXmishell/axmipic/internal/storage"
	"github.com/AXmishell/axmipic/internal/store"
)

func newStoragePoolFixture(t *testing.T) (*service.StorageService, *store.Repository, *storage.Manager) {
	t.Helper()
	repo, err := store.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	cipher, err := secret.New([]byte("test-master-key-32bytes-minimum"))
	if err != nil {
		t.Fatalf("secret.New: %v", err)
	}
	mgr := storage.NewManager()
	root := t.TempDir()
	svc := service.NewStorageService(repo, mgr, cipher, "http://localhost:8080", config.StorageConfig{
		Driver: "local",
		Local:  config.LocalStorageConfig{Root: root},
	})
	return svc, repo, mgr
}

func createLocalBackend(t *testing.T, svc *service.StorageService, name string) *service.StorageBackendDTO {
	t.Helper()
	dto, err := svc.Create(context.Background(), service.StorageInput{
		Name:     name,
		Driver:   "local",
		Settings: map[string]any{"root": filepath.Join(t.TempDir(), name)},
	}, false)
	if err != nil {
		t.Fatalf("Create backend %s: %v", name, err)
	}
	return dto
}

func TestStoragePoolLifecycleAndPlacement(t *testing.T) {
	svc, _, mgr := newStoragePoolFixture(t)
	ctx := context.Background()
	a := createLocalBackend(t, svc, "a")
	b := createLocalBackend(t, svc, "b")

	pool, err := svc.CreatePool(ctx, service.StoragePoolInput{
		Name:   "主池",
		Policy: storage.PoolPolicyWeightedRR,
		Members: []service.StoragePoolMemberInput{
			{BackendID: a.ID, Weight: 1, Enabled: true},
			{BackendID: b.ID, Weight: 1, Enabled: true},
		},
	})
	if err != nil {
		t.Fatalf("CreatePool: %v", err)
	}
	if len(pool.Members) != 2 {
		t.Fatalf("members = %d", len(pool.Members))
	}
	if err := svc.SetCurrentPool(ctx, pool.ID); err != nil {
		t.Fatalf("SetCurrentPool: %v", err)
	}
	if mgr.CurrentPoolID() != pool.ID {
		t.Fatalf("current pool = %q", mgr.CurrentPoolID())
	}
	counts := map[string]int{}
	for i := 0; i < 4; i++ {
		_, backendID, poolID, err := mgr.Pick(ctx)
		if err != nil {
			t.Fatalf("Pick: %v", err)
		}
		if poolID != pool.ID {
			t.Fatalf("pool id = %q", poolID)
		}
		counts[backendID]++
	}
	if counts[a.ID] != 2 || counts[b.ID] != 2 {
		t.Fatalf("distribution = %v", counts)
	}
	if err := svc.Delete(ctx, a.ID); !errors.Is(err, service.ErrStorageInUse) {
		t.Fatalf("delete backend in pool err = %v", err)
	}
	if err := svc.DeletePool(ctx, pool.ID); !errors.Is(err, service.ErrStorageInUse) {
		t.Fatalf("delete active pool err = %v", err)
	}
	if err := svc.SetCurrent(ctx, a.ID); err != nil {
		t.Fatalf("SetCurrent: %v", err)
	}
	if err := svc.DeletePool(ctx, pool.ID); err != nil {
		t.Fatalf("DeletePool: %v", err)
	}
}

func TestStoragePoolRejectsUnassignedImages(t *testing.T) {
	svc, repo, _ := newStoragePoolFixture(t)
	ctx := context.Background()
	a := createLocalBackend(t, svc, "a")

	img := &store.Image{
		ID: uuid.NewString(), Key: "2006/01/02/legacy", URL: "http://localhost/legacy",
		Size: 1, MimeType: "image/png", Hash: "legacy-hash",
	}
	if err := repo.Create(ctx, img); err != nil {
		t.Fatalf("Create image: %v", err)
	}
	pool, err := svc.CreatePool(ctx, service.StoragePoolInput{
		Name: "pool",
		Members: []service.StoragePoolMemberInput{
			{BackendID: a.ID, Weight: 1, Enabled: true},
		},
	})
	if err != nil {
		t.Fatalf("CreatePool: %v", err)
	}
	if err := svc.SetCurrentPool(ctx, pool.ID); !errors.Is(err, service.ErrStorageUnassigned) {
		t.Fatalf("SetCurrentPool err = %v, want ErrStorageUnassigned", err)
	}
	moved, err := repo.BackfillImageStorage(ctx, a.ID)
	if err != nil || moved != 1 {
		t.Fatalf("BackfillImageStorage = %d, %v", moved, err)
	}
	if err := svc.SetCurrentPool(ctx, pool.ID); err != nil {
		t.Fatalf("SetCurrentPool after backfill: %v", err)
	}
}

func TestUploadPlacesObjectsAcrossPoolMembers(t *testing.T) {
	repo := newRepo(t)
	mgr := storage.NewManager()
	for _, id := range []string{"a", "b"} {
		if _, err := mgr.Register(id, id, "http://localhost", config.StorageConfig{
			Driver: "local",
			Local:  config.LocalStorageConfig{Root: filepath.Join(t.TempDir(), id)},
		}); err != nil {
			t.Fatalf("register %s: %v", id, err)
		}
	}
	if err := mgr.RegisterPool("pool", "pool", storage.PoolPolicyWeightedRR, []storage.PoolMemberConfig{
		{BackendID: "a", Weight: 1, Enabled: true},
		{BackendID: "b", Weight: 1, Enabled: true},
	}); err != nil {
		t.Fatalf("RegisterPool: %v", err)
	}
	if err := mgr.SetCurrentPool("pool"); err != nil {
		t.Fatalf("SetCurrentPool: %v", err)
	}
	svc := service.NewUploadService(repo, mgr, pngPolicy())
	newCustomer(t, repo, "u1")
	newCustomer(t, repo, "u2")
	u1 := &auth.Principal{UserID: "u1", Role: auth.RoleUser}
	u2 := &auth.Principal{UserID: "u2", Role: auth.RoleUser}
	data := testPNG(t)
	first, err := svc.Upload(context.Background(), u1, service.UploadInput{Data: data, MimeType: "image/png"})
	if err != nil {
		t.Fatalf("u1 upload: %v", err)
	}
	second, err := svc.Upload(context.Background(), u2, service.UploadInput{Data: data, MimeType: "image/png"})
	if err != nil {
		t.Fatalf("u2 upload: %v", err)
	}
	got := map[string]bool{first.StorageID: true, second.StorageID: true}
	if !got["a"] || !got["b"] {
		t.Fatalf("storage ids = %q, %q; want a and b", first.StorageID, second.StorageID)
	}
}

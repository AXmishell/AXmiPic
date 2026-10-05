package service_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/AXmishell/axmipic/internal/service"
	"github.com/AXmishell/axmipic/internal/store"
)

func TestInstallStatusAndLock(t *testing.T) {
	dir := t.TempDir()
	lock := filepath.Join(dir, "install.lock")
	svc := service.NewInstallService(lock, filepath.Join(dir, "config.yaml"), false, "")

	status := svc.Status()
	if status.Installed {
		t.Fatalf("expected not installed, got %+v", status)
	}
	if svc.IsInstalled() {
		t.Fatal("IsInstalled should be false before install")
	}

	// 创建锁文件后视为已安装。
	if err := os.WriteFile(lock, []byte("installed_at=x\n"), 0o644); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	if !svc.IsInstalled() {
		t.Fatal("IsInstalled should be true after lock exists")
	}
}

func TestInstallDisabledSkipsCheck(t *testing.T) {
	svc := service.NewInstallService(filepath.Join(t.TempDir(), "missing.lock"), "", true, "")
	if !svc.IsInstalled() {
		t.Fatal("disabled installer should report installed")
	}
}

func TestInstallAuthorizeToken(t *testing.T) {
	svc := service.NewInstallService(filepath.Join(t.TempDir(), "missing.lock"), "", false, "s3cret")
	if !svc.Authorize("s3cret") {
		t.Fatal("correct token should be authorized")
	}
	if svc.Authorize("wrong") || svc.Authorize("") {
		t.Fatal("wrong or empty token must be rejected")
	}
	// 未配置令牌时放行（兼容旧部署与测试）。
	open := service.NewInstallService(filepath.Join(t.TempDir(), "missing.lock"), "", false, "")
	if !open.Authorize("") {
		t.Fatal("empty configured token should authorize")
	}
	// 禁用安装时不校验令牌。
	disabled := service.NewInstallService(filepath.Join(t.TempDir(), "missing.lock"), "", true, "s3cret")
	if !disabled.Authorize("anything") {
		t.Fatal("disabled installer should authorize any token")
	}
}

func TestInstallWritesConfigAndLock(t *testing.T) {
	dir := t.TempDir()
	lock := filepath.Join(dir, "install.lock")
	cfg := filepath.Join(dir, "config.yaml")
	svc := service.NewInstallService(lock, cfg, false, "")

	var seeded bool
	opener := func(driver, dsn string) (*store.Repository, error) {
		return store.Open("sqlite", filepath.Join(dir, "install.db"))
	}
	seed := func(ctx context.Context, repo *store.Repository, in service.InstallInput) error {
		seeded = true
		// 模拟安装过程中的播种：确保仓库可用。
		return repo.CreateRoleGroup(ctx, &store.RoleGroup{ID: "g1", Name: "默认角色组", IsDefault: true})
	}

	err := svc.Install(context.Background(), service.InstallInput{
		SiteName:       "测试图床",
		BaseURL:        "http://example.test",
		DatabaseDriver: "sqlite",
		DatabaseDSN:    filepath.Join(dir, "install.db"),
		AdminUsername:  "admin",
		AdminPassword:  "password123",
		StorageDriver:  "local",
		StorageRoot:    filepath.Join(dir, "uploads"),
	}, opener, seed)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !seeded {
		t.Fatal("seed callback was not invoked")
	}
	if !svc.IsInstalled() {
		t.Fatal("lock file was not written")
	}
	if _, err := os.Stat(cfg); err != nil {
		t.Fatalf("config file not written: %v", err)
	}

	// 重复安装应返回 ErrAlreadyInstalled。
	err = svc.Install(context.Background(), service.InstallInput{
		BaseURL:        "http://example.test",
		DatabaseDriver: "sqlite",
		DatabaseDSN:    filepath.Join(dir, "install.db"),
		AdminUsername:  "admin",
		AdminPassword:  "password123",
	}, opener, seed)
	if !errors.Is(err, service.ErrAlreadyInstalled) {
		t.Fatalf("second install err = %v, want ErrAlreadyInstalled", err)
	}
}

func TestInstallValidatesInput(t *testing.T) {
	dir := t.TempDir()
	svc := service.NewInstallService(filepath.Join(dir, "install.lock"), "", false, "")
	opener := func(driver, dsn string) (*store.Repository, error) {
		return store.Open("sqlite", filepath.Join(dir, "install.db"))
	}
	cases := []service.InstallInput{
		{DatabaseDriver: "sqlite", DatabaseDSN: "x", AdminUsername: "admin", AdminPassword: "password123"},                     // 缺 base_url
		{BaseURL: "ftp://x", DatabaseDriver: "sqlite", DatabaseDSN: "x", AdminUsername: "admin", AdminPassword: "password123"}, // 非法协议
		{BaseURL: "http://x", DatabaseDriver: "sqlite", DatabaseDSN: "", AdminUsername: "admin", AdminPassword: "password123"}, // 缺 dsn
		{BaseURL: "http://x", DatabaseDriver: "sqlite", DatabaseDSN: "x", AdminUsername: "ab", AdminPassword: "password123"},   // 用户名过短
		{BaseURL: "http://x", DatabaseDriver: "sqlite", DatabaseDSN: "x", AdminUsername: "admin", AdminPassword: "short"},      // 密码过短
	}
	for i, in := range cases {
		if err := svc.Install(context.Background(), in, opener, nil); !errors.Is(err, service.ErrInvalidInput) {
			t.Fatalf("case %d err = %v, want ErrInvalidInput", i, err)
		}
	}
}

func TestSeedGuestRoleGroupAndAccount(t *testing.T) {
	svc, _ := newPolicyService(t)
	ctx := context.Background()

	if err := svc.SeedDefaults(ctx); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	group, err := svc.SeedGuestRoleGroup(ctx, 8<<20, 2<<20)
	if err != nil {
		t.Fatalf("SeedGuestRoleGroup: %v", err)
	}
	if group.Name != service.GuestRoleGroupName {
		t.Fatalf("guest group name = %q", group.Name)
	}
	// 幂等：再次调用返回同一组。
	again, err := svc.SeedGuestRoleGroup(ctx, 8<<20, 2<<20)
	if err != nil || again.ID != group.ID {
		t.Fatalf("second seed id = %v err = %v", again, err)
	}

	effective, err := svc.ResolveGuest(ctx)
	if err != nil {
		t.Fatalf("ResolveGuest: %v", err)
	}
	if effective.UploadMaxBytes != 2<<20 || effective.QuotaBytes != 8<<20 {
		t.Fatalf("guest limits = %+v", effective)
	}
	if effective.HasFeature(service.FeaturePlaza) {
		t.Fatalf("guest should not have plaza feature: %+v", effective.Features)
	}
}

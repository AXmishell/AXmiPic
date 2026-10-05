package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/service"
	"github.com/AXmishell/axmipic/internal/store"
)

func newPolicyService(t *testing.T) (*service.PolicyService, *store.Repository) {
	t.Helper()
	repo := newRepo(t)
	svc := service.NewPolicyService(repo, service.PolicyDefaults{
		QuotaBytes:       100 << 20,
		UploadMaxBytes:   5 << 20,
		AllowedMIMETypes: []string{"image/png"},
		Rate: service.RateSettings{
			UploadPerMinute: 10,
			UploadBurst:     2,
			ImagePerMinute:  100,
			ImageBurst:      10,
		},
		Processing: service.ProcessingSettings{
			Enabled:        true,
			MaxWidth:       2048,
			MaxHeight:      2048,
			DefaultQuality: 80,
			AllowedFormats: []string{"jpeg", "png"},
		},
	})
	return svc, repo
}

func TestSeedDefaultsCreatesDefaultRoleGroup(t *testing.T) {
	svc, _ := newPolicyService(t)
	ctx := context.Background()

	if err := svc.SeedDefaults(ctx); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	// 幂等：再次播种不应重复创建。
	if err := svc.SeedDefaults(ctx); err != nil {
		t.Fatalf("SeedDefaults (second): %v", err)
	}

	groups, err := svc.ListRoleGroups(ctx)
	if err != nil {
		t.Fatalf("ListRoleGroups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("role group count = %d, want 1", len(groups))
	}
	group := groups[0]
	if !group.IsDefault {
		t.Fatalf("seeded group is not default: %+v", group)
	}
	if group.PolicyCount != 5 || len(group.Policies) != 5 {
		t.Fatalf("policy count = %d/%d, want 5", group.PolicyCount, len(group.Policies))
	}

	effective, err := svc.ResolveDefault(ctx)
	if err != nil {
		t.Fatalf("ResolveDefault: %v", err)
	}
	if effective.QuotaBytes != 100<<20 || effective.UploadMaxBytes != 5<<20 {
		t.Fatalf("unexpected effective limits: %+v", effective)
	}
	if !effective.HasFeature(service.FeaturePlaza) {
		t.Fatalf("expected plaza feature to be enabled: %+v", effective.Features)
	}
}

func TestRegistrationUsesDefaultRoleGroupPolicy(t *testing.T) {
	svc, repo := newPolicyService(t)
	ctx := context.Background()
	if err := svc.SeedDefaults(ctx); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}

	accounts := service.NewAccountService(repo, auth.NewSessionIssuer([]byte("secret"), time.Hour), true, 999)
	accounts.SetPolicyService(svc)

	user, err := accounts.RegisterCustomer(ctx, "alice", "password123")
	if err != nil {
		t.Fatalf("RegisterCustomer: %v", err)
	}
	if user.RoleGroupID == "" {
		t.Fatalf("registered user has no role group: %+v", user)
	}
	if user.QuotaBytes != 100<<20 {
		t.Fatalf("quota = %d, want %d", user.QuotaBytes, int64(100<<20))
	}
}

func TestResolveEffectivePolicyOverrides(t *testing.T) {
	svc, _ := newPolicyService(t)
	ctx := context.Background()
	if err := svc.SeedDefaults(ctx); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	groups, _ := svc.ListRoleGroups(ctx)
	groupID := groups[0].ID

	// 新建一个更大的配额策略并绑定到默认组，同类型策略应替换旧绑定。
	policy, err := svc.CreatePolicy(ctx, service.PolicyInput{
		Name:     "高配额",
		Type:     store.PolicyTypeQuota,
		Enabled:  true,
		Settings: json.RawMessage(`{"quota_mb": 512}`),
	})
	if err != nil {
		t.Fatalf("CreatePolicy: %v", err)
	}
	updated, err := svc.AttachPolicy(ctx, groupID, policy.ID)
	if err != nil {
		t.Fatalf("AttachPolicy: %v", err)
	}
	if updated.PolicyCount != 5 {
		t.Fatalf("policy count after replace = %d, want 5", updated.PolicyCount)
	}

	effective, err := svc.ResolveForRoleGroupID(ctx, groupID)
	if err != nil {
		t.Fatalf("ResolveForRoleGroupID: %v", err)
	}
	if effective.QuotaBytes != 512<<20 {
		t.Fatalf("quota = %d, want %d", effective.QuotaBytes, int64(512<<20))
	}
}

func TestFeaturePolicyRestrictsFeatures(t *testing.T) {
	svc, _ := newPolicyService(t)
	ctx := context.Background()
	if err := svc.SeedDefaults(ctx); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	groups, _ := svc.ListRoleGroups(ctx)

	policy, err := svc.CreatePolicy(ctx, service.PolicyInput{
		Name:     "受限功能",
		Type:     store.PolicyTypeFeature,
		Enabled:  true,
		Settings: json.RawMessage(`{"features":["plaza"]}`),
	})
	if err != nil {
		t.Fatalf("CreatePolicy: %v", err)
	}
	if _, err := svc.AttachPolicy(ctx, groups[0].ID, policy.ID); err != nil {
		t.Fatalf("AttachPolicy: %v", err)
	}

	effective, err := svc.ResolveDefault(ctx)
	if err != nil {
		t.Fatalf("ResolveDefault: %v", err)
	}
	if len(effective.Features) != 1 || !effective.HasFeature(service.FeaturePlaza) {
		t.Fatalf("unexpected features: %+v", effective.Features)
	}
	if effective.HasFeature(service.FeatureAPITokens) {
		t.Fatalf("api_tokens should be disabled: %+v", effective.Features)
	}
}

func TestPolicyValidationRejectsUnknownFeature(t *testing.T) {
	svc, _ := newPolicyService(t)
	_, err := svc.CreatePolicy(context.Background(), service.PolicyInput{
		Name:     "坏功能",
		Type:     store.PolicyTypeFeature,
		Enabled:  true,
		Settings: json.RawMessage(`{"features":["nope"]}`),
	})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("error = %v, want ErrInvalidInput", err)
	}
}

func TestRateLimitsForResolvesRoleGroupPolicy(t *testing.T) {
	svc, repo := newPolicyService(t)
	ctx := context.Background()
	if err := svc.SeedDefaults(ctx); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}

	// 默认组速率来自 PolicyDefaults。
	base, err := svc.RateLimitsFor(ctx, &auth.Principal{Role: auth.RoleUser})
	if err != nil {
		t.Fatalf("RateLimitsFor default: %v", err)
	}
	if base.UploadPerMinute != 10 || base.UploadBurst != 2 || base.ImagePerMinute != 100 || base.ImageBurst != 10 {
		t.Fatalf("default rate = %+v", base)
	}

	// 自定义角色组覆盖速率策略。
	custom, err := svc.CreatePolicy(ctx, service.PolicyInput{
		Name: "高速率", Type: store.PolicyTypeRate, Enabled: true,
		Settings: json.RawMessage(`{"upload_per_minute":99,"upload_burst":9,"image_per_minute":999,"image_burst":99}`),
	})
	if err != nil {
		t.Fatalf("CreatePolicy: %v", err)
	}
	group, err := svc.CreateRoleGroup(ctx, service.RoleGroupInput{Name: "高速组"})
	if err != nil {
		t.Fatalf("CreateRoleGroup: %v", err)
	}
	if _, err := svc.AttachPolicy(ctx, group.ID, custom.ID); err != nil {
		t.Fatalf("AttachPolicy: %v", err)
	}
	newCustomer(t, repo, "u1")
	groupID := group.ID
	if _, err := repo.UpdateCustomer(ctx, "u1", store.UserUpdate{RoleGroupID: &groupID}); err != nil {
		t.Fatalf("UpdateCustomer: %v", err)
	}

	got, err := svc.RateLimitsFor(ctx, &auth.Principal{UserID: "u1", Role: auth.RoleUser})
	if err != nil {
		t.Fatalf("RateLimitsFor custom: %v", err)
	}
	if got.UploadPerMinute != 99 || got.UploadBurst != 9 || got.ImagePerMinute != 999 || got.ImageBurst != 99 {
		t.Fatalf("custom rate = %+v", got)
	}
}

func TestDeleteRoleGroupInUse(t *testing.T) {
	svc, repo := newPolicyService(t)
	ctx := context.Background()
	if err := svc.SeedDefaults(ctx); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	groups, _ := svc.ListRoleGroups(ctx)

	accounts := service.NewAccountService(repo, auth.NewSessionIssuer([]byte("secret"), time.Hour), true, 0)
	accounts.SetPolicyService(svc)
	user, err := accounts.RegisterCustomer(ctx, "bob", "password123")
	if err != nil {
		t.Fatalf("RegisterCustomer: %v", err)
	}
	_ = user

	if err := svc.DeleteRoleGroup(ctx, groups[0].ID); !errors.Is(err, service.ErrRoleGroupInUse) {
		t.Fatalf("error = %v, want ErrRoleGroupInUse", err)
	}
}

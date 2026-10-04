package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/AXmishell/axmipic/internal/store"
)

func TestRoleGroupPolicyAttachReplacesSameType(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()

	if err := repo.CreateRoleGroup(ctx, &store.RoleGroup{ID: "g1", Name: "默认", IsDefault: true}); err != nil {
		t.Fatalf("CreateRoleGroup: %v", err)
	}
	quotaA := &store.Policy{ID: "pa", Name: "配额A", Type: store.PolicyTypeQuota, Enabled: true, Settings: `{}`}
	quotaB := &store.Policy{ID: "pb", Name: "配额B", Type: store.PolicyTypeQuota, Enabled: true, Settings: `{}`}
	upload := &store.Policy{ID: "pu", Name: "上传", Type: store.PolicyTypeUpload, Enabled: true, Settings: `{}`}
	for _, p := range []*store.Policy{quotaA, quotaB, upload} {
		if err := repo.CreatePolicy(ctx, p); err != nil {
			t.Fatalf("CreatePolicy %q: %v", p.ID, err)
		}
	}

	if err := repo.AttachPolicy(ctx, "g1", "pa"); err != nil {
		t.Fatalf("AttachPolicy pa: %v", err)
	}
	if err := repo.AttachPolicy(ctx, "g1", "pb"); err != nil {
		t.Fatalf("AttachPolicy pb: %v", err)
	}
	if err := repo.AttachPolicy(ctx, "g1", "pu"); err != nil {
		t.Fatalf("AttachPolicy pu: %v", err)
	}

	policies, err := repo.ListPoliciesByRoleGroup(ctx, "g1")
	if err != nil {
		t.Fatalf("ListPoliciesByRoleGroup: %v", err)
	}
	if len(policies) != 2 {
		t.Fatalf("policy count = %d, want 2", len(policies))
	}
	ids := map[string]bool{}
	for _, p := range policies {
		ids[p.ID] = true
	}
	if ids["pa"] || !ids["pb"] || !ids["pu"] {
		t.Fatalf("unexpected attached policies: %+v", ids)
	}
}

func TestDeleteRoleGroupAndPolicyGuards(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()

	if err := repo.CreateRoleGroup(ctx, &store.RoleGroup{ID: "g1", Name: "默认", IsDefault: true}); err != nil {
		t.Fatalf("CreateRoleGroup: %v", err)
	}
	if err := repo.CreatePolicy(ctx, &store.Policy{ID: "p1", Name: "配额", Type: store.PolicyTypeQuota, Enabled: true, Settings: `{}`}); err != nil {
		t.Fatalf("CreatePolicy: %v", err)
	}
	if err := repo.AttachPolicy(ctx, "g1", "p1"); err != nil {
		t.Fatalf("AttachPolicy: %v", err)
	}

	if err := repo.DeletePolicy(ctx, "p1"); !errors.Is(err, store.ErrPolicyInUse) {
		t.Fatalf("DeletePolicy error = %v, want ErrPolicyInUse", err)
	}

	if err := repo.CreateCustomer(ctx, &store.Customer{ID: "c1", Username: "c1", PasswordHash: "x", RoleGroupID: strPtr("g1")}); err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	if err := repo.DeleteRoleGroup(ctx, "g1"); !errors.Is(err, store.ErrRoleGroupInUse) {
		t.Fatalf("DeleteRoleGroup error = %v, want ErrRoleGroupInUse", err)
	}

	if err := repo.DetachPolicy(ctx, "g1", "p1"); err != nil {
		t.Fatalf("DetachPolicy: %v", err)
	}
	if err := repo.DeletePolicy(ctx, "p1"); err != nil {
		t.Fatalf("DeletePolicy after detach: %v", err)
	}
}

func TestSetCustomersQuotaByRoleGroup(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()

	if err := repo.CreateCustomer(ctx, &store.Customer{ID: "c1", Username: "c1", PasswordHash: "x", RoleGroupID: strPtr("g1")}); err != nil {
		t.Fatalf("CreateCustomer c1: %v", err)
	}
	if err := repo.CreateCustomer(ctx, &store.Customer{ID: "c2", Username: "c2", PasswordHash: "x"}); err != nil {
		t.Fatalf("CreateCustomer c2: %v", err)
	}

	updated, err := repo.SetCustomersQuotaByRoleGroup(ctx, "g1", 42, true)
	if err != nil {
		t.Fatalf("SetCustomersQuotaByRoleGroup: %v", err)
	}
	if updated != 2 {
		t.Fatalf("updated = %d, want 2", updated)
	}
	for _, id := range []string{"c1", "c2"} {
		account, err := repo.GetAccountByID(ctx, store.RoleCustomer, id)
		if err != nil {
			t.Fatalf("GetAccountByID %q: %v", id, err)
		}
		if account.QuotaBytes != 42 {
			t.Fatalf("customer %q quota = %d, want 42", id, account.QuotaBytes)
		}
	}
}

func strPtr(s string) *string { return &s }

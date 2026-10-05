package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/AXmishell/axmipic/internal/store"
)

func TestReserveGuestIPQuota(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	const limit = int64(100)
	window := time.Hour

	ok, err := repo.ReserveGuestIPQuota(ctx, "1.2.3.4", 60, limit, window)
	if err != nil || !ok {
		t.Fatalf("first reserve ok=%v err=%v", ok, err)
	}
	// 第二次会超出上限。
	ok, err = repo.ReserveGuestIPQuota(ctx, "1.2.3.4", 60, limit, window)
	if err != nil {
		t.Fatalf("second reserve err=%v", err)
	}
	if ok {
		t.Fatal("second reserve should exceed the limit")
	}
	// 释放后可再次预留。
	if err := repo.ReleaseGuestIPQuota(ctx, "1.2.3.4", 60); err != nil {
		t.Fatalf("release: %v", err)
	}
	if ok, err = repo.ReserveGuestIPQuota(ctx, "1.2.3.4", 100, limit, window); err != nil || !ok {
		t.Fatalf("reserve after release ok=%v err=%v", ok, err)
	}
	// 不同 IP 相互独立。
	if ok, err = repo.ReserveGuestIPQuota(ctx, "5.6.7.8", 60, limit, window); err != nil || !ok {
		t.Fatalf("other ip ok=%v err=%v", ok, err)
	}
}

func TestGuestIPQuotaWindowReset(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	const limit = int64(100)
	window := 5 * time.Millisecond

	if ok, err := repo.ReserveGuestIPQuota(ctx, "9.9.9.9", 100, limit, window); err != nil || !ok {
		t.Fatalf("first reserve ok=%v err=%v", ok, err)
	}
	time.Sleep(20 * time.Millisecond)
	if ok, err := repo.ReserveGuestIPQuota(ctx, "9.9.9.9", 100, limit, window); err != nil || !ok {
		t.Fatalf("reserve after window reset ok=%v err=%v", ok, err)
	}
}

func TestListCustomersExcludesGuests(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	if err := repo.CreateCustomer(ctx, &store.Customer{ID: "u1", Username: "alice", PasswordHash: "x"}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := repo.CreateCustomer(ctx, &store.Customer{ID: "g1", Username: "guest_abc", PasswordHash: "x", IsGuest: true}); err != nil {
		t.Fatalf("create guest: %v", err)
	}

	customers, err := repo.ListCustomers(ctx)
	if err != nil {
		t.Fatalf("ListCustomers: %v", err)
	}
	if len(customers) != 1 || customers[0].Username != "alice" {
		t.Fatalf("customers = %+v, want only alice", customers)
	}
	count, err := repo.CountCustomers(ctx)
	if err != nil || count != 1 {
		t.Fatalf("CountCustomers = %d/%v, want 1", count, err)
	}
}

func TestDeleteEmptyGuestAccounts(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	for _, c := range []*store.Customer{
		{ID: "g1", Username: "guest_empty", PasswordHash: "x", IsGuest: true, UsedBytes: 0},
		{ID: "g2", Username: "guest_used", PasswordHash: "x", IsGuest: true, UsedBytes: 10},
		{ID: "u1", Username: "alice", PasswordHash: "x"},
	} {
		if err := repo.CreateCustomer(ctx, c); err != nil {
			t.Fatalf("create %s: %v", c.Username, err)
		}
	}

	removed, err := repo.DeleteEmptyGuestAccounts(ctx, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("DeleteEmptyGuestAccounts: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
}

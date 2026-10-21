package store_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/AXmishell/axmipic/internal/store"
)

func TestListCustomersPagedFiltersAndSort(t *testing.T) {
	repo, err := store.Open("sqlite", filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	ctx := context.Background()
	for i := 1; i <= 6; i++ {
		username := fmt.Sprintf("user%d", i)
		customer := &store.Customer{
			ID:           fmt.Sprintf("id-%d", i),
			Username:     username,
			PasswordHash: "x",
			Disabled:     i == 2,
			QuotaBytes:   1 << 30,
		}
		if err := repo.CreateCustomer(ctx, customer); err != nil {
			t.Fatalf("CreateCustomer %s: %v", username, err)
		}
	}

	// 第一页按用户名升序。
	accounts, total, err := repo.ListCustomersPaged(ctx, store.CustomerListOptions{
		Offset: 0, Limit: 2, Sort: "username",
	})
	if err != nil {
		t.Fatalf("ListCustomersPaged: %v", err)
	}
	if total != 6 {
		t.Fatalf("total = %d, want 6", total)
	}
	if len(accounts) != 2 || accounts[0].Username != "user1" || accounts[1].Username != "user2" {
		t.Fatalf("unexpected first page: %+v", accounts)
	}

	// 关键字过滤。
	accounts, total, err = repo.ListCustomersPaged(ctx, store.CustomerListOptions{
		Limit: 10, Keyword: "user3",
	})
	if err != nil {
		t.Fatalf("keyword query: %v", err)
	}
	if total != 1 || len(accounts) != 1 || accounts[0].Username != "user3" {
		t.Fatalf("keyword result = %+v total=%d", accounts, total)
	}

	// 状态过滤。
	accounts, total, err = repo.ListCustomersPaged(ctx, store.CustomerListOptions{
		Limit: 10, Status: "disabled",
	})
	if err != nil {
		t.Fatalf("status query: %v", err)
	}
	if total != 1 || len(accounts) != 1 || accounts[0].Username != "user2" {
		t.Fatalf("disabled result = %+v total=%d", accounts, total)
	}
}

package store_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/AXmishell/axmipic/internal/store"
)

func TestListOrdersPagedFiltersAndPaginates(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	base := time.Now().Add(-time.Hour)
	statuses := []string{"pending", "paid", "pending"}
	for i, status := range statuses {
		order := &store.Order{
			ID:        fmt.Sprintf("order-%d", i),
			UserID:    fmt.Sprintf("user-%d", i%2),
			PlanID:    "plan-1",
			PlanName:  "套餐",
			Status:    status,
			Provider:  "manual",
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}
		if err := repo.CreateOrder(ctx, order); err != nil {
			t.Fatalf("CreateOrder: %v", err)
		}
	}

	orders, total, err := repo.ListOrdersPaged(ctx, "", "", 0, 2)
	if err != nil || total != 3 || len(orders) != 2 {
		t.Fatalf("all: total=%d len=%d err=%v", total, len(orders), err)
	}
	if orders[0].ID != "order-2" {
		t.Fatalf("newest order = %q, want order-2", orders[0].ID)
	}

	pending, total, err := repo.ListOrdersPaged(ctx, "", "pending", 0, 10)
	if err != nil || total != 2 || len(pending) != 2 {
		t.Fatalf("pending: total=%d len=%d err=%v", total, len(pending), err)
	}

	second, total, err := repo.ListOrdersPaged(ctx, "", "", 1, 1)
	if err != nil || total != 3 || len(second) != 1 || second[0].ID != "order-1" {
		t.Fatalf("offset: total=%d %+v err=%v", total, second, err)
	}

	mine, total, err := repo.ListOrdersPaged(ctx, "user-1", "", 0, 10)
	if err != nil || total != 1 || len(mine) != 1 || mine[0].ID != "order-1" {
		t.Fatalf("by user: total=%d %+v err=%v", total, mine, err)
	}
}

func TestListTicketsPagedFiltersAndPaginates(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	base := time.Now().Add(-time.Hour)
	for i, status := range []string{"open", "closed", "open"} {
		ticket := &store.Ticket{
			ID:        fmt.Sprintf("ticket-%d", i),
			UserID:    "user-1",
			Subject:   fmt.Sprintf("工单 %d", i),
			Status:    status,
			Priority:  "normal",
			UpdatedAt: base.Add(time.Duration(i) * time.Minute),
		}
		if err := repo.CreateTicket(ctx, ticket); err != nil {
			t.Fatalf("CreateTicket: %v", err)
		}
	}

	tickets, total, err := repo.ListTicketsPaged(ctx, "", "", 0, 2)
	if err != nil || total != 3 || len(tickets) != 2 || tickets[0].ID != "ticket-2" {
		t.Fatalf("all: total=%d %+v err=%v", total, tickets, err)
	}

	open, total, err := repo.ListTicketsPaged(ctx, "user-1", "open", 0, 10)
	if err != nil || total != 2 || len(open) != 2 {
		t.Fatalf("open: total=%d len=%d err=%v", total, len(open), err)
	}
}

func TestListReportsPagedFiltersAndPaginates(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	base := time.Now().Add(-time.Hour)
	for i, status := range []string{"pending", "resolved", "pending"} {
		report := &store.Report{
			ID:        fmt.Sprintf("report-%d", i),
			Reason:    "原因",
			Status:    status,
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}
		if err := repo.CreateReport(ctx, report); err != nil {
			t.Fatalf("CreateReport: %v", err)
		}
	}

	reports, total, err := repo.ListReportsPaged(ctx, "", 0, 2)
	if err != nil || total != 3 || len(reports) != 2 || reports[0].ID != "report-2" {
		t.Fatalf("all: total=%d %+v err=%v", total, reports, err)
	}

	pending, total, err := repo.ListReportsPaged(ctx, "pending", 0, 10)
	if err != nil || total != 2 || len(pending) != 2 {
		t.Fatalf("pending: total=%d len=%d err=%v", total, len(pending), err)
	}
}

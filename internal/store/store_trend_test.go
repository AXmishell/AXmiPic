package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/AXmishell/axmipic/internal/store"
)

func TestImageDailyStatsGroupsByLocalDay(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()

	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).
		AddDate(0, 0, -2)

	images := []*store.Image{
		{ID: "d1", Key: "k-d1", URL: "/i/d1", Size: 100, MimeType: "image/png", CreatedAt: start.Add(1 * time.Hour)},
		{ID: "d2", Key: "k-d2", URL: "/i/d2", Size: 250, MimeType: "image/png", CreatedAt: start.Add(5 * time.Hour)},
		{ID: "d3", Key: "k-d3", URL: "/i/d3", Size: 400, MimeType: "image/png", CreatedAt: start.AddDate(0, 0, 1).Add(2 * time.Hour)},
		// 早于查询起点的记录不应被统计。
		{ID: "old", Key: "k-old", URL: "/i/old", Size: 999, MimeType: "image/png", CreatedAt: start.Add(-time.Hour)},
	}
	for _, image := range images {
		if err := repo.Create(ctx, image); err != nil {
			t.Fatalf("Create %s: %v", image.ID, err)
		}
	}

	stats, err := repo.ImageDailyStats(ctx, start)
	if err != nil {
		t.Fatalf("ImageDailyStats: %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("got %d days, want 2: %+v", len(stats), stats)
	}
	if stats[0].Date != start.Format("2006-01-02") || stats[0].Count != 2 || stats[0].Bytes != 350 {
		t.Fatalf("unexpected first day: %+v", stats[0])
	}
	if stats[1].Count != 1 || stats[1].Bytes != 400 {
		t.Fatalf("unexpected second day: %+v", stats[1])
	}
}

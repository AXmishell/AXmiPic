package store

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// ImageDailyStats 返回 since 之后按自然日聚合的新增图像数量与字节数。
//
// 这里刻意不在 SQL 中做日期分组：SQLite、MySQL 与 PostgreSQL 的日期函数和
// 时区语义并不一致，容易在跨日边界上产生偏差。只取最近若干天的 created_at
// 与 size 两列，在 Go 中按本地时区归并，行为在三种数据库上完全一致。
func (r *Repository) ImageDailyStats(ctx context.Context, since time.Time) ([]ImageDailyStat, error) {
	var rows []struct {
		CreatedAt time.Time
		Size      int64
	}
	if err := r.db.WithContext(ctx).Model(&Image{}).
		Select("created_at, size").
		Where("created_at >= ?", since).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("store: image daily stats: %w", err)
	}

	byDate := make(map[string]*ImageDailyStat, len(rows))
	for _, row := range rows {
		date := row.CreatedAt.In(time.Local).Format("2006-01-02")
		stat, ok := byDate[date]
		if !ok {
			stat = &ImageDailyStat{Date: date}
			byDate[date] = stat
		}
		stat.Count++
		stat.Bytes += row.Size
	}

	stats := make([]ImageDailyStat, 0, len(byDate))
	for _, stat := range byDate {
		stats = append(stats, *stat)
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].Date < stats[j].Date })
	return stats, nil
}

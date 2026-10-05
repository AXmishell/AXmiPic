package store

import (
	"context"
	"fmt"
	"time"
)

// CreateNotifyLog 插入一条通知发送记录。
func (r *Repository) CreateNotifyLog(ctx context.Context, log *NotifyLog) error {
	if err := r.db.WithContext(ctx).Create(log).Error; err != nil {
		return fmt.Errorf("store: create notify log: %w", err)
	}
	return nil
}

// ListNotifyLogs 返回一页通知记录（按时间倒序）及总数。channel 非空时按其过滤。
func (r *Repository) ListNotifyLogs(ctx context.Context, channel string, offset, limit int) ([]NotifyLog, int64, error) {
	countQuery := r.db.WithContext(ctx).Model(&NotifyLog{})
	listQuery := r.db.WithContext(ctx).Model(&NotifyLog{})
	if channel != "" {
		countQuery = countQuery.Where("channel = ?", channel)
		listQuery = listQuery.Where("channel = ?", channel)
	}
	var total int64
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("store: count notify logs: %w", err)
	}
	var logs []NotifyLog
	if err := listQuery.Order("created_at DESC").Offset(offset).Limit(limit).Find(&logs).Error; err != nil {
		return nil, 0, fmt.Errorf("store: list notify logs: %w", err)
	}
	return logs, total, nil
}

// DeleteNotifyLogsBefore 删除创建时间早于 cutoff 的通知记录，返回删除数量。
func (r *Repository) DeleteNotifyLogsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	result := r.db.WithContext(ctx).Where("created_at < ?", cutoff).Delete(&NotifyLog{})
	if result.Error != nil {
		return 0, fmt.Errorf("store: prune notify logs: %w", result.Error)
	}
	return result.RowsAffected, nil
}

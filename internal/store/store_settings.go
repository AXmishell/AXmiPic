package store

import (
	"context"
	"fmt"

	"gorm.io/gorm/clause"
)

// GetSetting 返回指定键的设置值，记录不存在时返回 ErrNotFound。
func (r *Repository) GetSetting(ctx context.Context, key string) (string, error) {
	var settings []Setting
	// 使用 Find 而非 First：记录不存在时不触发 GORM 的「record not found」日志。
	if err := r.db.WithContext(ctx).Where(map[string]any{"key": key}).Limit(1).Find(&settings).Error; err != nil {
		return "", fmt.Errorf("store: get setting %q: %w", key, err)
	}
	if len(settings) == 0 {
		return "", fmt.Errorf("store: setting %q: %w", key, ErrNotFound)
	}
	return settings[0].Value, nil
}

// SetSetting 写入或覆盖指定键的设置值。
func (r *Repository) SetSetting(ctx context.Context, key, value string) error {
	setting := Setting{Key: key, Value: value}
	if err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{UpdateAll: true}).
		Create(&setting).Error; err != nil {
		return fmt.Errorf("store: set setting %q: %w", key, err)
	}
	return nil
}

package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UpsertEmailCode 写入或替换某个 key 的邮箱验证码，并顺带清理已过期记录。
func (r *Repository) UpsertEmailCode(ctx context.Context, key, email, code string, expiresAt time.Time) error {
	if err := r.DeleteExpiredEmailCodes(ctx, time.Now()); err != nil {
		return err
	}
	row := EmailCode{
		Key:       key,
		Email:     email,
		Code:      code,
		Attempts:  0,
		ExpiresAt: expiresAt,
	}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"email", "code", "attempts", "expires_at", "updated_at"}),
	}).Create(&row).Error; err != nil {
		return fmt.Errorf("store: upsert email code: %w", err)
	}
	return nil
}

// EmailCode 返回某个 key 的验证码，或 ErrNotFound。
func (r *Repository) EmailCode(ctx context.Context, key string) (*EmailCode, error) {
	var row EmailCode
	err := r.db.WithContext(ctx).First(&row, "key = ?", key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("store: email code %q: %w", key, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: get email code %q: %w", key, err)
	}
	return &row, nil
}

// SaveEmailCodeAttempts 更新某个 key 的失败尝试次数。
func (r *Repository) SaveEmailCodeAttempts(ctx context.Context, key string, attempts int) error {
	if err := r.db.WithContext(ctx).Model(&EmailCode{}).Where("key = ?", key).
		UpdateColumn("attempts", attempts).Error; err != nil {
		return fmt.Errorf("store: update email code attempts %q: %w", key, err)
	}
	return nil
}

// DeleteEmailCode 删除某个 key 的验证码；不存在时为无操作。
func (r *Repository) DeleteEmailCode(ctx context.Context, key string) error {
	if err := r.db.WithContext(ctx).Delete(&EmailCode{}, "key = ?", key).Error; err != nil {
		return fmt.Errorf("store: delete email code %q: %w", key, err)
	}
	return nil
}

// DeleteEmailCodeIfMatches 仅当 key 存在且验证码仍匹配时删除，返回是否删除成功。
// 用于原子地消费验证码，防止并发重放。
func (r *Repository) DeleteEmailCodeIfMatches(ctx context.Context, key, code string) (bool, error) {
	result := r.db.WithContext(ctx).Where("key = ? AND code = ?", key, code).Delete(&EmailCode{})
	if result.Error != nil {
		return false, fmt.Errorf("store: consume email code %q: %w", key, result.Error)
	}
	return result.RowsAffected == 1, nil
}

// DeleteExpiredEmailCodes 删除截至 now 已过期的验证码。
func (r *Repository) DeleteExpiredEmailCodes(ctx context.Context, now time.Time) error {
	if err := r.db.WithContext(ctx).Where("expires_at < ?", now).Delete(&EmailCode{}).Error; err != nil {
		return fmt.Errorf("store: delete expired email codes: %w", err)
	}
	return nil
}

// EmailDailyCount 返回某个 key 在指定自然日的验证码发送次数。
func (r *Repository) EmailDailyCount(ctx context.Context, key, day string) (int, error) {
	var row EmailCodeStat
	err := r.db.WithContext(ctx).First(&row, "key = ? AND day = ?", key, day).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("store: get email daily count %q: %w", key, err)
	}
	return row.Count, nil
}

// IncrementEmailDaily 原子地累加某个 key 在指定自然日的发送次数并返回新值。
func (r *Repository) IncrementEmailDaily(ctx context.Context, key, day string) (int, error) {
	row := EmailCodeStat{Key: key, Day: day, Count: 1}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "key"}, {Name: "day"}},
		DoUpdates: clause.Assignments(map[string]any{
			"count":      gorm.Expr("count + 1"),
			"updated_at": time.Now(),
		}),
	}).Create(&row).Error; err != nil {
		return 0, fmt.Errorf("store: increment email daily %q: %w", key, err)
	}
	return r.EmailDailyCount(ctx, key, day)
}

package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GuestIPHash 返回客户端 IP 的 sha256 十六进制摘要，用于匿名访客的 IP 配额记账。
func GuestIPHash(ip string) string {
	sum := sha256.Sum256([]byte(ip))
	return hex.EncodeToString(sum[:])
}

// ReserveGuestIPQuota 在固定时间窗口内为某个 IP 预留 amount 字节。窗口过期时先
// 重置计数。limit<=0 表示不限，此时直接放行。返回是否预留成功。
func (r *Repository) ReserveGuestIPQuota(ctx context.Context, ip string, amount int64, limit int64, window time.Duration) (bool, error) {
	if limit <= 0 {
		return true, nil
	}
	hash := GuestIPHash(ip)
	now := time.Now()

	// 确保存在该 IP 的记账行；并发创建时忽略冲突。
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).
		Create(&GuestIPUsage{IPHash: hash, WindowStart: now}).Error; err != nil {
		return false, fmt.Errorf("store: init guest ip usage: %w", err)
	}
	// 窗口过期则重置。
	if err := r.db.WithContext(ctx).Model(&GuestIPUsage{}).
		Where("ip_hash = ? AND window_start <= ?", hash, now.Add(-window)).
		Updates(map[string]any{"used_bytes": 0, "window_start": now}).Error; err != nil {
		return false, fmt.Errorf("store: reset guest ip usage: %w", err)
	}
	// 原子条件自增：仅当不超过上限时才累加，避免并发超发。
	result := r.db.WithContext(ctx).Model(&GuestIPUsage{}).
		Where("ip_hash = ? AND used_bytes + ? <= ?", hash, amount, limit).
		UpdateColumn("used_bytes", gorm.Expr("used_bytes + ?", amount))
	if result.Error != nil {
		return false, fmt.Errorf("store: reserve guest ip quota: %w", result.Error)
	}
	return result.RowsAffected == 1, nil
}

// ReleaseGuestIPQuota 归还某个 IP 之前预留的字节数。
func (r *Repository) ReleaseGuestIPQuota(ctx context.Context, ip string, amount int64) error {
	if amount <= 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&GuestIPUsage{}).
		Where("ip_hash = ? AND used_bytes >= ?", GuestIPHash(ip), amount).
		UpdateColumn("used_bytes", gorm.Expr("used_bytes - ?", amount)).Error; err != nil {
		return fmt.Errorf("store: release guest ip quota: %w", err)
	}
	return nil
}

// DeleteStaleGuestIPUsage 删除窗口起点早于 before 的访客 IP 用量记录。
func (r *Repository) DeleteStaleGuestIPUsage(ctx context.Context, before time.Time) (int64, error) {
	result := r.db.WithContext(ctx).Where("window_start < ?", before).Delete(&GuestIPUsage{})
	if result.Error != nil {
		return 0, fmt.Errorf("store: delete stale guest ip usage: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// DeleteEmptyGuestAccounts 删除在 before 之前创建、且未占用任何存储的访客账户，
// 用于清理因更换 cookie 而不断产生的空账户。
func (r *Repository) DeleteEmptyGuestAccounts(ctx context.Context, before time.Time) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("is_guest = ? AND used_bytes = 0 AND updated_at < ?", true, before).
		Delete(&Customer{})
	if result.Error != nil {
		return 0, fmt.Errorf("store: delete empty guest accounts: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// CountGuestAccounts 返回内置访客账户的数量。
func (r *Repository) CountGuestAccounts(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&Customer{}).Where("is_guest = ?", true).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("store: count guest accounts: %w", err)
	}
	return count, nil
}

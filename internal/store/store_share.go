package store

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// CreateShare 插入一个分享链接。
func (r *Repository) CreateShare(ctx context.Context, share *Share) error {
	if err := r.db.WithContext(ctx).Create(share).Error; err != nil {
		return fmt.Errorf("store: create share: %w", err)
	}
	return nil
}

// GetShareByID 返回具有给定 id 的分享，或 ErrNotFound。
func (r *Repository) GetShareByID(ctx context.Context, id string) (*Share, error) {
	var share Share
	if err := r.first(ctx, &share, "id = ?", id); err != nil {
		return nil, fmt.Errorf("store: share %q: %w", id, err)
	}
	return &share, nil
}

// GetShareByToken 返回具有给定 token 的分享，或 ErrNotFound。
func (r *Repository) GetShareByToken(ctx context.Context, token string) (*Share, error) {
	var share Share
	if err := r.first(ctx, &share, "token = ?", token); err != nil {
		return nil, fmt.Errorf("store: share token %q: %w", token, err)
	}
	return &share, nil
}

// ListSharesByUser 返回某个用户创建的分享，最新的在前。
func (r *Repository) ListSharesByUser(ctx context.Context, userID string) ([]Share, error) {
	var shares []Share
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).
		Order("created_at DESC").Find(&shares).Error; err != nil {
		return nil, fmt.Errorf("store: list shares: %w", err)
	}
	return shares, nil
}

// DeleteShare 移除某个用户拥有的分享，或返回 ErrNotFound。
func (r *Repository) DeleteShare(ctx context.Context, userID, id string) error {
	result := r.db.WithContext(ctx).Delete(&Share{}, "id = ? AND user_id = ?", id, userID)
	if result.Error != nil {
		return fmt.Errorf("store: delete share %q: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("store: share %q: %w", id, ErrNotFound)
	}
	return nil
}

// ConsumeShareView 在未超过访问次数上限时原子性地增加一次访问计数。它返回
// 本次访问是否被允许。
func (r *Repository) ConsumeShareView(ctx context.Context, id string) (bool, error) {
	result := r.db.WithContext(ctx).Model(&Share{}).
		Where("id = ? AND (max_views = 0 OR view_count < max_views)", id).
		UpdateColumn("view_count", gorm.Expr("view_count + 1"))
	if result.Error != nil {
		return false, fmt.Errorf("store: consume share view %q: %w", id, result.Error)
	}
	return result.RowsAffected == 1, nil
}

// DeleteSharesForTarget 在图片或相册被删除时清理其分享链接。
func (r *Repository) DeleteSharesForTarget(ctx context.Context, targetType, targetID string) error {
	if err := r.db.WithContext(ctx).
		Where("target_type = ? AND target_id = ?", targetType, targetID).
		Delete(&Share{}).Error; err != nil {
		return fmt.Errorf("store: delete shares for target: %w", err)
	}
	return nil
}

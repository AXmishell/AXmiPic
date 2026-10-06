package store

import (
	"context"
	"fmt"
)

// purgeChunkSize 限制 IN 查询的绑定参数数量，避免超出数据库上限。
const purgeChunkSize = 500

// ImageObjectRef 是删除用户内容时用于清理存储对象的一条引用。
type ImageObjectRef struct {
	Key       string
	StorageID *string
}

// ObjectRefsByUser 返回某个用户全部图片的对象引用（键与存储后端）。
func (r *Repository) ObjectRefsByUser(ctx context.Context, userID string) ([]ImageObjectRef, error) {
	var refs []ImageObjectRef
	if err := r.db.WithContext(ctx).Model(&Image{}).
		Select("key, storage_id").
		Where("user_id = ?", userID).
		Find(&refs).Error; err != nil {
		return nil, fmt.Errorf("store: list object refs for %q: %w", userID, err)
	}
	return refs, nil
}

// DeleteImagesByUser 删除某个用户的全部图片记录，返回删除的行数。
func (r *Repository) DeleteImagesByUser(ctx context.Context, userID string) (int64, error) {
	result := r.db.WithContext(ctx).Delete(&Image{}, "user_id = ?", userID)
	if result.Error != nil {
		return 0, fmt.Errorf("store: delete images for %q: %w", userID, result.Error)
	}
	return result.RowsAffected, nil
}

// DeleteAlbumsByUser 删除某个用户的全部相册记录。
func (r *Repository) DeleteAlbumsByUser(ctx context.Context, userID string) (int64, error) {
	result := r.db.WithContext(ctx).Delete(&Album{}, "user_id = ?", userID)
	if result.Error != nil {
		return 0, fmt.Errorf("store: delete albums for %q: %w", userID, result.Error)
	}
	return result.RowsAffected, nil
}

// DeleteSharesByUser 删除某个用户的全部分享记录。
func (r *Repository) DeleteSharesByUser(ctx context.Context, userID string) (int64, error) {
	result := r.db.WithContext(ctx).Delete(&Share{}, "user_id = ?", userID)
	if result.Error != nil {
		return 0, fmt.Errorf("store: delete shares for %q: %w", userID, result.Error)
	}
	return result.RowsAffected, nil
}

// DeleteTokensByUser 删除某个用户的全部 API 令牌记录。
func (r *Repository) DeleteTokensByUser(ctx context.Context, userID string) (int64, error) {
	result := r.db.WithContext(ctx).Delete(&APIToken{}, "user_id = ?", userID)
	if result.Error != nil {
		return 0, fmt.Errorf("store: delete tokens for %q: %w", userID, result.Error)
	}
	return result.RowsAffected, nil
}

// ExistingImageKeys 返回输入键集合中实际存在于 images 表中的子集。它按块查询
// 以避免超出数据库的绑定参数上限，用于孤儿对象对账。
func (r *Repository) ExistingImageKeys(ctx context.Context, keys []string) (map[string]struct{}, error) {
	existing := make(map[string]struct{}, len(keys))
	for start := 0; start < len(keys); start += purgeChunkSize {
		end := start + purgeChunkSize
		if end > len(keys) {
			end = len(keys)
		}
		var found []string
		if err := r.db.WithContext(ctx).Model(&Image{}).
			Where("key IN ?", keys[start:end]).
			Pluck("key", &found).Error; err != nil {
			return nil, fmt.Errorf("store: find existing image keys: %w", err)
		}
		for _, key := range found {
			existing[key] = struct{}{}
		}
	}
	return existing, nil
}

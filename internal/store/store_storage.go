package store

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// ListStorageBackends 返回所有已配置的存储后端，按创建时间升序。
func (r *Repository) ListStorageBackends(ctx context.Context) ([]StorageBackend, error) {
	var backends []StorageBackend
	if err := r.db.WithContext(ctx).Order("created_at ASC").Find(&backends).Error; err != nil {
		return nil, fmt.Errorf("store: list storage backends: %w", err)
	}
	return backends, nil
}

// GetStorageBackend 返回指定 id 的存储后端，不存在时返回 ErrNotFound。
func (r *Repository) GetStorageBackend(ctx context.Context, id string) (*StorageBackend, error) {
	var b StorageBackend
	if err := r.first(ctx, &b, "id = ?", id); err != nil {
		return nil, fmt.Errorf("store: storage backend %q: %w", id, err)
	}
	return &b, nil
}

// CreateStorageBackend 新增一条存储后端记录。
func (r *Repository) CreateStorageBackend(ctx context.Context, b *StorageBackend) error {
	if err := r.db.WithContext(ctx).Create(b).Error; err != nil {
		return fmt.Errorf("store: create storage backend: %w", err)
	}
	return nil
}

// UpdateStorageBackend 覆盖更新一条存储后端记录（含密钥与当前标记）。
func (r *Repository) UpdateStorageBackend(ctx context.Context, b *StorageBackend) error {
	result := r.db.WithContext(ctx).Model(&StorageBackend{}).Where("id = ?", b.ID).Updates(map[string]any{
		"name":       b.Name,
		"driver":     b.Driver,
		"settings":   b.Settings,
		"secrets":    b.Secrets,
		"is_current": b.IsCurrent,
	})
	if result.Error != nil {
		return fmt.Errorf("store: update storage backend %q: %w", b.ID, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("store: storage backend %q: %w", b.ID, ErrNotFound)
	}
	return nil
}

// DeleteStorageBackend 删除一条存储后端记录。
func (r *Repository) DeleteStorageBackend(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).Delete(&StorageBackend{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("store: delete storage backend %q: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("store: storage backend %q: %w", id, ErrNotFound)
	}
	return nil
}

// SetCurrentStorageBackend 在事务中把指定后端标记为当前默认，并清除其余标记。
func (r *Repository) SetCurrentStorageBackend(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&StorageBackend{}).Where("is_current = ?", true).
			Update("is_current", false).Error; err != nil {
			return err
		}
		result := tx.Model(&StorageBackend{}).Where("id = ?", id).
			Update("is_current", true)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// CountImagesOnStorage 统计某存储后端上的图片数量。
func (r *Repository) CountImagesOnStorage(ctx context.Context, storageID string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&Image{}).Where("storage_id = ?", storageID).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("store: count images on storage %q: %w", storageID, err)
	}
	return count, nil
}

// ErrStorageInUse 表示存储后端上仍有图片，不能删除。
var ErrStorageInUse = errors.New("store: storage backend still has images")

package store

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// CreateAlbum 插入一个相册。
func (r *Repository) CreateAlbum(ctx context.Context, album *Album) error {
	if err := r.db.WithContext(ctx).Create(album).Error; err != nil {
		return fmt.Errorf("store: create album: %w", err)
	}
	return nil
}

// GetAlbumByID 返回具有给定 id 的相册，或 ErrNotFound。
func (r *Repository) GetAlbumByID(ctx context.Context, id string) (*Album, error) {
	var album Album
	if err := r.first(ctx, &album, "id = ?", id); err != nil {
		return nil, fmt.Errorf("store: album %q: %w", id, err)
	}
	return &album, nil
}

// ListAlbums 返回相册列表。userID 非空时仅返回该用户的相册，并附带每个相册
// 的图片数量与所属用户名。计数通过单次聚合查询完成，避免逐个相册查询的 N+1 问题。
func (r *Repository) ListAlbums(ctx context.Context, userID string) ([]AlbumWithCount, error) {
	return r.listAlbums(ctx, userID, "")
}

// ListPublicAlbums 返回可见性为 public 的相册；userID 非空时仅返回该用户的。
func (r *Repository) ListPublicAlbums(ctx context.Context, userID string) ([]AlbumWithCount, error) {
	return r.listAlbums(ctx, userID, PermissionPublic)
}

// listAlbums 按可选的所有者与可见性过滤相册，并聚合图片数量与所有者用户名。
func (r *Repository) listAlbums(ctx context.Context, userID, permission string) ([]AlbumWithCount, error) {
	const columns = "albums.id, albums.user_id, albums.name, albums.intro, albums.permission, albums.created_at, albums.updated_at"
	query := r.db.WithContext(ctx).Table("albums").
		Select(columns + ", COALESCE(customers.username, '') AS owner_username, COUNT(images.id) AS image_count").
		Joins("LEFT JOIN images ON images.album_id = albums.id").
		Joins("LEFT JOIN customers ON customers.id = albums.user_id").
		Group(columns + ", COALESCE(customers.username, '')").
		Order("albums.created_at ASC")
	if userID != "" {
		query = query.Where("albums.user_id = ?", userID)
	}
	if permission != "" {
		query = query.Where("albums.permission = ?", permission)
	}
	var rows []albumCountRow
	if err := query.Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("store: list albums: %w", err)
	}
	result := make([]AlbumWithCount, 0, len(rows))
	for i := range rows {
		result = append(result, rows[i].albumWithCount())
	}
	return result, nil
}

// albumCountRow 是「相册 + 图片计数 + 所有者」聚合查询的扫描结果。
type albumCountRow struct {
	ID            string
	UserID        *string
	Name          string
	Intro         string
	Permission    string
	OwnerUsername string `gorm:"column:owner_username"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ImageCount    int64 `gorm:"column:image_count"`
}

// albumWithCount 将聚合行转换为领域模型。
func (row albumCountRow) albumWithCount() AlbumWithCount {
	return AlbumWithCount{
		Album: Album{
			ID:         row.ID,
			UserID:     row.UserID,
			Name:       row.Name,
			Intro:      row.Intro,
			Permission: row.Permission,
			CreatedAt:  row.CreatedAt,
			UpdatedAt:  row.UpdatedAt,
		},
		OwnerUsername: row.OwnerUsername,
		ImageCount:    row.ImageCount,
	}
}

// AlbumWithCount 是相册及其图片数量与所有者用户名。
type AlbumWithCount struct {
	Album         Album
	OwnerUsername string
	ImageCount    int64
}

// UpdateAlbum 更新相册的名称、简介与可见性。
func (r *Repository) UpdateAlbum(ctx context.Context, id, name, intro, permission string) (*Album, error) {
	result := r.db.WithContext(ctx).Model(&Album{}).Where("id = ?", id).
		Updates(map[string]any{"name": name, "intro": intro, "permission": permission})
	if result.Error != nil {
		return nil, fmt.Errorf("store: update album %q: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, fmt.Errorf("store: album %q: %w", id, ErrNotFound)
	}
	return r.GetAlbumByID(ctx, id)
}

// CountPublicImagesByUser 统计某个用户可见性为 public 的图片数量。
func (r *Repository) CountPublicImagesByUser(ctx context.Context, userID string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&Image{}).
		Where("user_id = ? AND permission = ?", userID, PermissionPublic).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("store: count public images for %q: %w", userID, err)
	}
	return count, nil
}

// DeleteAlbum 删除相册，并把其中的图片移出相册（album_id 置空）。
func (r *Repository) DeleteAlbum(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Image{}).Where("album_id = ?", id).
			UpdateColumn("album_id", nil).Error; err != nil {
			return err
		}
		result := tx.Delete(&Album{}, "id = ?", id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// CountImagesInAlbum 统计相册中的图片数量。
func (r *Repository) CountImagesInAlbum(ctx context.Context, albumID string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&Image{}).Where("album_id = ?", albumID).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("store: count images in album %q: %w", albumID, err)
	}
	return count, nil
}

// SetImagePermission 批量设置图片的可见性。
func (r *Repository) SetImagePermission(ctx context.Context, ids []string, permission string) error {
	if len(ids) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&Image{}).Where("id IN ?", ids).
		UpdateColumn("permission", permission).Error; err != nil {
		return fmt.Errorf("store: set image permission: %w", err)
	}
	return nil
}

// SetImageAlbum 批量把图片移动到相册（albumID 为 nil 表示移出相册）。
func (r *Repository) SetImageAlbum(ctx context.Context, ids []string, albumID *string) error {
	if len(ids) == 0 {
		return nil
	}
	var value any
	if albumID != nil {
		value = *albumID
	}
	if err := r.db.WithContext(ctx).Model(&Image{}).Where("id IN ?", ids).
		UpdateColumn("album_id", value).Error; err != nil {
		return fmt.Errorf("store: set image album: %w", err)
	}
	return nil
}

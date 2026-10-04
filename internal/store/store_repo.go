package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Create inserts image metadata.
func (r *Repository) Create(ctx context.Context, image *Image) error {
	if err := r.db.WithContext(ctx).Create(image).Error; err != nil {
		return fmt.Errorf("store: create image: %w", err)
	}
	return nil
}

// GetByID returns the image with the given id, or ErrNotFound.
func (r *Repository) GetByID(ctx context.Context, id string) (*Image, error) {
	var image Image
	err := r.db.WithContext(ctx).First(&image, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("store: image %q: %w", id, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: get image %q: %w", id, err)
	}
	return &image, nil
}

// GetByKey returns the image stored under key, or ErrNotFound.
func (r *Repository) GetByKey(ctx context.Context, key string) (*Image, error) {
	var image Image
	err := r.db.WithContext(ctx).First(&image, "key = ?", key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("store: image key %q: %w", key, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: get image by key %q: %w", key, err)
	}
	return &image, nil
}

// List returns a page of images ordered by creation time (newest first) along
// with the total number of records. When userID is non-empty, only images owned
// by that user are returned.
func (r *Repository) List(ctx context.Context, userID string, offset, limit int) ([]Image, int64, error) {
	countQuery := r.db.WithContext(ctx).Model(&Image{})
	listQuery := r.db.WithContext(ctx).Model(&Image{})
	if userID != "" {
		countQuery = countQuery.Where("user_id = ?", userID)
		listQuery = listQuery.Where("user_id = ?", userID)
	}

	var total int64
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("store: count images: %w", err)
	}
	var images []Image
	if err := listQuery.
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&images).Error; err != nil {
		return nil, 0, fmt.Errorf("store: list images: %w", err)
	}
	return images, total, nil
}

// Delete removes the image with the given id, or returns ErrNotFound.
func (r *Repository) Delete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).Delete(&Image{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("store: delete image %q: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("store: image %q: %w", id, ErrNotFound)
	}
	return nil
}

// CreatePendingUpload records a pending presigned upload.
func (r *Repository) CreatePendingUpload(ctx context.Context, pending *PendingUpload) error {
	if err := r.db.WithContext(ctx).Create(pending).Error; err != nil {
		return fmt.Errorf("store: create pending upload: %w", err)
	}
	return nil
}

// GetPendingUpload returns the pending upload for key, or ErrNotFound.
func (r *Repository) GetPendingUpload(ctx context.Context, key string) (*PendingUpload, error) {
	var pending PendingUpload
	err := r.db.WithContext(ctx).First(&pending, "key = ?", key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("store: pending upload %q: %w", key, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: get pending upload %q: %w", key, err)
	}
	return &pending, nil
}

// DeletePendingUpload removes a pending upload. Deleting a missing row is a
// no-op.
func (r *Repository) DeletePendingUpload(ctx context.Context, key string) error {
	if err := r.db.WithContext(ctx).Delete(&PendingUpload{}, "key = ?", key).Error; err != nil {
		return fmt.Errorf("store: delete pending upload %q: %w", key, err)
	}
	return nil
}

// ExpiredPendingUploads returns pending uploads whose expiry is before cutoff.
func (r *Repository) ExpiredPendingUploads(ctx context.Context, cutoff time.Time) ([]PendingUpload, error) {
	var pending []PendingUpload
	if err := r.db.WithContext(ctx).
		Where("expires_at < ?", cutoff).
		Find(&pending).Error; err != nil {
		return nil, fmt.Errorf("store: list expired pending uploads: %w", err)
	}
	return pending, nil
}

// CreateToken inserts an API token.
func (r *Repository) CreateToken(ctx context.Context, token *APIToken) error {
	if err := r.db.WithContext(ctx).Create(token).Error; err != nil {
		return fmt.Errorf("store: create token: %w", err)
	}
	return nil
}

// GetTokenByHash returns the token with the given hash, or ErrNotFound.
func (r *Repository) GetTokenByHash(ctx context.Context, hash string) (*APIToken, error) {
	var token APIToken
	err := r.db.WithContext(ctx).First(&token, "token_hash = ?", hash).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("store: token: %w", ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: get token: %w", err)
	}
	return &token, nil
}

// ListTokensByUser returns a user's tokens, newest first.
func (r *Repository) ListTokensByUser(ctx context.Context, userID string) ([]APIToken, error) {
	var tokens []APIToken
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&tokens).Error; err != nil {
		return nil, fmt.Errorf("store: list tokens for %q: %w", userID, err)
	}
	return tokens, nil
}

// DeleteToken removes a user's token, or returns ErrNotFound.
func (r *Repository) DeleteToken(ctx context.Context, userID, id string) error {
	result := r.db.WithContext(ctx).Delete(&APIToken{}, "id = ? AND user_id = ?", id, userID)
	if result.Error != nil {
		return fmt.Errorf("store: delete token %q: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("store: token %q: %w", id, ErrNotFound)
	}
	return nil
}

// TouchToken records a token's last-used time.
func (r *Repository) TouchToken(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).Model(&APIToken{}).
		Where("id = ?", id).
		UpdateColumn("last_used_at", time.Now()).Error; err != nil {
		return fmt.Errorf("store: touch token %q: %w", id, err)
	}
	return nil
}

// ImageStats returns the image count and total stored bytes.
func (r *Repository) ImageStats(ctx context.Context) (ImageStats, error) {
	var stats ImageStats
	if err := r.db.WithContext(ctx).Model(&Image{}).Count(&stats.Count).Error; err != nil {
		return ImageStats{}, fmt.Errorf("store: count images: %w", err)
	}
	if err := r.db.WithContext(ctx).Model(&Image{}).
		Select("COALESCE(SUM(size), 0)").
		Scan(&stats.TotalBytes).Error; err != nil {
		return ImageStats{}, fmt.Errorf("store: sum image size: %w", err)
	}
	return stats, nil
}

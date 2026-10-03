// Package store provides persistence for AXmiPic metadata.
package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("store: record not found")

// Repository provides persistence operations for image metadata.
type Repository struct {
	db *gorm.DB
}

// Open opens the SQLite database at dsn, runs migrations, and returns a
// ready-to-use repository.
func Open(dsn string) (*Repository, error) {
	if err := ensureSQLiteDir(dsn); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("store: open %q: %w", dsn, err)
	}
	if err := db.AutoMigrate(&Image{}, &PendingUpload{}); err != nil {
		return nil, fmt.Errorf("store: migrate: %w", err)
	}
	return &Repository{db: db}, nil
}

// Close releases the underlying database connection.
func (r *Repository) Close() error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return fmt.Errorf("store: connection: %w", err)
	}
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("store: close: %w", err)
	}
	return nil
}

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
// with the total number of records.
func (r *Repository) List(ctx context.Context, offset, limit int) ([]Image, int64, error) {
	var total int64
	if err := r.db.WithContext(ctx).Model(&Image{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("store: count images: %w", err)
	}
	var images []Image
	if err := r.db.WithContext(ctx).
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

// ensureSQLiteDir creates the parent directory for a file-backed SQLite DSN.
func ensureSQLiteDir(dsn string) error {
	if dsn == "" || dsn == ":memory:" || strings.HasPrefix(dsn, "file:") {
		return nil
	}
	dir := filepath.Dir(dsn)
	if dir == "." || dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("store: create database directory %q: %w", dir, err)
	}
	return nil
}

package store

import "time"

// Image is the persisted metadata for a single stored object.
type Image struct {
	ID        string `gorm:"primaryKey;size:36"`
	Key       string `gorm:"uniqueIndex;size:255;not null"`
	URL       string `gorm:"size:512;not null"`
	Size      int64  `gorm:"not null"`
	MimeType  string `gorm:"size:100;not null"`
	Width     int
	Height    int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName returns the table backing Image.
func (Image) TableName() string {
	return "images"
}

// PendingUpload tracks a presigned direct upload that has not been confirmed
// yet. The row is created when a presigned request is issued and consumed by
// the confirm step, which prevents clients from claiming arbitrary keys.
type PendingUpload struct {
	Key       string    `gorm:"primaryKey;size:255"`
	MimeType  string    `gorm:"size:100;not null"`
	MaxSize   int64     `gorm:"not null"`
	ExpiresAt time.Time `gorm:"index;not null"`
	CreatedAt time.Time
}

// TableName returns the table backing PendingUpload.
func (PendingUpload) TableName() string {
	return "pending_uploads"
}

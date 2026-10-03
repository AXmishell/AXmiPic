package store

import "time"

// Image is the persisted metadata for a single stored object.
type Image struct {
	ID        string  `gorm:"primaryKey;size:36"`
	Key       string  `gorm:"uniqueIndex;size:255;not null"`
	UserID    *string `gorm:"index;size:36"`
	URL       string  `gorm:"size:512;not null"`
	Size      int64   `gorm:"not null"`
	MimeType  string  `gorm:"size:100;not null"`
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
	UserID    *string   `gorm:"index;size:36"`
	MimeType  string    `gorm:"size:100;not null"`
	MaxSize   int64     `gorm:"not null"`
	ExpiresAt time.Time `gorm:"index;not null"`
	CreatedAt time.Time
}

// TableName returns the table backing PendingUpload.
func (PendingUpload) TableName() string {
	return "pending_uploads"
}

// User is a registered account.
type User struct {
	ID           string `gorm:"primaryKey;size:36"`
	Username     string `gorm:"uniqueIndex;size:64;not null"`
	PasswordHash string `gorm:"size:100;not null"`
	Role         string `gorm:"size:16;not null"`
	Disabled     bool   `gorm:"not null;default:false"`
	UsedBytes    int64  `gorm:"not null;default:0"`
	QuotaBytes   int64  `gorm:"not null;default:0"` // 0 means unlimited
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// TableName returns the table backing User.
func (User) TableName() string {
	return "users"
}

// UserUpdate carries optional account field changes. Nil fields are ignored.
type UserUpdate struct {
	Role     *string
	Disabled *bool
}

// ImageStats aggregates image counts and total bytes.
type ImageStats struct {
	Count      int64
	TotalBytes int64
}

// APIToken is a long-lived programmatic credential. Only the hash is stored.
type APIToken struct {
	ID         string `gorm:"primaryKey;size:36"`
	UserID     string `gorm:"index;size:36;not null"`
	Name       string `gorm:"size:100"`
	TokenHash  string `gorm:"uniqueIndex;size:64;not null"`
	Prefix     string `gorm:"size:16;not null"`
	LastUsedAt *time.Time
	ExpiresAt  *time.Time
	CreatedAt  time.Time
}

// TableName returns the table backing APIToken.
func (APIToken) TableName() string {
	return "api_tokens"
}

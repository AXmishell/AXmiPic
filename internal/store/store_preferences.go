package store

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// GetUserPreference 返回某个用户的偏好 JSON 字符串；不存在时返回空字符串。
func (r *Repository) GetUserPreference(ctx context.Context, userID string) (string, error) {
	var pref UserPreference
	err := r.db.WithContext(ctx).First(&pref, "user_id = ?", userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("store: get user preference %q: %w", userID, err)
	}
	return pref.Data, nil
}

// UpsertUserPreference 写入或替换某个用户的偏好 JSON。
func (r *Repository) UpsertUserPreference(ctx context.Context, userID, data string) error {
	pref := UserPreference{UserID: userID, Data: data}
	if err := r.db.WithContext(ctx).Save(&pref).Error; err != nil {
		return fmt.Errorf("store: save user preference %q: %w", userID, err)
	}
	return nil
}

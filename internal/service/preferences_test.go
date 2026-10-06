package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/service"
)

// TestUserPreferencesMergeAndValidate 验证用户偏好可持久化、未知键被忽略、
// 已知键的非法取值被拒绝，且未认证调用被拒绝。
func TestUserPreferencesMergeAndValidate(t *testing.T) {
	svc, repo := newAccountService(t, true, 0)
	ctx := context.Background()
	newCustomer(t, repo, "u1")
	principal := &auth.Principal{UserID: "u1", Username: "u1", Role: auth.RoleUser}

	prefs, err := svc.Preferences(ctx, principal)
	if err != nil {
		t.Fatalf("Preferences: %v", err)
	}
	if len(prefs) != 0 {
		t.Fatalf("initial preferences = %+v, want empty", prefs)
	}

	updated, err := svc.UpdatePreferences(ctx, principal, map[string]any{
		"viewer_mode":   "fit",
		"plaza_layout":  "masonry",
		"images_layout": "masonry",
		"unknown":       "ignored",
	})
	if err != nil {
		t.Fatalf("UpdatePreferences: %v", err)
	}
	if updated["viewer_mode"] != "fit" {
		t.Fatalf("viewer_mode = %v, want fit", updated["viewer_mode"])
	}
	if updated["plaza_layout"] != "masonry" {
		t.Fatalf("plaza_layout = %v, want masonry", updated["plaza_layout"])
	}
	if updated["images_layout"] != "masonry" {
		t.Fatalf("images_layout = %v, want masonry", updated["images_layout"])
	}
	if _, ok := updated["unknown"]; ok {
		t.Fatalf("unknown key should be ignored: %+v", updated)
	}

	again, err := svc.Preferences(ctx, principal)
	if err != nil {
		t.Fatalf("Preferences reload: %v", err)
	}
	if again["viewer_mode"] != "fit" {
		t.Fatalf("persisted viewer_mode = %v, want fit", again["viewer_mode"])
	}
	if again["plaza_layout"] != "masonry" {
		t.Fatalf("persisted plaza_layout = %v, want masonry", again["plaza_layout"])
	}
	if again["images_layout"] != "masonry" {
		t.Fatalf("persisted images_layout = %v, want masonry", again["images_layout"])
	}

	if _, err := svc.UpdatePreferences(ctx, principal, map[string]any{"viewer_mode": "bogus"}); !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("invalid viewer_mode error = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.UpdatePreferences(ctx, principal, map[string]any{"plaza_layout": "bogus"}); !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("invalid plaza_layout error = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.UpdatePreferences(ctx, principal, map[string]any{"images_layout": "bogus"}); !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("invalid images_layout error = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.UpdatePreferences(ctx, nil, map[string]any{"viewer_mode": "fit"}); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("unauthenticated error = %v, want ErrForbidden", err)
	}
}

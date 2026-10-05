package api_test

import (
	"context"
	"net/http"
	"testing"
)

// TestChangePasswordEndpoint 验证已登录用户可通过接口修改密码，且旧密码
// 立即失效。
func TestChangePasswordEndpoint(t *testing.T) {
	env := newTestEnv(t, true, 1<<20)
	user, err := env.accounts.RegisterCustomer(context.Background(), "pwapi", "password123")
	if err != nil {
		t.Fatalf("RegisterCustomer: %v", err)
	}
	token, err := env.accounts.CreateToken(context.Background(), user.ID, "test")
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	// 当前密码错误。
	status, body := do(t, env.router, http.MethodPost, "/api/v1/auth/password",
		`{"current_password":"wrong-password","new_password":"newpassword123"}`, token.Token)
	if status != http.StatusBadRequest {
		t.Fatalf("wrong current status = %d (%s), want 400", status, body)
	}

	// 正确修改。
	status, body = do(t, env.router, http.MethodPost, "/api/v1/auth/password",
		`{"current_password":"password123","new_password":"newpassword123"}`, token.Token)
	if status != http.StatusOK {
		t.Fatalf("change status = %d (%s), want 200", status, body)
	}

	// 旧密码登录失败，新密码登录成功。
	if status, _ = do(t, env.router, http.MethodPost, "/api/v1/auth/login",
		`{"username":"pwapi","password":"password123"}`, ""); status != http.StatusUnauthorized {
		t.Fatalf("old login status = %d, want 401", status)
	}
	if status, body = do(t, env.router, http.MethodPost, "/api/v1/auth/login",
		`{"username":"pwapi","password":"newpassword123"}`, ""); status != http.StatusOK {
		t.Fatalf("new login status = %d (%s), want 200", status, body)
	}

	// 未登录调用改密接口应被拒绝。
	if status, _ = do(t, env.router, http.MethodPost, "/api/v1/auth/password",
		`{"current_password":"password123","new_password":"whatever123"}`, ""); status != http.StatusUnauthorized {
		t.Fatalf("anonymous change status = %d, want 401", status)
	}
}

// TestPasswordResetRequiresEmailChannel 验证未配置邮件渠道时，找回密码接口
// 返回明确错误而非静默成功。
func TestPasswordResetRequiresEmailChannel(t *testing.T) {
	env := newTestEnv(t, true, 1<<20)
	status, body := do(t, env.router, http.MethodPost, "/api/v1/auth/password/reset/code",
		`{"email":"someone@example.com"}`, "")
	if status != http.StatusBadRequest {
		t.Fatalf("reset code status = %d (%s), want 400", status, body)
	}
}

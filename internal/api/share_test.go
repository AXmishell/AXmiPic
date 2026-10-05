package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// TestShareAccessRateLimited 验证分享访问（密码校验）接口按 IP 限流，从而
// 阻止对分享密码的暴力破解。
func TestShareAccessRateLimited(t *testing.T) {
	env, _ := newPolicyTestEnv(t)

	user, err := env.accounts.RegisterCustomer(context.Background(), "sharer", "password123")
	if err != nil {
		t.Fatalf("RegisterCustomer: %v", err)
	}
	token, err := env.accounts.CreateToken(context.Background(), user.ID, "test")
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	status, body := do(t, env.router, http.MethodPost, "/api/v1/albums", `{"name":"a"}`, token.Token)
	if status != http.StatusCreated {
		t.Fatalf("create album status = %d (%s)", status, body)
	}
	var album struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &album); err != nil || album.Data.ID == "" {
		t.Fatalf("bad album response: %s", body)
	}

	status, body = do(t, env.router, http.MethodPost, "/api/v1/shares",
		`{"target_type":"album","target_id":"`+album.Data.ID+`","password":"secret"}`, token.Token)
	if status != http.StatusCreated {
		t.Fatalf("create share status = %d (%s)", status, body)
	}
	var share struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &share); err != nil || share.Data.Token == "" {
		t.Fatalf("bad share response: %s", body)
	}

	accessPath := "/api/v1/shares/" + share.Data.Token + "/access"
	// 首次访问（突发额度为 1）应放行。
	if status, body = do(t, env.router, http.MethodPost, accessPath, `{"password":"secret"}`, ""); status != http.StatusOK {
		t.Fatalf("first access status = %d (%s), want 200", status, body)
	}
	// 立即再次访问应被限流。
	if status, body = do(t, env.router, http.MethodPost, accessPath, `{"password":"secret"}`, ""); status != http.StatusTooManyRequests {
		t.Fatalf("second access status = %d (%s), want 429", status, body)
	}
}

package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestAdminUpdateAdminProfile(t *testing.T) {
	env := newTestEnv(t, true, 1<<20)
	adminToken, adminID := env.adminToken(t, "root")
	ctx := context.Background()

	body := `{"username":"RootRenamed","email":"root@example.com"}`
	status, resp := do(t, env.router, http.MethodPatch,
		"/api/v1/admin/admins/"+adminID, body, adminToken)
	if status != http.StatusOK {
		t.Fatalf("update admin status = %d (%s)", status, resp)
	}
	if !strings.Contains(string(resp), `"username":"RootRenamed"`) {
		t.Fatalf("unexpected response: %s", resp)
	}
	if _, err := env.accounts.LoginAdmin(ctx, "rootrenamed", "password123"); err != nil {
		t.Fatalf("login with updated admin username: %v", err)
	}
}

func TestAdminUpdateCustomerProfile(t *testing.T) {
	env := newTestEnv(t, true, 1<<20)
	adminToken, _ := env.adminToken(t, "root")
	ctx := context.Background()

	user, err := env.accounts.RegisterCustomer(ctx, "original", "password123")
	if err != nil {
		t.Fatalf("RegisterCustomer: %v", err)
	}
	other, err := env.accounts.RegisterCustomer(ctx, "Taken", "password123")
	if err != nil {
		t.Fatalf("RegisterCustomer other: %v", err)
	}
	if other.ID == user.ID {
		t.Fatal("unexpected duplicate id")
	}

	body := `{"username":"RenamedUser","email":"renamed@example.com","password":"newpassword123"}`
	status, resp := do(t, env.router, http.MethodPatch,
		"/api/v1/admin/customers/"+user.ID, body, adminToken)
	if status != http.StatusOK {
		t.Fatalf("update status = %d (%s)", status, resp)
	}
	if !strings.Contains(string(resp), `"username":"RenamedUser"`) ||
		!strings.Contains(string(resp), `"email":"renamed@example.com"`) {
		t.Fatalf("unexpected response: %s", resp)
	}
	if _, err := env.accounts.LoginCustomer(ctx, "renameduser", "newpassword123"); err != nil {
		t.Fatalf("login with updated profile: %v", err)
	}

	// 同角色用户名唯一（大小写不敏感）。
	body = `{"username":"taken"}`
	status, resp = do(t, env.router, http.MethodPatch,
		"/api/v1/admin/customers/"+user.ID, body, adminToken)
	if status != http.StatusConflict {
		t.Fatalf("duplicate username status = %d (%s)", status, resp)
	}
}

// TestAdminCustomersPaginationAndFilters 验证普通用户列表的分页响应、关键字
// 过滤与排序；不带分页参数时仍返回旧数组形状以兼容 SDK。
func TestAdminCustomersPaginationAndFilters(t *testing.T) {
	env := newTestEnv(t, true, 1<<20)
	adminToken, _ := env.adminToken(t, "root")
	ctx := context.Background()

	for i := 1; i <= 5; i++ {
		if _, err := env.accounts.RegisterCustomer(ctx, fmt.Sprintf("pageuser%d", i), "password123"); err != nil {
			t.Fatalf("RegisterCustomer: %v", err)
		}
	}

	type pageBody struct {
		Data struct {
			Items []struct {
				Username string `json:"username"`
			} `json:"items"`
			Total    int64 `json:"total"`
			Page     int   `json:"page"`
			PageSize int   `json:"page_size"`
		} `json:"data"`
	}

	// 分页 + 用户名排序。
	status, body := do(t, env.router, http.MethodGet,
		"/api/v1/admin/customers?page=1&page_size=2&sort=username", "", adminToken)
	if status != http.StatusOK {
		t.Fatalf("status = %d (%s)", status, body)
	}
	var page pageBody
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if page.Data.Total != 5 || page.Data.Page != 1 || page.Data.PageSize != 2 {
		t.Fatalf("pagination meta = %+v", page.Data)
	}
	if len(page.Data.Items) != 2 || page.Data.Items[0].Username != "pageuser1" {
		t.Fatalf("unexpected items: %+v", page.Data.Items)
	}

	// 第二页。
	status, body = do(t, env.router, http.MethodGet,
		"/api/v1/admin/customers?page=3&page_size=2&sort=username", "", adminToken)
	if status != http.StatusOK {
		t.Fatalf("page 3 status = %d (%s)", status, body)
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("decode page 3: %v", err)
	}
	if len(page.Data.Items) != 1 || page.Data.Items[0].Username != "pageuser5" {
		t.Fatalf("unexpected page 3 items: %+v", page.Data.Items)
	}

	// 关键字过滤。
	status, body = do(t, env.router, http.MethodGet,
		"/api/v1/admin/customers?page=1&page_size=20&keyword=pageuser4", "", adminToken)
	if status != http.StatusOK {
		t.Fatalf("keyword status = %d (%s)", status, body)
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("decode keyword: %v", err)
	}
	if page.Data.Total != 1 || len(page.Data.Items) != 1 || page.Data.Items[0].Username != "pageuser4" {
		t.Fatalf("unexpected keyword items: %+v total=%d", page.Data.Items, page.Data.Total)
	}

	// 不带分页参数时保持旧数组形状（SDK 兼容）。
	status, body = do(t, env.router, http.MethodGet, "/api/v1/admin/customers", "", adminToken)
	if status != http.StatusOK {
		t.Fatalf("legacy status = %d (%s)", status, body)
	}
	var legacy struct {
		Data []struct {
			Username string `json:"username"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &legacy); err != nil {
		t.Fatalf("legacy decode: %v (%s)", err, body)
	}
	if len(legacy.Data) != 5 {
		t.Fatalf("legacy length = %d, want 5", len(legacy.Data))
	}
}

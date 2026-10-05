package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestAdminNotifyLogs 验证管理员可查看通知发送日志，普通用户无权访问。
func TestAdminNotifyLogs(t *testing.T) {
	env := newTestEnv(t, true, 1<<20)
	adminToken, _ := env.adminToken(t, "root")

	// 通过测试发送接口产生一条邮件日志。
	status, body := do(t, env.router, http.MethodPost, "/api/v1/admin/notify/test",
		`{"channel":"email","to":"x@example.com","subject":"s","body":"b"}`, adminToken)
	if status != http.StatusOK {
		t.Fatalf("test notify status = %d (%s)", status, body)
	}

	status, body = do(t, env.router, http.MethodGet, "/api/v1/admin/notify/logs?channel=email", "", adminToken)
	if status != http.StatusOK {
		t.Fatalf("logs status = %d (%s)", status, body)
	}
	var list struct {
		Data struct {
			Total int64 `json:"total"`
			Items []struct {
				Channel string `json:"channel"`
				To      string `json:"to"`
				Status  string `json:"status"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &list); err != nil || list.Data.Total != 1 || len(list.Data.Items) != 1 {
		t.Fatalf("logs = %s err=%v", body, err)
	}
	if list.Data.Items[0].To != "x@example.com" || list.Data.Items[0].Status != "sent" {
		t.Fatalf("log item = %+v", list.Data.Items[0])
	}

	// 普通用户无权访问。
	userToken := env.token(t, "plainuser")
	if status, _ := do(t, env.router, http.MethodGet, "/api/v1/admin/notify/logs", "", userToken); status != http.StatusForbidden {
		t.Fatalf("user logs status = %d, want 403", status)
	}
}

package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestUpdateImageAuthorRequiresAdmin(t *testing.T) {
	env := newTestEnv(t, false, 0)
	userToken := env.token(t, "alice")
	adminToken, _ := env.adminToken(t, "root")

	status, key := uploadPNG(t, env.router, testPNG(t, 8), userToken)
	if status != http.StatusOK {
		t.Fatalf("upload status = %d", status)
	}
	image, err := env.repo.GetByKey(context.Background(), key)
	if err != nil {
		t.Fatalf("GetByKey: %v", err)
	}
	id := image.ID

	body := `{"author_name":"げっか ☾ᶻ 𝗓 𐰁","author_handle":"mooNli9ht_4tune","source_url":"https://x.com/mooNli9ht_4tune/status/2112521500301607286"}`
	status, response := do(t, env.router, http.MethodPatch, "/api/v1/images/"+id, body, userToken)
	if status != http.StatusForbidden {
		t.Fatalf("user metadata update status = %d (%s)", status, response)
	}

	status, response = do(t, env.router, http.MethodPatch, "/api/v1/images/"+id, body, adminToken)
	if status != http.StatusOK {
		t.Fatalf("admin metadata update status = %d (%s)", status, response)
	}
	var payload struct {
		Code int `json:"code"`
		Data struct {
			AuthorName   string `json:"author_name"`
			AuthorHandle string `json:"author_handle"`
			AuthorURL    string `json:"author_url"`
			SourceURL    string `json:"source_url"`
			SourcePostID string `json:"source_post_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response, &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Data.AuthorName != "げっか ☾ᶻ 𝗓 𐰁" ||
		payload.Data.AuthorHandle != "mooNli9ht_4tune" ||
		payload.Data.AuthorURL != "https://x.com/mooNli9ht_4tune" {
		t.Fatalf("unexpected author: %+v", payload.Data)
	}
	if payload.Data.SourcePostID != "2112521500301607286" {
		t.Fatalf("source post id = %q", payload.Data.SourcePostID)
	}
}

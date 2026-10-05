package moderation_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AXmishell/axmipic/internal/moderation"
)

func TestReviewBuildsOpenAIRequest(t *testing.T) {
	var gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "SAFE"}}},
		})
	}))
	defer srv.Close()

	m := moderation.NewOpenAI(moderation.OpenAIConfig{
		BaseURL: srv.URL,
		APIKey:  "secret-key",
		Model:   "gpt-4o-mini",
	})
	decision, err := m.Review(context.Background(), []byte("png-bytes"), "image/png")
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("decision = %+v, want allowed", decision)
	}
	if gotAuth != "Bearer secret-key" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotBody["model"] != "gpt-4o-mini" {
		t.Fatalf("model = %v", gotBody["model"])
	}
	// 请求应包含 data URL 图片内容。
	raw, _ := json.Marshal(gotBody)
	if !strings.Contains(string(raw), "data:image/png;base64,") {
		t.Fatalf("request missing image data url: %s", raw)
	}
}

func TestReviewClassifiesVerdicts(t *testing.T) {
	cases := []struct {
		content     string
		wantAllowed bool
	}{
		{"SAFE", true},
		{"safe to publish", true},
		{"安全", true},
		{"UNSAFE", false},
		{"unsafe: nudity detected", false},
		{"该图片包含违规内容", false},
		{"", false},
		{"I am not sure", false},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{"message": map[string]any{"content": tc.content}}},
			})
		}))
		m := moderation.NewOpenAI(moderation.OpenAIConfig{BaseURL: srv.URL})
		decision, err := m.Review(context.Background(), []byte("x"), "image/png")
		srv.Close()
		if err != nil {
			t.Fatalf("content %q: %v", tc.content, err)
		}
		if decision.Allowed != tc.wantAllowed {
			t.Fatalf("content %q allowed = %v, want %v", tc.content, decision.Allowed, tc.wantAllowed)
		}
	}
}

func TestReviewReportsHTTPErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	m := moderation.NewOpenAI(moderation.OpenAIConfig{BaseURL: srv.URL})
	if _, err := m.Review(context.Background(), []byte("x"), "image/png"); err == nil {
		t.Fatal("expected an error for a non-200 response")
	}
}

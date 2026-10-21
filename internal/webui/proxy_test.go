package webui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewDevProxyForwardsFrontendRequests(t *testing.T) {
	var gotHost, gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost, gotPath = r.Host, r.URL.Path
		_, _ = io.WriteString(w, "vite")
	}))
	defer upstream.Close()

	handler, err := NewDevProxy(upstream.URL)
	if err != nil {
		t.Fatalf("NewDevProxy: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/@vite/client", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || rec.Body.String() != "vite" {
		t.Fatalf("unexpected response: %d %q", rec.Code, rec.Body.String())
	}
	if gotPath != "/@vite/client" {
		t.Fatalf("path = %q, want /@vite/client", gotPath)
	}
	if gotHost == "" || gotHost == req.Host {
		t.Fatalf("host should be rewritten to the target, got %q", gotHost)
	}
}

func TestNewDevProxyKeepsBackendPaths(t *testing.T) {
	forwarded := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded = true
	}))
	defer upstream.Close()

	handler, err := NewDevProxy(upstream.URL)
	if err != nil {
		t.Fatalf("NewDevProxy: %v", err)
	}

	for _, path := range []string{"/api/v1/unknown", "/i/missing", "/healthz"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d, want 404", path, rec.Code)
		}
	}
	if forwarded {
		t.Fatal("backend paths must not be proxied to Vite")
	}
}

func TestNewDevProxyRejectsInvalidTarget(t *testing.T) {
	for _, target := range []string{"", "127.0.0.1:5173", "ftp://127.0.0.1:5173"} {
		if _, err := NewDevProxy(target); err == nil {
			t.Fatalf("expected an error for target %q", target)
		}
	}
}

package axmipic

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestServer 启动一个模拟 AXmiPic 信封响应的服务端。
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("login should not send Authorization")
		}
		writeEnvelope(w, 0, map[string]any{
			"token":      "session-token",
			"expires_at": "2099-01-01T00:00:00Z",
			"user":       map[string]any{"id": "u1", "username": "alice", "role": "customer"},
		})
	})
	mux.HandleFunc("/api/v1/auth/me", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer session-token" {
			writeEnvelopeStatus(w, http.StatusUnauthorized, http.StatusUnauthorized, "authentication required", nil)
			return
		}
		writeEnvelope(w, 0, map[string]any{"id": "u1", "username": "alice", "role": "customer"})
	})
	mux.HandleFunc("/api/v1/images", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("permission"); got != "public" {
			t.Errorf("permission query = %q", got)
		}
		writeEnvelope(w, 0, map[string]any{
			"items":     []map[string]any{{"id": "i1", "key": "k1", "url": "http://x/i/k1", "permission": "public"}},
			"total":     1,
			"page":      1,
			"page_size": 20,
		})
	})
	mux.HandleFunc("/api/v1/images/i1", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodDelete:
			writeEnvelope(w, 0, map[string]string{"id": "i1"})
		default:
			writeEnvelope(w, 0, map[string]any{"id": "i1", "key": "k1", "permission": "private"})
		}
	})
	mux.HandleFunc("/api/v1/upload", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
		}
		if _, _, err := r.FormFile("file"); err != nil {
			t.Errorf("missing file field: %v", err)
		}
		writeEnvelope(w, 0, map[string]any{"id": "i2", "key": "k2", "url": "http://x/i/k2", "size": 5})
	})
	mux.HandleFunc("/api/v1/admin/stats", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(w, 0, map[string]any{"admins": 1, "customers": 2, "images": 3, "processor": "purego"})
	})
	mux.HandleFunc("/api/v1/error", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelopeStatus(w, http.StatusNotFound, http.StatusNotFound, "image not found", nil)
	})
	return httptest.NewServer(mux)
}

func writeEnvelope(w http.ResponseWriter, code int, data any) {
	writeEnvelopeStatus(w, http.StatusOK, code, "ok", data)
}

func writeEnvelopeStatus(w http.ResponseWriter, status, code int, message string, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "message": message, "data": data})
}

func TestLoginSetsTokenAndMe(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()
	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	session, err := client.Login(context.Background(), "alice", "password123")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if session.Token != "session-token" || client.Token() != "session-token" {
		t.Fatalf("token not stored: %+v", session)
	}
	me, err := client.Me(context.Background())
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	if me.Username != "alice" {
		t.Fatalf("me = %+v", me)
	}
}

func TestListImagesQuery(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()
	client, _ := New(server.URL)
	list, err := client.ListImages(context.Background(), ImageQuery{Page: 1, PageSize: 20, Permission: "public"})
	if err != nil {
		t.Fatalf("ListImages: %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].ID != "i1" {
		t.Fatalf("list = %+v", list)
	}
}

func TestUploadMultipart(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()
	client, _ := New(server.URL)
	img, err := client.Upload(context.Background(), "photo.png", []byte("hello"))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if img.ID != "i2" || img.Size != 5 {
		t.Fatalf("image = %+v", img)
	}
}

func TestDeleteImage(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()
	client, _ := New(server.URL)
	if err := client.DeleteImage(context.Background(), "i1"); err != nil {
		t.Fatalf("DeleteImage: %v", err)
	}
}

func TestErrorMapping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeEnvelopeStatus(w, http.StatusBadRequest, http.StatusBadRequest, "invalid input", nil)
	}))
	defer server.Close()
	client, _ := New(server.URL)
	if _, err := client.CreateAlbum(context.Background(), AlbumInput{Name: ""}); err == nil {
		t.Fatal("expected error")
	}
}

func TestErrorType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeEnvelopeStatus(w, http.StatusNotFound, http.StatusNotFound, "image not found", nil)
	}))
	defer server.Close()
	client, _ := New(server.URL)
	err := client.DeleteImage(context.Background(), "missing")
	var apiErr *Error
	if err == nil || !asError(err, &apiErr) {
		t.Fatalf("err = %v, want *Error", err)
	}
	if !apiErr.IsNotFound() || apiErr.StatusCode != 404 {
		t.Fatalf("apiErr = %+v", apiErr)
	}
}

func TestTransformURL(t *testing.T) {
	p := TransformParams{Width: 400, Height: 300, Fit: "cover", Format: "webp", Quality: 80, Watermark: "AXmiPic", WatermarkPosition: "center"}
	got := p.TransformURL("http://x/i/k.png")
	for _, want := range []string{"w=400", "h=300", "fit=cover", "f=webp", "q=80", "wm=AXmiPic", "wm_pos=center"} {
		if !strings.Contains(got, want) {
			t.Fatalf("url %q missing %q", got, want)
		}
	}
}

func TestAdminStats(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()
	client, _ := New(server.URL)
	stats, err := client.AdminStats(context.Background())
	if err != nil {
		t.Fatalf("AdminStats: %v", err)
	}
	if stats.Admins != 1 || stats.Processor != "purego" {
		t.Fatalf("stats = %+v", stats)
	}
}

// asError 使用标准库 errors.As 把 err 提取为 *Error。
func asError(err error, target **Error) bool {
	return errors.As(err, target)
}

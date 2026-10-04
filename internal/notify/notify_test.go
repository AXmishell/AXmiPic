package notify_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AXmishell/axmipic/internal/notify"
)

func TestMockSender(t *testing.T) {
	sender := notify.NewMockSender("sms")
	if err := sender.Send(context.Background(), notify.Message{To: "13800000000", Body: "hi"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(sender.Sent) != 1 || sender.Sent[0].To != "13800000000" {
		t.Fatalf("sent = %+v", sender.Sent)
	}
}

func TestHTTPSSenderPostsParams(t *testing.T) {
	var gotTo, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotTo = r.Form.Get("phone")
		gotBody = r.Form.Get("text")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer server.Close()

	sender, err := notify.NewHTTPSSender(notify.HTTPOptions{
		Name:     "test-sms",
		Endpoint: server.URL,
		Method:   "POST",
		Params: map[string]string{
			"phone": "{{to}}",
			"text":  "{{body}}",
		},
	})
	if err != nil {
		t.Fatalf("NewHTTPSSender: %v", err)
	}
	if sender.Name() != "test-sms" {
		t.Fatalf("name = %q", sender.Name())
	}
	if err := sender.Send(context.Background(), notify.Message{To: "13900000000", Body: "验证码 1234"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotTo != "13900000000" || gotBody != "验证码 1234" {
		t.Fatalf("server received to=%q body=%q", gotTo, gotBody)
	}
}

func TestHTTPSSenderRejectsFailureCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 400, "msg": "bad"})
	}))
	defer server.Close()

	sender, _ := notify.NewHTTPSSender(notify.HTTPOptions{Endpoint: server.URL})
	if err := sender.Send(context.Background(), notify.Message{To: "1", Body: "x"}); err == nil {
		t.Fatal("expected error for failure code")
	}
}

func TestValidateMessage(t *testing.T) {
	if err := notify.ValidateMessage(notify.Message{Body: "x"}); err == nil {
		t.Fatal("expected error for missing recipient")
	}
	if err := notify.ValidateMessage(notify.Message{To: "a@b.c"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

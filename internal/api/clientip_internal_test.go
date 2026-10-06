package api

import (
	"net/http/httptest"
	"testing"

	"github.com/AXmishell/axmipic/internal/config"
)

func resolveIP(t *testing.T, cfg config.ClientIPConfig, remoteAddr string, headers map[string]string) string {
	t.Helper()
	r := NewClientIPResolver(cfg, false, nil)
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = remoteAddr
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return r.resolve(req)
}

func TestClientIPRemoteIgnoresHeaders(t *testing.T) {
	got := resolveIP(t, config.ClientIPConfig{Source: "remote"}, "203.0.113.9:1234", map[string]string{
		"X-Forwarded-For": "1.2.3.4",
		"X-Real-IP":       "5.6.7.8",
	})
	if got != "203.0.113.9" {
		t.Fatalf("remote source = %q, want peer", got)
	}
}

func TestClientIPXFFRightmostUntrusted(t *testing.T) {
	cfg := config.ClientIPConfig{Source: "x-forwarded-for", TrustedProxies: []string{"10.0.0.0/8"}}
	got := resolveIP(t, cfg, "10.0.0.5:1234", map[string]string{
		"X-Forwarded-For": "1.2.3.4, 10.0.0.7",
	})
	if got != "1.2.3.4" {
		t.Fatalf("xff rightmost-untrusted = %q, want 1.2.3.4", got)
	}
}

func TestClientIPXFFDepth(t *testing.T) {
	cfg := config.ClientIPConfig{Source: "x-forwarded-for", TrustedProxies: []string{"10.0.0.0/8"}, XFFDepth: 1}
	got := resolveIP(t, cfg, "10.0.0.5:1234", map[string]string{
		"X-Forwarded-For": "1.2.3.4, 10.0.0.7, 10.0.0.8",
	})
	if got != "10.0.0.7" {
		t.Fatalf("xff depth=1 = %q, want 10.0.0.7", got)
	}
}

func TestClientIPUntrustedPeerFallsBack(t *testing.T) {
	cfg := config.ClientIPConfig{Source: "x-forwarded-for", TrustedProxies: []string{"10.0.0.0/8"}}
	got := resolveIP(t, cfg, "203.0.113.9:1234", map[string]string{
		"X-Forwarded-For": "1.2.3.4",
	})
	if got != "203.0.113.9" {
		t.Fatalf("untrusted peer = %q, want peer", got)
	}
}

func TestClientIPNoTrustedProxiesFallsBackToRemote(t *testing.T) {
	r := NewClientIPResolver(config.ClientIPConfig{Source: "cf-connecting-ip"}, false, nil)
	if r.source != "remote" {
		t.Fatalf("source = %q, want remote fallback", r.source)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "203.0.113.9:1234"
	req.Header.Set("CF-Connecting-IP", "1.2.3.4")
	if got := r.resolve(req); got != "203.0.113.9" {
		t.Fatalf("fallback resolve = %q, want peer", got)
	}
}

func TestClientIPCloudflareHeader(t *testing.T) {
	cfg := config.ClientIPConfig{Source: "cf-connecting-ip", TrustedProxies: []string{"10.0.0.0/8"}}
	got := resolveIP(t, cfg, "10.0.0.5:1234", map[string]string{"CF-Connecting-IP": "1.2.3.4"})
	if got != "1.2.3.4" {
		t.Fatalf("cf-connecting-ip = %q, want 1.2.3.4", got)
	}
}

func TestClientIPForwardedRFC7239(t *testing.T) {
	cfg := config.ClientIPConfig{Source: "forwarded", TrustedProxies: []string{"10.0.0.0/8"}}
	got := resolveIP(t, cfg, "10.0.0.5:1234", map[string]string{
		"Forwarded": `for=1.2.3.4;proto=https, for="10.0.0.7"`,
	})
	if got != "1.2.3.4" {
		t.Fatalf("forwarded = %q, want 1.2.3.4", got)
	}
}

func TestClientIPCustomHeaderAndInvalid(t *testing.T) {
	cfg := config.ClientIPConfig{Source: "custom", Header: "X-Real-Client", TrustedProxies: []string{"10.0.0.0/8"}}
	if got := resolveIP(t, cfg, "10.0.0.5:1234", map[string]string{"X-Real-Client": "1.2.3.4:5678"}); got != "1.2.3.4" {
		t.Fatalf("custom header = %q, want 1.2.3.4", got)
	}
	// 非法头值回退对端。
	if got := resolveIP(t, cfg, "10.0.0.5:1234", map[string]string{"X-Real-Client": "not-an-ip"}); got != "10.0.0.5" {
		t.Fatalf("invalid header = %q, want peer", got)
	}
}

func TestNormalizeIP(t *testing.T) {
	cases := map[string]string{
		"1.2.3.4":          "1.2.3.4",
		"1.2.3.4:5678":     "1.2.3.4",
		"[2001:db8::1]:80": "2001:db8::1",
		"[2001:db8::1]":    "2001:db8::1",
		"fe80::1%eth0":     "fe80::1",
		"garbage":          "",
	}
	for in, want := range cases {
		if got := normalizeIP(in); got != want {
			t.Fatalf("normalizeIP(%q) = %q, want %q", in, got, want)
		}
	}
}

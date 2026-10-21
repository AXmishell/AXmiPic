package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestNormalizeResolvedPostFiltersMediaAndCanonicalizes(t *testing.T) {
	ref := XStatusRef{
		CanonicalURL: "https://x.com/i/status/2112521500301607286",
		PostID:       "2112521500301607286",
	}
	post, err := normalizeResolvedPost(&XResolvedPost{
		Platform: "x",
		PostID:   ref.PostID,
		Author:   XResolvedAuthor{Name: " げっか ", Handle: "@mooNli9ht_4tune"},
		Media: []XResolvedMedia{
			{ID: "1", Type: "photo", URL: "https://pbs.twimg.com/media/a.jpg?name=orig"},
			{ID: "2", Type: "video", URL: "https://video.twimg.com/a.mp4"},
			{ID: "3", Type: "photo", URL: "http://pbs.twimg.com/media/b.jpg"},
			{ID: "4", Type: "photo", URL: "https://evil.example/c.jpg"},
		},
	}, ref)
	if err != nil {
		t.Fatalf("normalizeResolvedPost: %v", err)
	}
	if post.Author.Handle != "mooNli9ht_4tune" || post.Author.Name != "げっか" {
		t.Fatalf("author = %+v", post.Author)
	}
	if post.SourceURL != "https://x.com/mooNli9ht_4tune/status/2112521500301607286" {
		t.Fatalf("source = %q", post.SourceURL)
	}
	if len(post.Media) != 1 || post.Media[0].ID != "1" {
		t.Fatalf("media = %+v", post.Media)
	}
}

func TestValidXMediaURL(t *testing.T) {
	for _, raw := range []string{
		"https://pbs.twimg.com/media/a.jpg?name=orig",
		"https://foo.twimg.com/media/a.jpg",
	} {
		if !validXMediaURL(raw) {
			t.Fatalf("%q should be allowed", raw)
		}
	}
	for _, raw := range []string{
		"http://pbs.twimg.com/media/a.jpg",
		"https://pbs.twimg.com.evil.example/a.jpg",
		"https://evil.example/a.jpg",
		"https://user@pbs.twimg.com/a.jpg",
		"https://pbs.twimg.com:8443/a.jpg",
	} {
		if validXMediaURL(raw) {
			t.Fatalf("%q should be rejected", raw)
		}
	}
}

func TestSelectXMedia(t *testing.T) {
	media := []XResolvedMedia{{ID: "1"}, {ID: "2"}}
	selected, err := selectXMedia(media, []string{"2"})
	if err != nil {
		t.Fatalf("selectXMedia: %v", err)
	}
	if len(selected) != 1 || selected[0].ID != "2" {
		t.Fatalf("selected = %+v", selected)
	}
	if _, err := selectXMedia(media, []string{"3"}); err == nil {
		t.Fatal("expected an error for unknown media id")
	}
}

func TestProxyFromConfig(t *testing.T) {
	proxy, err := proxyFromConfig(map[string]string{})
	if err != nil || proxy != nil {
		t.Fatalf("empty proxy = %v, %v", proxy, err)
	}

	proxy, err = proxyFromConfig(map[string]string{"proxy_url": "127.0.0.1:7890"})
	if err != nil {
		t.Fatalf("proxyFromConfig: %v", err)
	}
	if proxy.String() != "http://127.0.0.1:7890" {
		t.Fatalf("proxy = %q", proxy.String())
	}

	proxy, err = proxyFromConfig(map[string]string{
		"proxy_url":      "socks5://127.0.0.1:1080",
		"proxy_username": "user",
		"proxy_password": "pass",
	})
	if err != nil {
		t.Fatalf("proxyFromConfig: %v", err)
	}
	if proxy.Scheme != "socks5" || proxy.Host != "127.0.0.1:1080" {
		t.Fatalf("proxy = %q", proxy.String())
	}
	if user := proxy.User.Username(); user != "user" {
		t.Fatalf("proxy username = %q", user)
	}
	if pass, _ := proxy.User.Password(); pass != "pass" {
		t.Fatalf("proxy password = %q", pass)
	}

	if proxy, err := proxyFromConfig(map[string]string{"proxy_username": "user"}); err != nil || proxy != nil {
		t.Fatalf("orphan credentials should be ignored: %v, %v", proxy, err)
	}
	if _, err := proxyFromConfig(map[string]string{"proxy_url": "ftp://127.0.0.1:21"}); err == nil {
		t.Fatal("expected an error for an unsupported proxy scheme")
	}
}

func TestResolverUsesConfiguredProxy(t *testing.T) {
	p := newBuiltinXResolver()
	if err := p.Configure(context.Background(), map[string]string{
		"provider":  "fxtwitter",
		"proxy_url": "http://127.0.0.1:7890",
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	transport, ok := p.client.Transport.(*http.Transport)
	if !ok || transport.Proxy == nil {
		t.Fatal("expected a proxy-enabled transport")
	}
	proxy, err := transport.Proxy(&http.Request{URL: &url.URL{Scheme: "https", Host: "api.fxtwitter.com"}})
	if err != nil {
		t.Fatalf("proxy: %v", err)
	}
	if proxy == nil || proxy.String() != "http://127.0.0.1:7890" {
		t.Fatalf("proxy = %v", proxy)
	}
}

func TestWithImageVariant(t *testing.T) {
	got := withImageVariant("https://pbs.twimg.com/media/a.jpg?name=orig", "large")
	if got != "https://pbs.twimg.com/media/a.jpg?name=large" {
		t.Fatalf("got %q", got)
	}
}

func TestFxtwitterResolverMapping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/i/status/2112521500301607286" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
			"code": 200,
			"message": "OK",
			"tweet": {
				"id": "2112521500301607286",
				"text": "",
				"possibly_sensitive": false,
				"author": {"name": "げっか ☾ᶻ 𝗓 𐰁 🐾", "screen_name": "mooNli9ht_4tune", "protected": false},
				"media": {"all": [
					{"id": "2112521494509297664", "type": "photo", "url": "https://pbs.twimg.com/media/HVEvG0yaoAAaeci.jpg?name=orig", "width": 928, "height": 1232}
				]}
			}
		}`))
	}))
	defer server.Close()

	oldBase := fxtwitterAPIBase
	fxtwitterAPIBase = server.URL
	defer func() { fxtwitterAPIBase = oldBase }()

	p := newBuiltinXResolver()
	ref := XStatusRef{PostID: "2112521500301607286", Handle: "mooNli9ht_4tune"}
	post, err := p.resolveFxtwitter(context.Background(), server.Client(), "orig", ref)
	if err != nil {
		t.Fatalf("resolveFxtwitter: %v", err)
	}
	if post.Author.Handle != "mooNli9ht_4tune" || post.Author.Name != "げっか ☾ᶻ 𝗓 𐰁 🐾" {
		t.Fatalf("author = %+v", post.Author)
	}
	if len(post.Media) != 1 || post.Media[0].Width != 928 || post.Media[0].Height != 1232 {
		t.Fatalf("media = %+v", post.Media)
	}
	if post.SourceURL != "https://x.com/mooNli9ht_4tune/status/2112521500301607286" {
		t.Fatalf("source = %q", post.SourceURL)
	}
}

func TestOfficialResolverMapping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("authorization = %q", got)
		}
		_, _ = w.Write([]byte(`{
			"data": {
				"id": "2112521500301607286",
				"text": "",
				"author_id": "865496286325227520",
				"possibly_sensitive": false
			},
			"includes": {
				"users": [{"id": "865496286325227520", "name": "げっか ☾ᶻ 𝗓 𐰁 🐾", "username": "mooNli9ht_4tune"}],
				"media": [{"media_key": "3_1", "type": "photo", "url": "https://pbs.twimg.com/media/HVEvG0yaoAAaeci.jpg", "width": 928, "height": 1232}]
			}
		}`))
	}))
	defer server.Close()

	oldBase := officialAPIBase
	officialAPIBase = server.URL
	defer func() { officialAPIBase = oldBase }()

	p := newBuiltinXResolver()
	ref := XStatusRef{PostID: "2112521500301607286"}
	post, err := p.resolveOfficial(context.Background(), server.Client(), "test-token", "large", ref)
	if err != nil {
		t.Fatalf("resolveOfficial: %v", err)
	}
	if post.Author.Handle != "mooNli9ht_4tune" {
		t.Fatalf("author = %+v", post.Author)
	}
	if len(post.Media) != 1 || post.Media[0].ID != "3_1" {
		t.Fatalf("media = %+v", post.Media)
	}
	if post.Media[0].URL != "https://pbs.twimg.com/media/HVEvG0yaoAAaeci.jpg?name=large" {
		t.Fatalf("media url = %q", post.Media[0].URL)
	}
}

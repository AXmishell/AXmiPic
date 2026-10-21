package service_test

import (
	"errors"
	"testing"

	"github.com/AXmishell/axmipic/internal/service"
)

func TestParseXStatusURL(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		wantURL  string
		wantUser string
		wantID   string
	}{
		{
			name:     "missing scheme",
			raw:      "x.com/mooNli9ht_4tune/status/2112521500301607286",
			wantURL:  "https://x.com/mooNli9ht_4tune/status/2112521500301607286",
			wantUser: "mooNli9ht_4tune",
			wantID:   "2112521500301607286",
		},
		{
			name:     "canonical",
			raw:      "https://x.com/mooNli9ht_4tune/status/2112521500301607286",
			wantURL:  "https://x.com/mooNli9ht_4tune/status/2112521500301607286",
			wantUser: "mooNli9ht_4tune",
			wantID:   "2112521500301607286",
		},
		{
			name:     "photo suffix and tracking query",
			raw:      "https://twitter.com/mooNli9ht_4tune/status/2112521500301607286/photo/1?s=20",
			wantURL:  "https://x.com/mooNli9ht_4tune/status/2112521500301607286",
			wantUser: "mooNli9ht_4tune",
			wantID:   "2112521500301607286",
		},
		{
			name:    "i status",
			raw:     "https://x.com/i/status/2112521500301607286",
			wantURL: "https://x.com/i/status/2112521500301607286",
			wantID:  "2112521500301607286",
		},
		{
			name:    "i web status",
			raw:     "https://x.com/i/web/status/2112521500301607286",
			wantURL: "https://x.com/i/status/2112521500301607286",
			wantID:  "2112521500301607286",
		},
		{
			name:     "fxtwitter mirror",
			raw:      "https://fxtwitter.com/mooNli9ht_4tune/status/2112521500301607286",
			wantURL:  "https://x.com/mooNli9ht_4tune/status/2112521500301607286",
			wantUser: "mooNli9ht_4tune",
			wantID:   "2112521500301607286",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ref, err := service.ParseXStatusURL(tc.raw)
			if err != nil {
				t.Fatalf("ParseXStatusURL: %v", err)
			}
			if ref.CanonicalURL != tc.wantURL || ref.Handle != tc.wantUser || ref.PostID != tc.wantID {
				t.Fatalf("got %+v, want url=%q user=%q id=%q", ref, tc.wantURL, tc.wantUser, tc.wantID)
			}
		})
	}
}

func TestParseXStatusURLRejectsNonStatusLinks(t *testing.T) {
	for _, raw := range []string{
		"https://x.com/mooNli9ht_4tune",
		"https://example.com/mooNli9ht_4tune/status/123",
		"ftp://x.com/mooNli9ht_4tune/status/123",
		"https://x.com/mooNli9ht_4tune/status/not-a-number",
	} {
		if _, err := service.ParseXStatusURL(raw); !errors.Is(err, service.ErrInvalidInput) {
			t.Fatalf("%q: expected ErrInvalidInput, got %v", raw, err)
		}
	}
}

func TestNormalizeImageMeta(t *testing.T) {
	meta, err := service.NormalizeImageMeta(service.ImageMeta{
		AuthorName:   "  げっか ☾ᶻ 𝗓 𐰁  ",
		AuthorHandle: "@mooNli9ht_4tune",
		SourceURL:    "https://x.com/mooNli9ht_4tune/status/2112521500301607286/photo/1?s=20",
	})
	if err != nil {
		t.Fatalf("NormalizeImageMeta: %v", err)
	}
	if meta.AuthorName != "げっか ☾ᶻ 𝗓 𐰁" {
		t.Fatalf("author name = %q", meta.AuthorName)
	}
	if meta.AuthorHandle != "mooNli9ht_4tune" || meta.AuthorPlatform != "x" {
		t.Fatalf("handle/platform = %q/%q", meta.AuthorHandle, meta.AuthorPlatform)
	}
	if meta.SourceURL != "https://x.com/mooNli9ht_4tune/status/2112521500301607286" {
		t.Fatalf("source url = %q", meta.SourceURL)
	}
	if meta.SourcePostID != "2112521500301607286" {
		t.Fatalf("source post id = %q", meta.SourcePostID)
	}
	if got := service.AuthorURL(meta.AuthorPlatform, meta.AuthorHandle); got != "https://x.com/mooNli9ht_4tune" {
		t.Fatalf("author url = %q", got)
	}
}

func TestNormalizeImageMetaAllowsEmpty(t *testing.T) {
	meta, err := service.NormalizeImageMeta(service.ImageMeta{})
	if err != nil {
		t.Fatalf("NormalizeImageMeta: %v", err)
	}
	if meta != (service.ImageMeta{}) {
		t.Fatalf("expected empty meta, got %+v", meta)
	}
}

func TestNormalizeImageMetaRejectsInvalidValues(t *testing.T) {
	cases := []service.ImageMeta{
		{AuthorHandle: "bad handle"},
		{AuthorHandle: "toolonghandle123"},
		{AuthorName: string(rune(0x1f))},
		{SourceURL: "javascript:alert(1)"},
		{SourceURL: "https://x.com/user"},
	}
	for _, meta := range cases {
		if _, err := service.NormalizeImageMeta(meta); !errors.Is(err, service.ErrInvalidInput) {
			t.Fatalf("%+v: expected ErrInvalidInput, got %v", meta, err)
		}
	}
}

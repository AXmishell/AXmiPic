package auth_test

import (
	"strings"
	"testing"
	"time"

	"github.com/axmipic/axmipic/internal/auth"
)

func TestPasswordHashAndVerify(t *testing.T) {
	hash, err := auth.HashPassword("s3cret-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !auth.VerifyPassword(hash, "s3cret-password") {
		t.Fatal("correct password rejected")
	}
	if auth.VerifyPassword(hash, "wrong-password") {
		t.Fatal("wrong password accepted")
	}
}

func TestGenerateAPIToken(t *testing.T) {
	plaintext, hash, prefix, err := auth.GenerateAPIToken()
	if err != nil {
		t.Fatalf("GenerateAPIToken: %v", err)
	}
	if !strings.HasPrefix(plaintext, auth.TokenPrefix) {
		t.Fatalf("token %q lacks prefix", plaintext)
	}
	if hash != auth.HashToken(plaintext) {
		t.Fatal("hash does not match token")
	}
	if prefix != plaintext[:8] {
		t.Fatalf("prefix = %q, want %q", prefix, plaintext[:8])
	}
	other, _, _, err := auth.GenerateAPIToken()
	if err != nil {
		t.Fatalf("GenerateAPIToken: %v", err)
	}
	if other == plaintext {
		t.Fatal("two generated tokens are identical")
	}
}

func TestSessionIssuerRoundTrip(t *testing.T) {
	issuer := auth.NewSessionIssuer([]byte("secret"), time.Hour)
	token, expiresAt, err := issuer.Issue("user-1", "admin")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if !expiresAt.After(time.Now()) {
		t.Fatal("expiry is not in the future")
	}
	userID, role, err := issuer.Parse(token)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if userID != "user-1" || role != "admin" {
		t.Fatalf("parsed = %q/%q", userID, role)
	}
}

func TestSessionIssuerRejectsInvalid(t *testing.T) {
	issuer := auth.NewSessionIssuer([]byte("secret"), time.Hour)
	if _, _, err := issuer.Parse("not-a-jwt"); err == nil {
		t.Fatal("garbage token accepted")
	}
	token, _, err := issuer.Issue("user-1", "user")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	other := auth.NewSessionIssuer([]byte("different-secret"), time.Hour)
	if _, _, err := other.Parse(token); err == nil {
		t.Fatal("token accepted under a different secret")
	}
}

func TestRateLimiterBurst(t *testing.T) {
	limiter := auth.NewRateLimiter(60, 2)
	if !limiter.Allow("k") || !limiter.Allow("k") {
		t.Fatal("burst of 2 should be allowed")
	}
	if limiter.Allow("k") {
		t.Fatal("third request should be limited")
	}
	if !limiter.Allow("other") {
		t.Fatal("a different key should not be limited")
	}
}

func TestNilRateLimiterAllowsEverything(t *testing.T) {
	var limiter *auth.RateLimiter
	if !limiter.Allow("k") {
		t.Fatal("nil limiter should allow")
	}
	if auth.NewRateLimiter(0, 0) != nil {
		t.Fatal("zero limits should disable the limiter")
	}
}

package auth_test

import (
	"testing"

	"github.com/AXmishell/axmipic/internal/auth"
)

func TestGuestSignerRoundTrip(t *testing.T) {
	signer := auth.NewGuestSigner([]byte("guest-secret"))
	value := signer.Sign("11111111-2222-3333-4444-555555555555")

	id, ok := signer.Verify(value)
	if !ok || id != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("verify = %q/%v, want the signed id", id, ok)
	}

	// 篡改 ID 或签名都不应通过。
	if _, ok := signer.Verify("x" + value); ok {
		t.Fatal("tampered payload accepted")
	}
	if _, ok := signer.Verify(value + "x"); ok {
		t.Fatal("tampered signature accepted")
	}
	if _, ok := signer.Verify("not-a-valid-value"); ok {
		t.Fatal("malformed value accepted")
	}

	// 不同密钥签发的值不应通过。
	other := auth.NewGuestSigner([]byte("other-secret"))
	if _, ok := other.Verify(value); ok {
		t.Fatal("value signed with another secret accepted")
	}
}

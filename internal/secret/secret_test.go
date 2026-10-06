package secret_test

import (
	"strings"
	"testing"

	"github.com/AXmishell/axmipic/internal/secret"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	c, err := secret.New([]byte("master-key"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	const plain = "super-secret-access-key"
	enc, err := c.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if enc == plain || enc == "" {
		t.Fatalf("ciphertext looks wrong: %q", enc)
	}
	dec, err := c.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if dec != plain {
		t.Fatalf("round trip = %q, want %q", dec, plain)
	}
}

func TestEncryptEmpty(t *testing.T) {
	c, _ := secret.New([]byte("k"))
	enc, err := c.Encrypt("")
	if err != nil || enc != "" {
		t.Fatalf("Encrypt(\"\") = %q, %v", enc, err)
	}
	dec, err := c.Decrypt("")
	if err != nil || dec != "" {
		t.Fatalf("Decrypt(\"\") = %q, %v", dec, err)
	}
}

func TestDecryptRejectsTampered(t *testing.T) {
	c, _ := secret.New([]byte("k"))
	enc, _ := c.Encrypt("value")
	if _, err := c.Decrypt(enc + "x"); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
	if _, err := c.Decrypt("not-base64!!"); err == nil {
		t.Fatal("invalid base64 accepted")
	}
}

func TestDifferentKeysCannotDecrypt(t *testing.T) {
	a, _ := secret.New([]byte("key-a"))
	b, _ := secret.New([]byte("key-b"))
	enc, _ := a.Encrypt("value")
	if _, err := b.Decrypt(enc); err == nil {
		t.Fatal("decrypted with wrong key")
	}
}

func TestCiphertextHasVersionPrefix(t *testing.T) {
	c, _ := secret.New([]byte("master-key-0123456789"))
	enc, _ := c.Encrypt("value")
	if !strings.HasPrefix(enc, "v1:") {
		t.Fatalf("ciphertext missing version prefix: %q", enc)
	}
}

func TestAADRoundTripAndBinding(t *testing.T) {
	c, _ := secret.New([]byte("master-key-0123456789"))
	enc, err := c.EncryptWithAAD("secret", "settings.smtp.password")
	if err != nil {
		t.Fatalf("EncryptWithAAD: %v", err)
	}
	dec, err := c.DecryptWithAAD(enc, "settings.smtp.password")
	if err != nil || dec != "secret" {
		t.Fatalf("AAD round trip = %q, %v", dec, err)
	}
	// 错误 AAD 不应能解开绑定过的密文。
	if _, err := c.DecryptWithAAD(enc, "settings.other"); err == nil {
		t.Fatal("ciphertext decrypted with wrong AAD")
	}
	// 绑定 AAD 的密文也不能用空 AAD 解开。
	if _, err := c.Decrypt(enc); err == nil {
		t.Fatal("AAD-bound ciphertext decrypted without AAD")
	}
}

func TestLegacyCiphertextStillDecrypts(t *testing.T) {
	c, _ := secret.New([]byte("master-key-0123456789"))
	enc, _ := c.Encrypt("legacy-value") // AAD 为空
	// 去掉版本前缀即模拟历史（无前缀、无 AAD）密文。
	legacy := strings.TrimPrefix(enc, "v1:")
	dec, err := c.DecryptWithAAD(legacy, "some.field.aad")
	if err != nil || dec != "legacy-value" {
		t.Fatalf("legacy decrypt = %q, %v", dec, err)
	}
}

func TestWeakKey(t *testing.T) {
	if !secret.WeakKey([]byte("short")) {
		t.Fatal("short key should be reported weak")
	}
	if secret.WeakKey([]byte("0123456789abcdef")) {
		t.Fatal("16-byte key should not be weak")
	}
}

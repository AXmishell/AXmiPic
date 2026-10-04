package secret_test

import (
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

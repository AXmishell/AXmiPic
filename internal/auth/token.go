package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// TokenPrefix marks an AXmiPic API token.
const TokenPrefix = "axp_"

// GenerateAPIToken returns a new token's plaintext, its storage hash, and a
// short display prefix. Only the hash is persisted.
func GenerateAPIToken() (plaintext, hash, prefix string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", "", fmt.Errorf("auth: generate token: %w", err)
	}
	plaintext = TokenPrefix + base64.RawURLEncoding.EncodeToString(buf)
	return plaintext, HashToken(plaintext), plaintext[:8], nil
}

// HashToken returns the hex sha256 of a token, used for lookups.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

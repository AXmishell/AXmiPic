package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// TokenPrefix 是 AXmiPic API 令牌的标识前缀。
const TokenPrefix = "axp_"

// GenerateAPIToken 返回新令牌的明文、用于存储的哈希以及短展示前缀。
// 仅哈希会被持久化。
func GenerateAPIToken() (plaintext, hash, prefix string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", "", fmt.Errorf("auth: generate token: %w", err)
	}
	plaintext = TokenPrefix + base64.RawURLEncoding.EncodeToString(buf)
	return plaintext, HashToken(plaintext), plaintext[:8], nil
}

// HashToken 返回令牌的 sha256 十六进制摘要，用于查询。
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

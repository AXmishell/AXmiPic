package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword 返回 plain 的 bcrypt 哈希值。
func HashPassword(plain string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("auth: hash password: %w", err)
	}
	return string(hashed), nil
}

// VerifyPassword 判断 plain 是否与已存储的 bcrypt 哈希匹配。
func VerifyPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

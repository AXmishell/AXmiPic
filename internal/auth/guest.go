package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

// GuestCookieName 是匿名访客身份 cookie 的名称。
const GuestCookieName = "axmipic_guest"

// GuestSigner 为访客账户 id 生成与校验签名，防止客户端伪造他人的访客身份。
// 值形如 base64url(id) + "." + base64url(hmac(id))。
type GuestSigner struct {
	secret []byte
}

// NewGuestSigner 创建访客签名器。secret 应与会话密钥相同或独立派生。
func NewGuestSigner(secret []byte) *GuestSigner {
	return &GuestSigner{secret: secret}
}

// Sign 为访客账户 id 生成签名 cookie 值。
func (s *GuestSigner) Sign(id string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(id))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString([]byte(id)) + "." + sig
}

// Verify 校验签名 cookie 值并返回其中的访客账户 id。签名不匹配或格式错误时
// 返回 ok=false。
func (s *GuestSigner) Verify(value string) (id string, ok bool) {
	payload, sig, found := strings.Cut(value, ".")
	if !found || payload == "" || sig == "" {
		return "", false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil || len(decoded) == 0 {
		return "", false
	}
	candidate := string(decoded)
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(candidate))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(sig)) {
		return "", false
	}
	return candidate, true
}

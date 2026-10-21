package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AdminEntryCookieName 是管理员安全入口验证后签发的短生命周期 Cookie 名称。
const AdminEntryCookieName = "axmipic_admin_entry"

const adminEntryScope = "admin_entry"

// adminEntryClaims 标记一次通过安全入口的验证，并绑定安全入口版本。
type adminEntryClaims struct {
	Version int    `json:"ver"`
	Scope   string `json:"scope"`
	jwt.RegisteredClaims
}

// AdminEntryIssuer 负责签发与校验管理员安全入口 Cookie。
type AdminEntryIssuer struct {
	secret []byte
	ttl    time.Duration
}

// NewAdminEntryIssuer 创建安全入口 Cookie 签发器。
func NewAdminEntryIssuer(secret []byte, ttl time.Duration) *AdminEntryIssuer {
	return &AdminEntryIssuer{secret: secret, ttl: ttl}
}

// TTL 返回入口 Cookie 的有效期。
func (s *AdminEntryIssuer) TTL() time.Duration { return s.ttl }

// Issue 签发一个绑定当前安全入口版本的入口 Cookie。
func (s *AdminEntryIssuer) Issue(version int) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(s.ttl)
	claims := adminEntryClaims{
		Version: version,
		Scope:   adminEntryScope,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: sign admin entry: %w", err)
	}
	return signed, expiresAt, nil
}

// Parse 校验入口 Cookie 并返回其绑定的安全入口版本。
func (s *AdminEntryIssuer) Parse(tokenString string) (int, error) {
	claims := &adminEntryClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("auth: unexpected signing method %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil || !token.Valid || claims.Scope != adminEntryScope {
		return 0, fmt.Errorf("%w: invalid admin entry cookie", ErrUnauthenticated)
	}
	return claims.Version, nil
}

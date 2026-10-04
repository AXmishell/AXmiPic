package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// SessionIssuer 负责签发与校验会话 JWT。
type SessionIssuer struct {
	secret []byte
	ttl    time.Duration
}

// sessionClaims 携带授权会话所需的数据。
type sessionClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// NewSessionIssuer 创建会话签发器。
func NewSessionIssuer(secret []byte, ttl time.Duration) *SessionIssuer {
	return &SessionIssuer{secret: secret, ttl: ttl}
}

// Issue 返回已签名的会话令牌及其过期时间。
func (s *SessionIssuer) Issue(userID, role string) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(s.ttl)
	claims := sessionClaims{
		Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: sign session: %w", err)
	}
	return signed, expiresAt, nil
}

// Parse 校验会话令牌并返回其主体与角色。
func (s *SessionIssuer) Parse(tokenString string) (userID, role string, err error) {
	claims := &sessionClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("auth: unexpected signing method %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil || !token.Valid {
		return "", "", fmt.Errorf("%w: %v", ErrUnauthenticated, err)
	}
	if claims.Subject == "" {
		return "", "", ErrUnauthenticated
	}
	return claims.Subject, claims.Role, nil
}

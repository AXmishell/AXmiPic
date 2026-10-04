package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// scopeChallenge 标记 TOTP 登录挑战令牌；会话令牌不带该作用域。
const scopeChallenge = "totp"

// challengeTTL 是 TOTP 登录挑战令牌的有效期。
const challengeTTL = 5 * time.Minute

// SessionIssuer 负责签发与校验会话 JWT。
type SessionIssuer struct {
	secret []byte
	ttl    time.Duration
}

// sessionClaims 携带授权会话所需的数据。
type sessionClaims struct {
	Role  string `json:"role"`
	Scope string `json:"scope,omitempty"`
	jwt.RegisteredClaims
}

// NewSessionIssuer 创建会话签发器。
func NewSessionIssuer(secret []byte, ttl time.Duration) *SessionIssuer {
	return &SessionIssuer{secret: secret, ttl: ttl}
}

// Issue 返回已签名的会话令牌及其过期时间。
func (s *SessionIssuer) Issue(userID, role string) (string, time.Time, error) {
	return s.issue(userID, role, "", s.ttl)
}

// IssueChallenge 签发一个用于完成 TOTP 验证的短期挑战令牌。它不能作为会话使用。
func (s *SessionIssuer) IssueChallenge(userID, role string) (string, time.Time, error) {
	ttl := challengeTTL
	if s.ttl > 0 && s.ttl < ttl {
		ttl = s.ttl
	}
	return s.issue(userID, role, scopeChallenge, ttl)
}

func (s *SessionIssuer) issue(userID, role, scope string, ttl time.Duration) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(ttl)
	claims := sessionClaims{
		Role:  role,
		Scope: scope,
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

// Parse 校验会话令牌并返回其主体与角色。TOTP 挑战令牌会被拒绝。
func (s *SessionIssuer) Parse(tokenString string) (userID, role string, err error) {
	claims, err := s.parse(tokenString)
	if err != nil {
		return "", "", err
	}
	if claims.Scope != "" {
		return "", "", fmt.Errorf("%w: token is not a session", ErrUnauthenticated)
	}
	return claims.Subject, claims.Role, nil
}

// ParseChallenge 校验 TOTP 挑战令牌并返回其主体与角色。
func (s *SessionIssuer) ParseChallenge(tokenString string) (userID, role string, err error) {
	claims, err := s.parse(tokenString)
	if err != nil {
		return "", "", err
	}
	if claims.Scope != scopeChallenge {
		return "", "", fmt.Errorf("%w: token is not a totp challenge", ErrUnauthenticated)
	}
	return claims.Subject, claims.Role, nil
}

func (s *SessionIssuer) parse(tokenString string) (*sessionClaims, error) {
	claims := &sessionClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("auth: unexpected signing method %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("%w: %v", ErrUnauthenticated, err)
	}
	if claims.Subject == "" {
		return nil, ErrUnauthenticated
	}
	return claims, nil
}

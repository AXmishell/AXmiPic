package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// SessionIssuer signs and verifies session JWTs.
type SessionIssuer struct {
	secret []byte
	ttl    time.Duration
}

// sessionClaims carries the data needed to authorize a session.
type sessionClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// NewSessionIssuer creates a session issuer.
func NewSessionIssuer(secret []byte, ttl time.Duration) *SessionIssuer {
	return &SessionIssuer{secret: secret, ttl: ttl}
}

// Issue returns a signed session token and its expiry.
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

// Parse verifies a session token and returns its subject and role.
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

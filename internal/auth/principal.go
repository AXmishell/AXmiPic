// Package auth provides authentication primitives: principals, password
// hashing, API tokens, session JWTs, middleware, and rate limiting.
package auth

import (
	"context"
	"errors"
)

// Role is an account's authorization level.
type Role string

// Account roles.
const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

// Errors returned by authentication.
var (
	// ErrUnauthenticated indicates a missing or invalid credential.
	ErrUnauthenticated = errors.New("auth: authentication required")
	// ErrInvalidCredentials indicates a bad username or password.
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
)

// Principal is an authenticated caller. A nil Principal represents an
// anonymous guest.
type Principal struct {
	UserID   string
	Username string
	Role     Role
	Guest    bool
	TokenID  string
}

// IsAdmin reports whether the principal has the admin role.
func (p *Principal) IsAdmin() bool {
	return p != nil && p.Role == RoleAdmin
}

// IsGuest reports whether the principal is anonymous.
func (p *Principal) IsGuest() bool {
	return p == nil || p.Guest
}

type principalKey struct{}

// WithPrincipal stores p in ctx.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// FromContext returns the principal stored in ctx, if any.
func FromContext(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(*Principal)
	return p, ok
}

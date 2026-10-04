// Package auth 提供认证基础设施：主体身份、密码哈希、API 令牌、会话 JWT、
// 中间件与限流。
package auth

import (
	"context"
	"errors"

	"github.com/AXmishell/axmipic/internal/store"
)

// Role 表示账号的授权级别。
type Role string

// 账号角色，与 store 中相互独立的表一一对应。
const (
	RoleUser  Role = "customer"
	RoleAdmin Role = "admin"
)

// 认证过程中返回的错误。
var (
	// ErrUnauthenticated 表示缺少凭证或凭证无效。
	ErrUnauthenticated = errors.New("auth: authentication required")
	// ErrInvalidCredentials 表示用户名或密码错误。
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
)

// Principal 表示已认证的调用方。Principal 为 nil 时代表匿名访客。
type Principal struct {
	UserID   string
	Username string
	Role     Role
	Guest    bool
	TokenID  string
}

// IsAdmin 判断该主体是否拥有管理员角色。
func (p *Principal) IsAdmin() bool {
	return p != nil && p.Role == RoleAdmin
}

// StoreRole 将主体的角色映射为 store 中的账号角色。
func (p *Principal) StoreRole() store.AccountRole {
	if p.IsAdmin() {
		return store.RoleAdmin
	}
	return store.RoleCustomer
}

// IsGuest 判断该主体是否为匿名访客。
func (p *Principal) IsGuest() bool {
	return p == nil || p.Guest
}

type principalKey struct{}

// WithPrincipal 将 p 存入 ctx。
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// FromContext 返回 ctx 中保存的主体身份（若存在）。
func FromContext(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(*Principal)
	return p, ok
}

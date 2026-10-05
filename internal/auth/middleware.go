package auth

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/AXmishell/axmipic/internal/store"
)

// tokenTouchInterval 用于对「最近使用时间」的写入进行节流，避免大量 API 请求
// 导致每个请求都写一次数据库。
const tokenTouchInterval = time.Minute

// Authenticator 将 Bearer 凭证（API 令牌或会话 JWT）解析为 Principal。
// 它不会拒绝匿名请求；如需强制认证，请使用 RequireAuth。
type Authenticator struct {
	repo   *store.Repository
	issuer *SessionIssuer
	// guest 非 nil 时，未携带凭证的匿名请求会被赋予该访客主体，从而使访客上传
	// 计入内置 Guest 账户的存储配额。仅在允许访客上传时设置；可在运行时热替换。
	guest atomic.Pointer[Principal]
}

// NewAuthenticator 创建一个 Authenticator。
func NewAuthenticator(repo *store.Repository, issuer *SessionIssuer) *Authenticator {
	return &Authenticator{repo: repo, issuer: issuer}
}

// SetGuestPrincipal 设置在匿名请求上附加的访客主体（传 nil 以禁用）。它可在
// 运行时安全调用，以支持「允许访客上传」的在线切换。
func (a *Authenticator) SetGuestPrincipal(guest *Principal) {
	if guest == nil {
		a.guest.Store(nil)
		return
	}
	g := *guest
	a.guest.Store(&g)
}

// Authenticate 是可选认证：有效凭证会附加到请求上下文，无效凭证会被拒绝，
// 无凭证则作为访客放行（配置了访客主体时附带 Guest 身份）。
func (a *Authenticator) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		credential := bearerToken(r)
		if credential == "" {
			if guest := a.guest.Load(); guest != nil {
				g := *guest
				next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), &g)))
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		principal, err := a.resolve(r, credential)
		if err != nil {
			writeAuthError(w, http.StatusUnauthorized, "invalid or expired credential")
			return
		}
		next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), principal)))
	})
}

func (a *Authenticator) resolve(r *http.Request, credential string) (*Principal, error) {
	ctx := r.Context()
	if strings.HasPrefix(credential, TokenPrefix) {
		token, err := a.repo.GetTokenByHash(ctx, HashToken(credential))
		if err != nil {
			return nil, ErrUnauthenticated
		}
		if token.ExpiresAt != nil && time.Now().After(*token.ExpiresAt) {
			return nil, ErrUnauthenticated
		}
		if user, err := a.repo.GetAccountByID(ctx, store.RoleCustomer, token.UserID); err == nil {
			if user.Disabled {
				return nil, ErrUnauthenticated
			}
			if token.LastUsedAt == nil || time.Since(*token.LastUsedAt) > tokenTouchInterval {
				if err := a.repo.TouchToken(ctx, token.ID); err != nil {
					// 「最近使用时间」更新失败不应阻塞本身有效的请求。
					_ = err
				}
			}
			return &Principal{
				UserID:   user.ID,
				Username: user.Username,
				Role:     RoleUser,
				TokenID:  token.ID,
			}, nil
		}
		// 管理员存放在独立的表中。
		admin, err := a.repo.GetAccountByID(ctx, store.RoleAdmin, token.UserID)
		if err != nil {
			return nil, ErrUnauthenticated
		}
		if admin.Disabled {
			return nil, ErrUnauthenticated
		}
		if token.LastUsedAt == nil || time.Since(*token.LastUsedAt) > tokenTouchInterval {
			if err := a.repo.TouchToken(ctx, token.ID); err != nil {
				_ = err
			}
		}
		return &Principal{
			UserID:   admin.ID,
			Username: admin.Username,
			Role:     RoleAdmin,
			TokenID:  token.ID,
		}, nil
	}

	userID, role, err := a.issuer.Parse(credential)
	if err != nil {
		return nil, err
	}
	if role == string(RoleAdmin) {
		admin, err := a.repo.GetAccountByID(ctx, store.RoleAdmin, userID)
		if err != nil {
			return nil, ErrUnauthenticated
		}
		if admin.Disabled {
			return nil, ErrUnauthenticated
		}
		return &Principal{UserID: admin.ID, Username: admin.Username, Role: RoleAdmin}, nil
	}
	customer, err := a.repo.GetAccountByID(ctx, store.RoleCustomer, userID)
	if err != nil {
		return nil, ErrUnauthenticated
	}
	if customer.Disabled {
		return nil, ErrUnauthenticated
	}
	return &Principal{UserID: customer.ID, Username: customer.Username, Role: RoleUser}, nil
}

// RequireAdmin 拒绝非管理员已认证用户的请求。
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := FromContext(r.Context())
		if !ok || principal.IsGuest() {
			writeAuthError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		if !principal.IsAdmin() {
			writeAuthError(w, http.StatusForbidden, "admin role required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAuth 拒绝没有已认证（且非访客）主体的请求。
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := FromContext(r.Context())
		if !ok || principal.IsGuest() {
			writeAuthError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

// writeAuthError 输出 API 错误信封。auth 包直接写入，以避免依赖 api 包。
func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":    status,
		"message": message,
		"data":    nil,
	})
}

package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/service"
)

// guestEnsure 为匿名访客建立稳定的、按 cookie 隔离的 Guest 身份：已有有效签名
// cookie 且对应账户存在时直接复用，否则新建一个独立 Guest 账户并下发签名 cookie。
// 仅在开启「允许访客上传」且请求尚未以非访客身份认证时生效。
func (h *Handler) guestEnsure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.accounts == nil || h.guestSigner == nil || !h.currentAuth().AllowGuestUpload {
			next.ServeHTTP(w, r)
			return
		}
		if p, ok := auth.FromContext(r.Context()); ok && !p.IsGuest() {
			next.ServeHTTP(w, r)
			return
		}
		if cookie, err := r.Cookie(auth.GuestCookieName); err == nil {
			if id, valid := h.guestSigner.Verify(cookie.Value); valid && h.accounts.GuestVisitorExists(r.Context(), id) {
				next.ServeHTTP(w, r.WithContext(withGuest(r.Context(), id)))
				return
			}
		}
		id, err := h.accounts.CreateGuestVisitor(r.Context())
		if err != nil {
			h.logger.ErrorContext(r.Context(), "create guest visitor", slog.Any("error", err))
			next.ServeHTTP(w, r)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     auth.GuestCookieName,
			Value:    h.guestSigner.Sign(id),
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   30 * 24 * 60 * 60,
		})
		next.ServeHTTP(w, r.WithContext(withGuest(r.Context(), id)))
	})
}

// withGuest 返回附带内置访客主体（Guest=true）的上下文。
func withGuest(ctx context.Context, id string) context.Context {
	return auth.WithPrincipal(ctx, &auth.Principal{
		UserID:   id,
		Username: service.GuestUsername,
		Role:     auth.RoleUser,
		Guest:    true,
	})
}

package api

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/service"
)

// policyLimitsTTL 是生效限流缓存的存活时长。管理员调整角色组策略后最多延迟
// 该时长生效，从而避免每个请求都查询数据库。
const policyLimitsTTL = 30 * time.Second

// policyLimitsCacheMax 是缓存条目上限；超出时整体清空，避免无界增长。
const policyLimitsCacheMax = 10000

// policyLimits 按主体解析并短期缓存生效的速率限制。
type policyLimits struct {
	policies *service.PolicyService

	mu    sync.Mutex
	cache map[string]policyEntry
}

// policyEntry 是一条缓存的速率限制。
type policyEntry struct {
	rate    service.RateSettings
	expires time.Time
}

// newPolicyLimits 构造一个策略限流解析器。
func newPolicyLimits(policies *service.PolicyService) *policyLimits {
	return &policyLimits{policies: policies, cache: make(map[string]policyEntry)}
}

// forPrincipal 返回某个主体生效的速率限制，命中缓存时直接返回。
func (p *policyLimits) forPrincipal(ctx context.Context, principal *auth.Principal) (service.RateSettings, error) {
	key := "anonymous"
	if principal != nil && principal.UserID != "" {
		if principal.IsGuest() {
			// 所有访客共用 Guest 角色组策略，按 IP 限流，无需按访客账户区分缓存。
			key = "guest"
		} else {
			key = string(principal.Role) + ":" + principal.UserID
		}
	}
	now := time.Now()
	p.mu.Lock()
	if entry, ok := p.cache[key]; ok && now.Before(entry.expires) {
		p.mu.Unlock()
		return entry.rate, nil
	}
	p.mu.Unlock()

	limits, err := p.policies.RateLimitsFor(ctx, principal)
	if err != nil {
		return service.RateSettings{}, err
	}
	p.mu.Lock()
	if len(p.cache) >= policyLimitsCacheMax {
		p.cache = make(map[string]policyEntry)
	}
	p.cache[key] = policyEntry{rate: limits, expires: now.Add(policyLimitsTTL)}
	p.mu.Unlock()
	return limits, nil
}

// uploadRateLimit 依据调用方角色组策略对上传接口限流。策略解析失败时放行并
// 记录告警，避免数据库瞬时故障导致上传不可用。
func (h *Handler) uploadRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal := principalOf(r)
		limits, err := h.policyLimits.forPrincipal(r.Context(), principal)
		if err != nil {
			h.logger.WarnContext(r.Context(), "resolve rate limits", slog.Any("error", err))
			next.ServeHTTP(w, r)
			return
		}
		key := "ip:" + requestClientIP(r)
		if principal != nil && !principal.IsGuest() {
			key = "user:" + principal.UserID
		}
		if !h.policyRateLimiter.AllowWith(key, limits.UploadPerMinute, limits.UploadBurst) {
			w.Header().Set("Retry-After", "60")
			writeError(w, http.StatusTooManyRequests, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// imageRateLimit 依据匿名访客（回退默认）策略按 IP 对公开图片读取/处理限流。
func (h *Handler) imageRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limits, err := h.policyLimits.forPrincipal(r.Context(), nil)
		if err != nil {
			h.logger.WarnContext(r.Context(), "resolve rate limits", slog.Any("error", err))
			next.ServeHTTP(w, r)
			return
		}
		if !h.policyRateLimiter.AllowWith("ip:"+requestClientIP(r), limits.ImagePerMinute, limits.ImageBurst) {
			w.Header().Set("Retry-After", "60")
			writeError(w, http.StatusTooManyRequests, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requestClientIP 返回请求的客户端地址（不含端口）。当 trust_proxy 启用时，
// RemoteAddr 已在更早的中间件中被转发头重写。
func requestClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

package auth

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// maxRateLimiterKeys 是限流器映射在触发淘汰前的规模上限。
const maxRateLimiterKeys = 10000

// staleLimiterAfter 是空闲键在淘汰前保留的时长。
const staleLimiterAfter = 10 * time.Minute

// RateLimiter 是按 key 划分的令牌桶限流器。nil 指针允许一切请求，
// 便于直接关闭限流。
type RateLimiter struct {
	mu       sync.Mutex
	limit    rate.Limit
	burst    int
	limiters map[string]*rate.Limiter
	seen     map[string]time.Time
}

// NewRateLimiter 创建每分钟允许 perMinute 次请求、突发为 burst 的限流器。
// 当任一参数非正时返回 nil（表示禁用限流）。
func NewRateLimiter(perMinute, burst int) *RateLimiter {
	if perMinute <= 0 || burst <= 0 {
		return nil
	}
	return &RateLimiter{
		limit:    rate.Limit(float64(perMinute) / 60.0),
		burst:    burst,
		limiters: make(map[string]*rate.Limiter),
		seen:     make(map[string]time.Time),
	}
}

// Allow 判断 key 是否可以继续。nil 限流器始终放行。
func (r *RateLimiter) Allow(key string) bool {
	if r == nil {
		return true
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	limiter, ok := r.limiters[key]
	if !ok {
		limiter = rate.NewLimiter(r.limit, r.burst)
		r.limiters[key] = limiter
		if len(r.limiters) > maxRateLimiterKeys {
			r.evictLocked()
		}
	}
	r.seen[key] = time.Now()
	return limiter.Allow()
}

// NewDynamicRateLimiter 创建一个按 key 动态指定限额的限流器，供策略驱动的限流
// 使用。与 NewRateLimiter 不同，每个 key 的速率与突发额度在 AllowWith 时给出。
func NewDynamicRateLimiter() *RateLimiter {
	return &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		seen:     make(map[string]time.Time),
	}
}

// AllowWith 以给定的每分钟限额与突发额度判断 key 是否放行。perMinute 或 burst
// 非正表示不限制。当同一 key 的限额发生变化（例如管理员调整了角色组策略）时会
// 重建对应的令牌桶。
func (r *RateLimiter) AllowWith(key string, perMinute, burst int) bool {
	if r == nil || perMinute <= 0 || burst <= 0 {
		return true
	}
	limit := rate.Limit(float64(perMinute) / 60.0)
	r.mu.Lock()
	defer r.mu.Unlock()

	limiter, ok := r.limiters[key]
	if !ok || limiter.Limit() != limit || limiter.Burst() != burst {
		limiter = rate.NewLimiter(limit, burst)
		r.limiters[key] = limiter
		if len(r.limiters) > maxRateLimiterKeys {
			r.evictLocked()
		}
	}
	r.seen[key] = time.Now()
	return limiter.Allow()
}

// evictLocked 淘汰已空闲一段时间的限流器。调用方需持有 mu。
func (r *RateLimiter) evictLocked() {
	cutoff := time.Now().Add(-staleLimiterAfter)
	for key, seen := range r.seen {
		if seen.Before(cutoff) {
			delete(r.limiters, key)
			delete(r.seen, key)
		}
	}
}

// Middleware 按客户端 IP 进行限流。nil 限流器放行所有请求。
func (r *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if r == nil {
			next.ServeHTTP(w, req)
			return
		}
		if !r.Allow("ip:" + clientIP(req)) {
			w.Header().Set("Retry-After", "60")
			writeAuthError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, req)
	})
}

// UploadLimiter 对已认证用户与访客分别施加不同的限流。
type UploadLimiter struct {
	User  *RateLimiter
	Guest *RateLimiter
}

// Middleware 根据请求主体身份施加对应的限流。
func (u *UploadLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u == nil {
			next.ServeHTTP(w, r)
			return
		}
		limiter := u.Guest
		key := "ip:" + clientIP(r)
		if principal, ok := FromContext(r.Context()); ok && !principal.IsGuest() {
			limiter = u.User
			key = "user:" + principal.UserID
		}
		if !limiter.Allow(key) {
			w.Header().Set("Retry-After", "60")
			writeAuthError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

package auth

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// maxRateLimiterKeys bounds the in-memory limiter map before eviction runs.
const maxRateLimiterKeys = 10000

// staleLimiterAfter is how long an idle key is retained before eviction.
const staleLimiterAfter = 10 * time.Minute

// RateLimiter is a per-key token-bucket limiter. A nil *RateLimiter allows
// everything, which conveniently disables limiting.
type RateLimiter struct {
	mu       sync.Mutex
	limit    rate.Limit
	burst    int
	limiters map[string]*rate.Limiter
	seen     map[string]time.Time
}

// NewRateLimiter creates a limiter allowing perMinute requests with the given
// burst. It returns nil (disabled) when either value is non-positive.
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

// Allow reports whether the key may proceed. A nil limiter always allows.
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

// evictLocked drops limiters that have been idle for a while. Caller holds mu.
func (r *RateLimiter) evictLocked() {
	cutoff := time.Now().Add(-staleLimiterAfter)
	for key, seen := range r.seen {
		if seen.Before(cutoff) {
			delete(r.limiters, key)
			delete(r.seen, key)
		}
	}
}

// Middleware rate-limits requests by client IP. A nil limiter allows all.
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

// UploadLimiter applies distinct limits to authenticated users and guests.
type UploadLimiter struct {
	User  *RateLimiter
	Guest *RateLimiter
}

// Middleware enforces the appropriate limit based on the request principal.
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

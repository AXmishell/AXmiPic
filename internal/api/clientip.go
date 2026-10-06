package api

import (
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/AXmishell/axmipic/internal/config"
)

// clientIPResolver 依据配置解析客户端真实 IP。核心安全约束：只有当直接对端位于
// 可信代理网段内时，才采信转发头；否则一律回退到对端地址，防止伪造。
type clientIPResolver struct {
	source  string
	header  string
	trusted []*net.IPNet
	depth   int
	logger  *slog.Logger
}

// NewClientIPMiddleware 构建解析客户端 IP 的中间件。legacyTrustProxy 为旧的
// server.trust_proxy 开关：当未显式配置 source 且其为真时，等价于
// source=x-forwarded-for（但仍要求 trusted_proxies，否则回退 remote）。
func NewClientIPMiddleware(cfg config.ClientIPConfig, legacyTrustProxy bool, logger *slog.Logger) func(http.Handler) http.Handler {
	return newClientIPResolver(cfg, legacyTrustProxy, logger).middleware
}

func newClientIPResolver(cfg config.ClientIPConfig, legacyTrustProxy bool, logger *slog.Logger) *clientIPResolver {
	source := strings.ToLower(strings.TrimSpace(cfg.Source))
	if source == "" {
		if legacyTrustProxy {
			source = "x-forwarded-for"
		} else {
			source = "remote"
		}
	}
	resolver := &clientIPResolver{
		source: source,
		header: strings.TrimSpace(cfg.Header),
		depth:  cfg.XFFDepth,
		logger: logger,
	}
	for _, raw := range cfg.TrustedProxies {
		if _, ipnet, err := net.ParseCIDR(strings.TrimSpace(raw)); err == nil {
			resolver.trusted = append(resolver.trusted, ipnet)
		}
	}
	if source != "remote" && len(resolver.trusted) == 0 {
		if logger != nil {
			logger.Warn("client_ip.source requires trusted_proxies; falling back to the peer address",
				slog.String("source", source))
		}
		resolver.source = "remote"
	}
	return resolver
}

// middleware 用解析结果重写 RemoteAddr，供下游限流/配额/日志统一使用。
func (r *clientIPResolver) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if ip := r.resolve(req); ip != "" {
			req.RemoteAddr = ip
		}
		next.ServeHTTP(w, req)
	})
}

// resolve 返回用于限流/配额/日志的客户端 IP（不含端口）。
func (r *clientIPResolver) resolve(req *http.Request) string {
	peer := hostOnly(req.RemoteAddr)
	if r == nil || r.source == "remote" {
		return peer
	}
	if !r.isTrustedPeer(net.ParseIP(peer)) {
		return peer
	}
	if candidate := r.candidate(req); candidate != "" {
		return candidate
	}
	return peer
}

func (r *clientIPResolver) isTrustedPeer(ip net.IP) bool {
	if ip == nil {
		return false
	}
	for _, n := range r.trusted {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func (r *clientIPResolver) candidate(req *http.Request) string {
	switch r.source {
	case "x-forwarded-for":
		return pickForwardedFor(req.Header.Get("X-Forwarded-For"), r.trusted, r.depth)
	case "x-real-ip":
		return normalizeIP(req.Header.Get("X-Real-IP"))
	case "cf-connecting-ip":
		return normalizeIP(req.Header.Get("CF-Connecting-IP"))
	case "true-client-ip":
		return normalizeIP(req.Header.Get("True-Client-IP"))
	case "x-client-ip":
		return normalizeIP(req.Header.Get("X-Client-IP"))
	case "forwarded":
		return pickForwardedRFC7239(req.Header.Get("Forwarded"), r.trusted, r.depth)
	case "custom":
		if r.header == "" {
			return ""
		}
		return normalizeIP(req.Header.Get(r.header))
	default:
		return ""
	}
}

// pickForwardedFor 从 X-Forwarded-For 中选出客户端地址。depth>0 时从右往左跳过
// depth 个（可信代理）后取值；depth==0 时取右起第一个不在可信网段中的地址。
func pickForwardedFor(value string, trusted []*net.IPNet, depth int) string {
	ips := make([]string, 0)
	for _, part := range strings.Split(value, ",") {
		if ip := normalizeIP(part); ip != "" {
			ips = append(ips, ip)
		}
	}
	return selectForwarded(ips, trusted, depth)
}

// pickForwardedRFC7239 从 RFC 7239 的 Forwarded 头中解析 for= 参数并选出客户端地址。
func pickForwardedRFC7239(value string, trusted []*net.IPNet, depth int) string {
	ips := make([]string, 0)
	for _, element := range strings.Split(value, ",") {
		for _, pair := range strings.Split(element, ";") {
			key, raw, ok := strings.Cut(pair, "=")
			if !ok || !strings.EqualFold(strings.TrimSpace(key), "for") {
				continue
			}
			raw = strings.Trim(strings.TrimSpace(raw), `"`)
			if ip := normalizeIP(raw); ip != "" {
				ips = append(ips, ip)
			}
		}
	}
	return selectForwarded(ips, trusted, depth)
}

func selectForwarded(ips []string, trusted []*net.IPNet, depth int) string {
	if len(ips) == 0 {
		return ""
	}
	if depth > 0 {
		idx := len(ips) - 1 - depth
		if idx < 0 {
			return ""
		}
		return ips[idx]
	}
	for i := len(ips) - 1; i >= 0; i-- {
		if !ipInNets(net.ParseIP(ips[i]), trusted) {
			return ips[i]
		}
	}
	return ips[0]
}

func ipInNets(ip net.IP, nets []*net.IPNet) bool {
	if ip == nil {
		return false
	}
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// normalizeIP 去掉端口、方括号与 IPv6 zone，返回规范 IP；非法返回空字符串。
func normalizeIP(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if i := strings.IndexByte(s, '%'); i >= 0 {
		s = s[:i]
	}
	if host, _, err := net.SplitHostPort(s); err == nil {
		s = host
	} else {
		s = strings.Trim(s, "[]")
	}
	if net.ParseIP(s) == nil {
		return ""
	}
	return s
}

// hostOnly 返回 host:port 中的 host；无端口时原样返回。
func hostOnly(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

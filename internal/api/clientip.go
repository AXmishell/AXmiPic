package api

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/AXmishell/axmipic/internal/config"
)

// ClientIPResolver 依据配置解析客户端真实 IP，并支持运行时热更新。核心安全约束：
// 只有当直接对端位于可信代理网段内时，才采信转发头；否则一律回退到对端地址，
// 防止伪造。
type ClientIPResolver struct {
	mu      sync.RWMutex
	source  string
	header  string
	trusted []*net.IPNet
	depth   int
	logger  *slog.Logger
	legacy  bool
}

// ClientIPInfo 描述一次请求/模拟的 IP 解析结果，供后台校验配置。
type ClientIPInfo struct {
	Resolved       string   `json:"resolved"`
	Peer           string   `json:"peer"`
	Source         string   `json:"source"`
	Header         string   `json:"header"`
	TrustedPeer    bool     `json:"trusted_peer"`
	TrustedProxies []string `json:"trusted_proxies"`
	XFFDepth       int      `json:"xff_depth"`
	XForwardedFor  string   `json:"x_forwarded_for,omitempty"`
	XRealIP        string   `json:"x_real_ip,omitempty"`
	CFConnectingIP string   `json:"cf_connecting_ip,omitempty"`
	TrueClientIP   string   `json:"true_client_ip,omitempty"`
	Forwarded      string   `json:"forwarded,omitempty"`
}

// NewClientIPResolver 构造解析器。legacyTrustProxy 为旧的 server.trust_proxy
// 开关：当未显式配置 source 且其为真时，等价于 source=x-forwarded-for。
func NewClientIPResolver(cfg config.ClientIPConfig, legacyTrustProxy bool, logger *slog.Logger) *ClientIPResolver {
	r := &ClientIPResolver{logger: logger, legacy: legacyTrustProxy}
	r.Update(cfg)
	return r
}

// Update 用新配置替换解析规则（运行时热更新）。
func (r *ClientIPResolver) Update(cfg config.ClientIPConfig) {
	source := strings.ToLower(strings.TrimSpace(cfg.Source))
	if source == "" {
		if r.legacy {
			source = "x-forwarded-for"
		} else {
			source = "remote"
		}
	}
	var trusted []*net.IPNet
	for _, raw := range cfg.TrustedProxies {
		if _, ipnet, err := net.ParseCIDR(strings.TrimSpace(raw)); err == nil {
			trusted = append(trusted, ipnet)
		}
	}
	if source != "remote" && len(trusted) == 0 {
		if r.logger != nil {
			r.logger.Warn("client_ip.source requires trusted_proxies; falling back to the peer address",
				slog.String("source", source))
		}
		source = "remote"
	}
	r.mu.Lock()
	r.source = source
	r.header = strings.TrimSpace(cfg.Header)
	r.trusted = trusted
	r.depth = cfg.XFFDepth
	r.mu.Unlock()
}

// Middleware 用解析结果重写 RemoteAddr，供下游限流/配额/日志统一使用。
func (r *ClientIPResolver) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if ip := r.resolve(req); ip != "" {
				req.RemoteAddr = ip
			}
			next.ServeHTTP(w, req)
		})
	}
}

func (r *ClientIPResolver) snapshot() (string, string, []*net.IPNet, int) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.source, r.header, r.trusted, r.depth
}

// resolve 返回用于限流/配额/日志的客户端 IP（不含端口）。
func (r *ClientIPResolver) resolve(req *http.Request) string {
	resolved, _, _ := r.resolveFrom(req.RemoteAddr, req.Header.Get)
	return resolved
}

func (r *ClientIPResolver) resolveFrom(remoteAddr string, get func(string) string) (resolved, peer string, trustedPeer bool) {
	peer = hostOnly(remoteAddr)
	source, header, trusted, depth := r.snapshot()
	if source == "remote" {
		return peer, peer, false
	}
	trustedPeer = ipInNets(net.ParseIP(peer), trusted)
	if !trustedPeer {
		return peer, peer, false
	}
	if candidate := r.candidateFrom(source, header, get, trusted, depth); candidate != "" {
		return candidate, peer, true
	}
	return peer, peer, true
}

func (r *ClientIPResolver) candidateFrom(source, header string, get func(string) string, trusted []*net.IPNet, depth int) string {
	switch source {
	case "x-forwarded-for":
		return pickForwardedFor(get("X-Forwarded-For"), trusted, depth)
	case "x-real-ip":
		return normalizeIP(get("X-Real-IP"))
	case "cf-connecting-ip":
		return normalizeIP(get("CF-Connecting-IP"))
	case "true-client-ip":
		return normalizeIP(get("True-Client-IP"))
	case "x-client-ip":
		return normalizeIP(get("X-Client-IP"))
	case "forwarded":
		return pickForwardedRFC7239(get("Forwarded"), trusted, depth)
	case "custom":
		if header == "" {
			return ""
		}
		return normalizeIP(get(header))
	default:
		return ""
	}
}

// Describe 返回当前请求的解析详情，供后台校验。
func (r *ClientIPResolver) Describe(req *http.Request) ClientIPInfo {
	return r.describe(req.RemoteAddr, req.Header.Get)
}

// Preview 用给定的对端地址与请求头模拟解析，供后台校验规则。
func (r *ClientIPResolver) Preview(remoteAddr string, headers map[string]string) ClientIPInfo {
	return r.describe(remoteAddr, func(name string) string { return headers[name] })
}

func (r *ClientIPResolver) describe(remoteAddr string, get func(string) string) ClientIPInfo {
	source, header, trusted, depth := r.snapshot()
	resolved, peer, trustedPeer := r.resolveFrom(remoteAddr, get)
	proxies := make([]string, 0, len(trusted))
	for _, n := range trusted {
		proxies = append(proxies, n.String())
	}
	return ClientIPInfo{
		Resolved:       resolved,
		Peer:           peer,
		Source:         source,
		Header:         header,
		TrustedPeer:    trustedPeer,
		TrustedProxies: proxies,
		XFFDepth:       depth,
		XForwardedFor:  get("X-Forwarded-For"),
		XRealIP:        get("X-Real-IP"),
		CFConnectingIP: get("CF-Connecting-IP"),
		TrueClientIP:   get("True-Client-IP"),
		Forwarded:      get("Forwarded"),
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

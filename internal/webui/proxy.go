package webui

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// NewDevProxy 返回一个把前端请求反向代理到 Vite 开发服务器的处理器，
// 用于本地开发时的 HMR 热更新模式。浏览器只需访问后端端口（默认 8080），
// 页面、模块与 HMR WebSocket 均由 Vite 提供。
//
// 为避免反向代理环路，/api、/i/ 与 /healthz 前缀不转发：这些路径始终由后端
// 自己处理（Vite 的 proxy 配置仅用于直接访问 :5173 的场景）。
func NewDevProxy(target string) (http.Handler, error) {
	u, err := url.Parse(strings.TrimSpace(target))
	if err != nil {
		return nil, fmt.Errorf("webui: parse hot reload target: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("webui: hot reload target %q must use http or https", target)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("webui: hot reload target %q must include a host", target)
	}

	proxy := httputil.NewSingleHostReverseProxy(u)
	director := proxy.Director
	proxy.Director = func(r *http.Request) {
		director(r)
		// 让 Vite 的 Host 校验通过：转发到目标主机而不是后端监听地址。
		r.Host = u.Host
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Warn("web hot reload proxy failed",
			slog.String("path", r.URL.Path),
			slog.String("target", u.String()),
			slog.Any("error", err),
		)
		http.Error(w, "web hot reload proxy: is the Vite dev server running?", http.StatusBadGateway)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/"):
			http.NotFound(w, r)
			return
		case strings.HasPrefix(r.URL.Path, "/i/"):
			http.NotFound(w, r)
			return
		case r.URL.Path == "/healthz":
			http.NotFound(w, r)
			return
		}
		proxy.ServeHTTP(w, r)
	}), nil
}

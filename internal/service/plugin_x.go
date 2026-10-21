package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AXmishell/axmipic/internal/plugin"
)

// BuiltinXResolver 是内置的 X 链接解析插件名称。它只负责把状态链接解析为
// 作者与图片 URL，真正的下载、上传与发布由宿主完成。
const BuiltinXResolver = "builtin-x-resolver"

// X 导入解析接口。后台可在插件配置中切换。
const (
	XResolverFxtwitter = "fxtwitter"
	XResolverOfficial  = "official"
)

const (
	xResolverDefaultTimeout = 15 * time.Second
	xResolverMaxBody        = 2 << 20
	xResolverMaxPhotos      = 4
)

// 解析接口根地址；测试中可替换为本地 httptest 服务。
var (
	fxtwitterAPIBase = "https://api.fxtwitter.com"
	officialAPIBase  = "https://api.x.com"
)

// XResolvedAuthor 是解析结果中的作者信息。
type XResolvedAuthor struct {
	Name   string `json:"name"`
	Handle string `json:"handle"`
}

// XResolvedMedia 是解析结果中的一张候选图片。
type XResolvedMedia struct {
	ID     string `json:"id,omitempty"`
	Type   string `json:"type"`
	URL    string `json:"url"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	Alt    string `json:"alt,omitempty"`
}

// XResolvedPost 是解析插件返回的规范化帖子信息。
type XResolvedPost struct {
	Platform  string           `json:"platform"`
	PostID    string           `json:"post_id"`
	SourceURL string           `json:"source_url"`
	Author    XResolvedAuthor  `json:"author"`
	Text      string           `json:"text,omitempty"`
	Sensitive bool             `json:"sensitive,omitempty"`
	Media     []XResolvedMedia `json:"media"`
}

// builtinXResolverProvider 通过 fxtwitter 或 X 官方 API 解析状态链接。
type builtinXResolverProvider struct {
	mu       sync.RWMutex
	provider string
	bearer   string
	variant  string
	timeout  time.Duration
	client   *http.Client
}

// RegisterBuiltinXResolver 把 X 链接解析器注册为内置插件。它随宿主编译，
// 默认受信、无需 WASM 沙箱；配置仍通过插件市场管理。
func (s *PluginService) RegisterBuiltinXResolver() {
	if s.manager == nil {
		return
	}
	s.manager.RegisterBuiltin(plugin.Manifest{
		Name:     BuiltinXResolver,
		Version:  "1.0.0",
		Category: plugin.CategoryImportX,
		Runtime:  plugin.RuntimeBuiltin,
		ABI:      plugin.ABI,
	}, newBuiltinXResolver())
}

func newBuiltinXResolver() *builtinXResolverProvider {
	return &builtinXResolverProvider{
		provider: XResolverFxtwitter,
		variant:  "orig",
		timeout:  xResolverDefaultTimeout,
	}
}

// Descriptor 返回插件自描述，后台据此渲染配置表单。
func (p *builtinXResolverProvider) Descriptor() plugin.Descriptor {
	return plugin.Descriptor{
		Category: plugin.CategoryImportX,
		Name:     BuiltinXResolver,
		Title:    "X 链接导入（内置）",
		Version:  "1.0.0",
		Origin:   plugin.OriginBuiltin,
		Fields: []plugin.Field{
			{
				Key:      "provider",
				Label:    "解析接口",
				Type:     plugin.FieldSelect,
				Required: true,
				Default:  XResolverFxtwitter,
				Options:  []string{XResolverFxtwitter, XResolverOfficial},
				Help:     "fxtwitter 免费、无需密钥；official 使用 X 官方 API v2，需要 Bearer Token。",
			},
			{
				Key:    "bearer_token",
				Label:  "X API Bearer Token",
				Type:   plugin.FieldPassword,
				Secret: true,
				Help:   "仅 official 接口需要。保存后不会回显明文。",
				VisibleWhen: &plugin.FieldVisibility{
					Key:    "provider",
					Values: []string{XResolverOfficial},
				},
			},
			{
				Key:     "timeout_sec",
				Label:   "请求超时（秒）",
				Type:    plugin.FieldInt,
				Default: "15",
			},
			{
				Key:     "image_variant",
				Label:   "图片尺寸",
				Type:    plugin.FieldSelect,
				Default: "orig",
				Options: []string{"orig", "large", "medium"},
				Help:    "orig 为原图，large/medium 体积更小。",
			},
			{
				Key:   "proxy_url",
				Label: "代理地址（可选）",
				Help:  "例如 http://127.0.0.1:7890 或 socks5://127.0.0.1:1080；留空时使用系统环境变量代理。需要认证时请使用下方用户名/密码字段。",
			},
			{
				Key:   "proxy_username",
				Label: "代理用户名（可选）",
				Help:  "仅当代理需要认证时填写。",
			},
			{
				Key:    "proxy_password",
				Label:  "代理密码（可选）",
				Type:   plugin.FieldPassword,
				Secret: true,
				Help:   "与代理用户名配合使用；保存后不会回显明文。",
			},
		},
	}
}

// Configure 应用后台保存的配置。
func (p *builtinXResolverProvider) Configure(_ context.Context, cfg map[string]string) error {
	provider := strings.ToLower(strings.TrimSpace(cfg["provider"]))
	if provider == "" {
		provider = XResolverFxtwitter
	}
	if provider != XResolverFxtwitter && provider != XResolverOfficial {
		return fmt.Errorf("plugin %q: unsupported provider %q", BuiltinXResolver, provider)
	}
	bearer := strings.TrimSpace(cfg["bearer_token"])
	timeout := xResolverDefaultTimeout
	if raw := strings.TrimSpace(cfg["timeout_sec"]); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 3 || n > 120 {
			return fmt.Errorf("plugin %q: timeout_sec must be between 3 and 120", BuiltinXResolver)
		}
		timeout = time.Duration(n) * time.Second
	}
	variant := strings.ToLower(strings.TrimSpace(cfg["image_variant"]))
	switch variant {
	case "", "orig":
		variant = "orig"
	case "large", "medium":
	default:
		return fmt.Errorf("plugin %q: unsupported image_variant %q", BuiltinXResolver, variant)
	}
	proxy, err := proxyFromConfig(cfg)
	if err != nil {
		return fmt.Errorf("plugin %q: %w", BuiltinXResolver, err)
	}

	p.mu.Lock()
	p.provider = provider
	p.bearer = bearer
	p.variant = variant
	p.timeout = timeout
	p.client = &http.Client{
		Timeout:   timeout,
		Transport: transportWithProxy(proxy),
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			if !isXResolverHost(req.URL.Hostname()) {
				return fmt.Errorf("plugin %q: redirect to unsupported host %q", BuiltinXResolver, req.URL.Hostname())
			}
			return nil
		},
	}
	p.mu.Unlock()
	return nil
}

// XResolverProxy 返回内置 X 解析插件当前配置的代理。未配置代理时返回 nil。
// 宿主导入服务在下载图片时复用它，确保解析与下载走同一出口。
func (s *PluginService) XResolverProxy(ctx context.Context) (*url.URL, error) {
	cfg, err := s.resolve(ctx, BuiltinXResolver)
	if err != nil {
		return nil, err
	}
	return proxyFromConfig(cfg)
}

// proxyFromConfig 从插件配置构造代理 URL。支持 http/https/socks5/socks5h；
// 未配置 proxy_url 时返回 nil，保留 Go 默认的环境变量代理行为。
func proxyFromConfig(cfg map[string]string) (*url.URL, error) {
	raw := strings.TrimSpace(cfg["proxy_url"])
	username := strings.TrimSpace(cfg["proxy_username"])
	password := cfg["proxy_password"]
	if raw == "" {
		// 没有代理地址时忽略孤立的用户名/密码，避免无法清除密文字段时把插件
		// 配置卡在错误状态。
		return nil, nil
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return nil, fmt.Errorf("invalid proxy_url %q", raw)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	switch parsed.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, fmt.Errorf("unsupported proxy scheme %q", parsed.Scheme)
	}
	if username != "" {
		parsed.User = url.UserPassword(username, password)
	}
	return parsed, nil
}

// transportWithProxy 复制默认 Transport 并按需覆盖代理。nil 代理时保留
// ProxyFromEnvironment，便于容器/系统通过环境变量统一配置代理。
func transportWithProxy(proxy *url.URL) *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxy != nil {
		transport.Proxy = http.ProxyURL(proxy)
	}
	return transport
}

// Invoke 执行 resolve 操作。
func (p *builtinXResolverProvider) Invoke(ctx context.Context, op string, input []byte) ([]byte, error) {
	if op != "resolve" {
		return nil, fmt.Errorf("plugin %q: unsupported op %q", BuiltinXResolver, op)
	}
	var req struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("plugin %q: invalid input: %w", BuiltinXResolver, err)
	}
	ref, err := ParseXStatusURL(req.URL)
	if err != nil {
		return nil, err
	}
	post, err := p.resolve(ctx, ref)
	if err != nil {
		return nil, err
	}
	return json.Marshal(post)
}

func (p *builtinXResolverProvider) Close(context.Context) error { return nil }

// snapshot 读取当前配置，避免请求期间被后台配置修改。
func (p *builtinXResolverProvider) snapshot() (provider, bearer, variant string, timeout time.Duration, client *http.Client) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	provider, bearer, variant, timeout = p.provider, p.bearer, p.variant, p.timeout
	client = p.client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	return
}

func (p *builtinXResolverProvider) resolve(ctx context.Context, ref XStatusRef) (*XResolvedPost, error) {
	provider, bearer, variant, timeout, client := p.snapshot()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	switch provider {
	case XResolverOfficial:
		if bearer == "" {
			return nil, fmt.Errorf("plugin %q: bearer_token is not configured for the official API", BuiltinXResolver)
		}
		return p.resolveOfficial(ctx, client, bearer, variant, ref)
	default:
		return p.resolveFxtwitter(ctx, client, variant, ref)
	}
}

// resolveFxtwitter 调用 fxtwitter 的公开 API。
func (p *builtinXResolverProvider) resolveFxtwitter(ctx context.Context, client *http.Client, variant string, ref XStatusRef) (*XResolvedPost, error) {
	endpoint := fxtwitterAPIBase + "/i/status/" + ref.PostID
	var payload fxTweetResponse
	if err := fetchJSON(ctx, client, endpoint, "", &payload); err != nil {
		return nil, fmt.Errorf("x resolver (fxtwitter): %w", err)
	}
	if payload.Code != 0 && payload.Code != http.StatusOK {
		msg := strings.TrimSpace(payload.Message)
		if msg == "" {
			msg = "upstream returned an error"
		}
		return nil, fmt.Errorf("x resolver (fxtwitter): %s", msg)
	}
	tweet := payload.Tweet
	if tweet.ID == "" {
		tweet.ID = ref.PostID
	}
	handle := strings.TrimPrefix(strings.TrimSpace(tweet.Author.ScreenName), "@")
	if handle == "" {
		handle = ref.Handle
	}
	name := strings.TrimSpace(tweet.Author.Name)
	if tweet.Author.Protected {
		return nil, fmt.Errorf("%w: the X account is protected", ErrForbidden)
	}
	post := &XResolvedPost{
		Platform:  "x",
		PostID:    tweet.ID,
		SourceURL: canonicalXURL(handle, tweet.ID),
		Author:    XResolvedAuthor{Name: name, Handle: handle},
		Text:      tweet.Text,
		Sensitive: tweet.PossiblySensitive,
	}
	for _, media := range tweet.Media.All {
		if !strings.EqualFold(media.Type, "photo") || strings.TrimSpace(media.URL) == "" {
			continue
		}
		post.Media = append(post.Media, XResolvedMedia{
			ID:     media.ID,
			Type:   "photo",
			URL:    withImageVariant(media.URL, variant),
			Width:  media.Width,
			Height: media.Height,
			Alt:    media.AltText,
		})
		if len(post.Media) >= xResolverMaxPhotos {
			break
		}
	}
	if len(post.Media) == 0 {
		return nil, fmt.Errorf("%w: the X post contains no photos", ErrInvalidInput)
	}
	return post, nil
}

// resolveOfficial 调用 X 官方 API v2。
func (p *builtinXResolverProvider) resolveOfficial(ctx context.Context, client *http.Client, bearer, variant string, ref XStatusRef) (*XResolvedPost, error) {
	query := url.Values{}
	query.Set("expansions", "author_id,attachments.media_keys")
	query.Set("tweet.fields", "attachments,author_id,possibly_sensitive")
	query.Set("media.fields", "url,preview_image_url,width,height,type,alt_text")
	query.Set("user.fields", "name,username")
	endpoint := officialAPIBase + "/2/tweets/" + ref.PostID + "?" + query.Encode()

	var payload xOfficialResponse
	if err := fetchJSON(ctx, client, endpoint, bearer, &payload); err != nil {
		return nil, fmt.Errorf("x resolver (official): %w", err)
	}
	if payload.Data.ID == "" {
		return nil, fmt.Errorf("x resolver (official): empty tweet response")
	}
	author := XResolvedAuthor{}
	for _, user := range payload.Includes.Users {
		if user.ID == payload.Data.AuthorID {
			author = XResolvedAuthor{Name: strings.TrimSpace(user.Name), Handle: strings.TrimPrefix(strings.TrimSpace(user.Username), "@")}
			break
		}
	}
	if author.Handle == "" {
		author.Handle = ref.Handle
	}
	handle := author.Handle
	post := &XResolvedPost{
		Platform:  "x",
		PostID:    payload.Data.ID,
		SourceURL: canonicalXURL(handle, payload.Data.ID),
		Author:    author,
		Text:      payload.Data.Text,
		Sensitive: payload.Data.PossiblySensitive,
	}
	for _, media := range payload.Includes.Media {
		if !strings.EqualFold(media.Type, "photo") {
			continue
		}
		raw := strings.TrimSpace(media.URL)
		if raw == "" {
			raw = strings.TrimSpace(media.PreviewImageURL)
		}
		if raw == "" {
			continue
		}
		post.Media = append(post.Media, XResolvedMedia{
			ID:     media.MediaKey,
			Type:   "photo",
			URL:    withImageVariant(raw, variant),
			Width:  media.Width,
			Height: media.Height,
			Alt:    media.AltText,
		})
		if len(post.Media) >= xResolverMaxPhotos {
			break
		}
	}
	if len(post.Media) == 0 {
		return nil, fmt.Errorf("%w: the X post contains no photos", ErrInvalidInput)
	}
	return post, nil
}

// fetchJSON 发起一次受限的 GET 请求并解析 JSON。
func fetchJSON(ctx context.Context, client *http.Client, endpoint, bearer string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "AXmiPic/X-Import")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, xResolverMaxBody+1))
	if err != nil {
		return err
	}
	if int64(len(body)) > xResolverMaxBody {
		return fmt.Errorf("upstream response is too large")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("upstream returned HTTP %d: %s", resp.StatusCode, truncateForError(body))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode upstream response: %w", err)
	}
	return nil
}

func truncateForError(body []byte) string {
	const max = 200
	text := strings.TrimSpace(string(body))
	if len(text) > max {
		return text[:max] + "…"
	}
	return text
}

func canonicalXURL(handle, postID string) string {
	handle = strings.TrimPrefix(strings.TrimSpace(handle), "@")
	if handle == "" {
		return "https://x.com/i/status/" + postID
	}
	return "https://x.com/" + handle + "/status/" + postID
}

// withImageVariant 把 pbs.twimg.com 图片的 name 查询参数改写为配置的尺寸。
func withImageVariant(raw, variant string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || variant == "" {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	query := parsed.Query()
	query.Set("name", variant)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

// isXResolverHost 限制解析接口可访问的主机。
func isXResolverHost(host string) bool {
	switch strings.ToLower(host) {
	case "api.fxtwitter.com", "api.x.com", "api.twitter.com":
		return true
	default:
		return false
	}
}

// fxtwitter 响应结构。
type fxTweetResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Tweet   struct {
		ID                string `json:"id"`
		URL               string `json:"url"`
		Text              string `json:"text"`
		PossiblySensitive bool   `json:"possibly_sensitive"`
		Author            struct {
			Name       string `json:"name"`
			ScreenName string `json:"screen_name"`
			Protected  bool   `json:"protected"`
		} `json:"author"`
		Media struct {
			All []struct {
				ID      string `json:"id"`
				Type    string `json:"type"`
				URL     string `json:"url"`
				Width   int    `json:"width"`
				Height  int    `json:"height"`
				AltText string `json:"altText"`
			} `json:"all"`
		} `json:"media"`
	} `json:"tweet"`
}

// X 官方 API v2 响应结构。
type xOfficialResponse struct {
	Data struct {
		ID                string `json:"id"`
		Text              string `json:"text"`
		AuthorID          string `json:"author_id"`
		PossiblySensitive bool   `json:"possibly_sensitive"`
	} `json:"data"`
	Includes struct {
		Users []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Username string `json:"username"`
		} `json:"users"`
		Media []struct {
			MediaKey        string `json:"media_key"`
			Type            string `json:"type"`
			URL             string `json:"url"`
			PreviewImageURL string `json:"preview_image_url"`
			Width           int    `json:"width"`
			Height          int    `json:"height"`
			AltText         string `json:"alt_text"`
		} `json:"media"`
	} `json:"includes"`
}

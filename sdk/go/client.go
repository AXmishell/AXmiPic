// Package axmipic 是 AXmiPic 图床服务的 Go 客户端 SDK。
//
// 它封装了 /api/v1 下的全部接口：认证、上传（含预签名直传）、图片、相册、
// 分享、公告/举报/页面、套餐/订单/优惠券、工单以及管理员接口。所有响应统一
// 解包 `{ code, message, data }` 信封，非零 code 会返回 *Error。
//
// 基本用法：
//
//	client, err := axmipic.New("https://pic.example.com")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	client.SetToken("登录或 API 令牌")
//	image, err := client.Upload(ctx, file)
package axmipic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Version 是 SDK 版本。
const Version = "0.1.0"

// DefaultTimeout 是默认的 HTTP 超时。
const DefaultTimeout = 30 * time.Second

// Client 是 AXmiPic 的 API 客户端。它可安全并发使用。
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	// userAgent 附加到每个请求。
	userAgent string
}

// Option 配置 Client。
type Option func(*Client)

// WithHTTPClient 使用自定义的 *http.Client。
func WithHTTPClient(c *http.Client) Option {
	return func(client *Client) {
		if c != nil {
			client.httpClient = c
		}
	}
}

// WithToken 设置初始的会话或 API 令牌。
func WithToken(token string) Option {
	return func(client *Client) { client.token = token }
}

// WithUserAgent 覆盖默认的 User-Agent。
func WithUserAgent(ua string) Option {
	return func(client *Client) {
		if strings.TrimSpace(ua) != "" {
			client.userAgent = ua
		}
	}
}

// New 使用服务端 baseURL（例如 https://pic.example.com）构造客户端。
func New(baseURL string, opts ...Option) (*Client, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return nil, fmt.Errorf("axmipic: base URL must not be empty")
	}
	if _, err := url.Parse(trimmed); err != nil {
		return nil, fmt.Errorf("axmipic: invalid base URL: %w", err)
	}
	c := &Client{
		baseURL:    trimmed,
		httpClient: &http.Client{Timeout: DefaultTimeout},
		userAgent:  "axmipic-go/" + Version,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// SetToken 设置后续请求使用的令牌。
func (c *Client) SetToken(token string) { c.token = token }

// Token 返回当前的令牌。
func (c *Client) Token() string { return c.token }

// BaseURL 返回服务端基地址。
func (c *Client) BaseURL() string { return c.baseURL }

// envelope 是统一的响应信封。
type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// Error 表示一次失败的 API 调用。
type Error struct {
	// StatusCode 是 HTTP 状态码（网络错误时为 0）。
	StatusCode int
	// Code 是信封中的错误码（通常与 StatusCode 相同）。
	Code int
	// Message 是服务端返回的错误消息。
	Message string
}

// Error 实现 error。
func (e *Error) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("axmipic: request failed with status %d", e.StatusCode)
	}
	return fmt.Sprintf("axmipic: %s (status %d)", e.Message, e.StatusCode)
}

// IsNotFound 报告错误是否为 404。
func (e *Error) IsNotFound() bool { return e.StatusCode == http.StatusNotFound }

// IsUnauthorized 报告错误是否为 401。
func (e *Error) IsUnauthorized() bool { return e.StatusCode == http.StatusUnauthorized }

// IsForbidden 报告错误是否为 403。
func (e *Error) IsForbidden() bool { return e.StatusCode == http.StatusForbidden }

// Do 发送一次请求并把 data 解包到 out（out 可为 nil）。body 为可选的请求体；
// contentType 非空时作为 Content-Type。
func (c *Client) Do(ctx context.Context, method, path string, body []byte, contentType string, out any) error {
	target := c.baseURL + path
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return fmt.Errorf("axmipic: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &Error{StatusCode: 0, Code: -1, Message: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return fmt.Errorf("axmipic: read response: %w", err)
	}
	var env envelope
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &env); err != nil {
			// 非 JSON 响应（例如网关错误页）。
			if resp.StatusCode >= 400 {
				return &Error{StatusCode: resp.StatusCode, Code: resp.StatusCode, Message: strings.TrimSpace(string(raw))}
			}
			return fmt.Errorf("axmipic: decode response: %w", err)
		}
	}
	if resp.StatusCode >= 400 || env.Code != 0 {
		message := env.Message
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return &Error{StatusCode: resp.StatusCode, Code: env.Code, Message: message}
	}
	if out != nil && len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("axmipic: decode data: %w", err)
		}
	}
	return nil
}

// get 发送 GET 请求。
func (c *Client) get(ctx context.Context, path string, out any) error {
	return c.Do(ctx, http.MethodGet, path, nil, "", out)
}

// postJSON 发送 JSON POST 请求。
func (c *Client) postJSON(ctx context.Context, path string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("axmipic: encode request: %w", err)
	}
	return c.Do(ctx, http.MethodPost, path, body, "application/json", out)
}

// putJSON 发送 JSON PUT 请求。
func (c *Client) putJSON(ctx context.Context, path string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("axmipic: encode request: %w", err)
	}
	return c.Do(ctx, http.MethodPut, path, body, "application/json", out)
}

// patchJSON 发送 JSON PATCH 请求。
func (c *Client) patchJSON(ctx context.Context, path string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("axmipic: encode request: %w", err)
	}
	return c.Do(ctx, http.MethodPatch, path, body, "application/json", out)
}

// delete 发送 DELETE 请求。
func (c *Client) delete(ctx context.Context, path string, out any) error {
	return c.Do(ctx, http.MethodDelete, path, nil, "", out)
}

// encodeQuery 把非空参数拼接到路径上。
func encodeQuery(path string, params map[string]string) string {
	if len(params) == 0 {
		return path
	}
	values := url.Values{}
	for k, v := range params {
		if strings.TrimSpace(v) != "" {
			values.Set(k, v)
		}
	}
	if len(values) == 0 {
		return path
	}
	if strings.Contains(path, "?") {
		return path + "&" + values.Encode()
	}
	return path + "?" + values.Encode()
}

// newUploadBody 构造 multipart 上传体。
func newUploadBody(field, filename string, data []byte) (*bytes.Buffer, string, error) {
	buf := &bytes.Buffer{}
	writer := multipart.NewWriter(buf)
	part, err := writer.CreateFormFile(field, filename)
	if err != nil {
		return nil, "", fmt.Errorf("axmipic: build multipart: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return nil, "", fmt.Errorf("axmipic: write multipart: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("axmipic: close multipart: %w", err)
	}
	return buf, writer.FormDataContentType(), nil
}

// multipartFields 按预签名的表单字段构造 multipart 体（七牛等后端使用）。
func multipartFields(fields map[string]string, fileField string, data []byte) (*bytes.Buffer, string, error) {
	buf := &bytes.Buffer{}
	writer := multipart.NewWriter(buf)
	for k, v := range fields {
		if err := writer.WriteField(k, v); err != nil {
			return nil, "", fmt.Errorf("axmipic: write field: %w", err)
		}
	}
	part, err := writer.CreateFormFile(fileField, "file")
	if err != nil {
		return nil, "", fmt.Errorf("axmipic: create file field: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return nil, "", fmt.Errorf("axmipic: write file: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("axmipic: close multipart: %w", err)
	}
	return buf, writer.FormDataContentType(), nil
}

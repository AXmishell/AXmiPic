package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HTTPSSender 通过一个通用的 HTTP 接口发送短信。它可以对接自建网关或把请求
// 转交给云厂商的 HTTP API；通过 Method、Params 模板与 Headers 配置请求形态。
type HTTPSSender struct {
	name       string
	endpoint   string
	method     string
	params     map[string]string
	headers    map[string]string
	httpClient *http.Client
}

// HTTPOptions 是构造 HTTPSSender 的参数。params 与 headers 的值支持以下占位符：
// {{to}}、{{body}}、{{subject}}、{{sign}}、{{template}}。
type HTTPOptions struct {
	Name     string
	Endpoint string
	Method   string
	Params   map[string]string
	Headers  map[string]string
	Timeout  time.Duration
}

// NewHTTPSSender 构造一个 HTTP 短信渠道。
func NewHTTPSSender(opts HTTPOptions) (*HTTPSSender, error) {
	if strings.TrimSpace(opts.Endpoint) == "" {
		return nil, fmt.Errorf("notify: sms endpoint is required")
	}
	method := strings.ToUpper(strings.TrimSpace(opts.Method))
	if method == "" {
		method = http.MethodPost
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	name := opts.Name
	if name == "" {
		name = "http"
	}
	return &HTTPSSender{
		name:       name,
		endpoint:   opts.Endpoint,
		method:     method,
		params:     opts.Params,
		headers:    opts.Headers,
		httpClient: &http.Client{Timeout: timeout},
	}, nil
}

// Name 返回渠道标识。
func (s *HTTPSSender) Name() string { return s.name }

// Send 通过 HTTP 发送一条短信。
func (s *HTTPSSender) Send(ctx context.Context, msg Message) error {
	if err := ValidateMessage(msg); err != nil {
		return err
	}
	values := url.Values{}
	for key, template := range s.params {
		values.Set(key, expand(template, msg))
	}
	var req *http.Request
	var err error
	if s.method == http.MethodGet {
		target := s.endpoint
		if encoded := values.Encode(); encoded != "" {
			target += "?" + encoded
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	} else {
		body := values.Encode()
		req, err = http.NewRequestWithContext(ctx, s.method, s.endpoint, strings.NewReader(body))
		if req != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	}
	if err != nil {
		return fmt.Errorf("notify: sms request: %w", err)
	}
	for key, template := range s.headers {
		req.Header.Set(key, expand(template, msg))
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("notify: sms call: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("notify: sms rejected (%d): %s", resp.StatusCode, string(raw))
	}
	// 若网关返回 JSON 且包含明确的失败标记，则视为失败。
	var parsed map[string]any
	if json.Unmarshal(raw, &parsed) == nil {
		if code, ok := parsed["code"]; ok {
			if !isSuccessCode(code) {
				return fmt.Errorf("notify: sms provider returned code %v", code)
			}
		}
	}
	return nil
}

// expand 替换模板中的占位符。
func expand(template string, msg Message) string {
	replacer := strings.NewReplacer(
		"{{to}}", msg.To,
		"{{body}}", msg.Body,
		"{{subject}}", msg.Subject,
	)
	return replacer.Replace(template)
}

// isSuccessCode 判断服务商返回的业务码是否表示成功（0、"0" 或 "OK"）。
func isSuccessCode(code any) bool {
	switch v := code.(type) {
	case float64:
		return v == 0
	case string:
		return v == "0" || strings.EqualFold(v, "ok") || v == ""
	default:
		return true
	}
}

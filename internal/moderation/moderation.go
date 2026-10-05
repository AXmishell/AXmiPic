// Package moderation 提供基于 OpenAI 兼容接口的图片内容审查，用于在图片进入
// 图片广场（公开）之前判定其是否包含违规内容。
package moderation

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Decision 是一次图片审查的结论。
type Decision struct {
	// Allowed 表示图片可以公开到图片广场。
	Allowed bool
	// Reason 是模型返回的原始判定或拒绝原因，便于排查。
	Reason string
}

// Moderator 审查一张图片是否可以公开。
type Moderator interface {
	Review(ctx context.Context, data []byte, mimeType string) (Decision, error)
}

// DefaultPrompt 是默认的审查提示词：要求模型只回答 SAFE 或 UNSAFE。
const DefaultPrompt = "你是图片内容安全审查员。判断这张图片是否包含违规内容" +
	"（色情、暴露、暴力、血腥、恐怖、仇恨、违法、自残、令人不适或其他不适合公开展示的内容）。" +
	"只回答一个词：SAFE 或 UNSAFE。"

// OpenAIConfig 配置标准 OpenAI 兼容的视觉审查接口。
type OpenAIConfig struct {
	// BaseURL 为接口根地址，例如 https://api.openai.com/v1。
	BaseURL string
	// APIKey 为 Bearer 令牌；为空时不带 Authorization。
	APIKey string
	// Model 为视觉模型名，例如 gpt-4o-mini。
	Model string
	// Prompt 覆盖默认审查提示词。
	Prompt string
	// Timeout 为单次请求超时。
	Timeout time.Duration
}

type openAIModerator struct {
	cfg    OpenAIConfig
	client *http.Client
}

// NewOpenAI 构造一个 OpenAI 兼容的审查器。缺省项会被补全为合理默认值。
func NewOpenAI(cfg OpenAIConfig) Moderator {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = "gpt-4o-mini"
	}
	if strings.TrimSpace(cfg.Prompt) == "" {
		cfg.Prompt = DefaultPrompt
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &openAIModerator{cfg: cfg, client: &http.Client{Timeout: cfg.Timeout}}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Review 调用 /chat/completions 审查一张图片。返回 error 表示调用失败（网络、
// 非 200、解析失败等），调用方应按「不通过」处理。
func (m *openAIModerator) Review(ctx context.Context, data []byte, mimeType string) (Decision, error) {
	dataURL := "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
	payload, err := json.Marshal(chatRequest{
		Model:       m.cfg.Model,
		Temperature: 0,
		Messages: []chatMessage{{
			Role: "user",
			Content: []contentPart{
				{Type: "text", Text: m.cfg.Prompt},
				{Type: "image_url", ImageURL: &imageURL{URL: dataURL}},
			},
		}},
	})
	if err != nil {
		return Decision{}, fmt.Errorf("moderation: encode request: %w", err)
	}

	endpoint := strings.TrimRight(m.cfg.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return Decision{}, fmt.Errorf("moderation: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if key := strings.TrimSpace(m.cfg.APIKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return Decision{}, fmt.Errorf("moderation: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Decision{}, fmt.Errorf("moderation: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Decision{}, fmt.Errorf("moderation: status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Decision{}, fmt.Errorf("moderation: decode response: %w", err)
	}
	if parsed.Error != nil {
		return Decision{}, fmt.Errorf("moderation: api error: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		// 没有返回任何内容：按违规处理由调用方负责（Allowed=false）。
		return Decision{Allowed: false, Reason: "no choices returned"}, nil
	}
	return classify(parsed.Choices[0].Message.Content), nil
}

// classify 依据模型回答判定是否安全。为避免误判，要求出现明确的 SAFE 判定才
// 放行；空内容或无法识别的内容一律按违规处理。
func classify(content string) Decision {
	trimmed := strings.TrimSpace(content)
	lower := strings.ToLower(trimmed)
	switch {
	case strings.Contains(lower, "unsafe") || strings.Contains(lower, "violat") ||
		strings.Contains(trimmed, "违规") || strings.Contains(trimmed, "不安全") ||
		strings.Contains(trimmed, "不合规"):
		return Decision{Allowed: false, Reason: trimmed}
	case strings.Contains(lower, "safe") || strings.Contains(trimmed, "安全") ||
		strings.Contains(trimmed, "合规") || strings.Contains(trimmed, "通过"):
		return Decision{Allowed: true, Reason: trimmed}
	default:
		return Decision{Allowed: false, Reason: "unrecognized verdict: " + trimmed}
	}
}

package notify

import (
	"strings"
	"testing"
)

// TestBuildEmailSanitizesHeaders 验证头部字段中的换行不会造成 SMTP 头注入，
// 并且非 ASCII 主题会被编码。
func TestBuildEmailSanitizesHeaders(t *testing.T) {
	body := buildEmail("sender@example.com", Message{
		To:      "victim@example.com\r\nBcc: attacker@example.com",
		Subject: "你好\r\nX-Evil: 1",
		Body:    "hi",
	})
	text := string(body)
	parts := strings.SplitN(text, "\r\n\r\n", 2)
	if len(parts) != 2 {
		t.Fatalf("email has no header/body separator:\n%s", text)
	}
	lines := strings.Split(parts[0], "\r\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 header lines, got %d:\n%s", len(lines), text)
	}
	for _, line := range lines {
		if strings.HasPrefix(line, "Bcc:") || strings.HasPrefix(line, "X-Evil:") {
			t.Fatalf("injected header survived sanitization: %q", line)
		}
	}
	if !strings.Contains(text, "=?UTF-8?") {
		t.Fatalf("non-ASCII subject was not encoded:\n%s", text)
	}
}

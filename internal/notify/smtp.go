package notify

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
)

// SMTPSender 通过 SMTP 发送邮件。它支持 STARTTLS 与隐式 TLS。
type SMTPSender struct {
	host     string
	port     int
	username string
	password string
	from     string
	// useTLS 为 true 时使用隐式 TLS（通常是 465 端口）。
	useTLS bool
}

// SMTPOptions 是构造 SMTPSender 的参数。
type SMTPOptions struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	UseTLS   bool
}

// NewSMTPSender 构造一个 SMTP 邮件渠道。
func NewSMTPSender(opts SMTPOptions) (*SMTPSender, error) {
	if strings.TrimSpace(opts.Host) == "" {
		return nil, fmt.Errorf("notify: smtp host is required")
	}
	port := opts.Port
	if port == 0 {
		port = 587
	}
	from := strings.TrimSpace(opts.From)
	if from == "" {
		from = opts.Username
	}
	if strings.TrimSpace(from) == "" {
		return nil, fmt.Errorf("notify: smtp from is required")
	}
	return &SMTPSender{
		host:     opts.Host,
		port:     port,
		username: opts.Username,
		password: opts.Password,
		from:     from,
		useTLS:   opts.UseTLS,
	}, nil
}

// Name 返回渠道标识。
func (s *SMTPSender) Name() string { return "smtp" }

// Send 发送一封纯文本邮件。
func (s *SMTPSender) Send(ctx context.Context, msg Message) error {
	if err := ValidateMessage(msg); err != nil {
		return err
	}
	addr := net.JoinHostPort(s.host, fmt.Sprintf("%d", s.port))
	var auth smtp.Auth
	if s.username != "" {
		auth = smtp.PlainAuth("", s.username, s.password, s.host)
	}
	body := buildEmail(s.from, msg)
	if s.useTLS {
		return s.sendImplicitTLS(ctx, addr, auth, msg.To, body)
	}
	return s.sendWithStartTLS(ctx, addr, auth, msg.To, body)
}

// sendWithStartTLS 使用 STARTTLS（若服务端支持）发送。
func (s *SMTPSender) sendWithStartTLS(ctx context.Context, addr string, auth smtp.Auth, to string, body []byte) error {
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("notify: smtp dial: %w", err)
	}
	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("notify: smtp client: %w", err)
	}
	defer func() { _ = client.Close() }()
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: s.host}); err != nil {
			return fmt.Errorf("notify: smtp starttls: %w", err)
		}
	}
	return s.deliver(client, auth, to, body)
}

// sendImplicitTLS 使用隐式 TLS 发送。
func (s *SMTPSender) sendImplicitTLS(ctx context.Context, addr string, auth smtp.Auth, to string, body []byte) error {
	dialer := &tls.Dialer{Config: &tls.Config{ServerName: s.host}}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("notify: smtp tls dial: %w", err)
	}
	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("notify: smtp client: %w", err)
	}
	defer func() { _ = client.Close() }()
	return s.deliver(client, auth, to, body)
}

// deliver 完成认证与数据投递。
func (s *SMTPSender) deliver(client *smtp.Client, auth smtp.Auth, to string, body []byte) error {
	if auth != nil {
		if ok, _ := client.Extension("AUTH"); ok {
			if err := client.Auth(auth); err != nil {
				return fmt.Errorf("notify: smtp auth: %w", err)
			}
		}
	}
	if err := client.Mail(s.from); err != nil {
		return fmt.Errorf("notify: smtp mail from: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("notify: smtp rcpt: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("notify: smtp data: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("notify: smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("notify: smtp close: %w", err)
	}
	return client.Quit()
}

// buildEmail 组装一封 UTF-8 纯文本邮件。
func buildEmail(from string, msg Message) []byte {
	subject := msg.Subject
	if subject == "" {
		subject = "AXmiPic 通知"
	}
	headers := []string{
		"From: " + from,
		"To: " + msg.To,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
	}
	return []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + msg.Body + "\r\n")
}

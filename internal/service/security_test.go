package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/notify"
	"github.com/AXmishell/axmipic/internal/security"
	"github.com/AXmishell/axmipic/internal/service"
)

func TestUploadScannerBlocksDangerousContent(t *testing.T) {
	repo := newRepo(t)
	newCustomer(t, repo, "u1")
	upload := service.NewUploadService(repo, managerWithFallback(t, newFakeStorage()), pngPolicy())
	upload.SetScanner(security.NewBlockingScanner([]string{"image/png"}))
	ctx := context.Background()
	u1 := &auth.Principal{UserID: "u1", Role: auth.RoleUser}

	// 合法 PNG 通过。
	if _, err := upload.Upload(ctx, u1, service.UploadInput{Data: testPNGSize(t, 8), MimeType: "image/png"}); err != nil {
		t.Fatalf("Upload valid: %v", err)
	}

	// 声明为 PNG 但内容是 ELF 可执行文件时被拒绝。
	elf := []byte{0x7F, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00}
	if _, err := upload.Upload(ctx, u1, service.UploadInput{Data: elf, MimeType: "image/png"}); !errors.Is(err, service.ErrContentBlocked) {
		t.Fatalf("Upload dangerous err = %v, want ErrContentBlocked", err)
	}
}

func TestNotifyService(t *testing.T) {
	sms := notify.NewMockSender("sms")
	email := notify.NewMockSender("email")
	svc := service.NewNotifyService(sms, email)
	ctx := context.Background()

	if err := svc.SendSMS(ctx, "13800000000", "验证码 0000"); err != nil {
		t.Fatalf("SendSMS: %v", err)
	}
	if len(sms.Sent) != 1 || sms.Sent[0].Body != "验证码 0000" {
		t.Fatalf("sms = %+v", sms.Sent)
	}

	if err := svc.SendEmail(ctx, "user@example.com", "主题", "正文"); err != nil {
		t.Fatalf("SendEmail: %v", err)
	}
	if len(email.Sent) != 1 || email.Sent[0].Subject != "主题" {
		t.Fatalf("email = %+v", email.Sent)
	}

	if err := svc.SendSMS(ctx, "", "x"); !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("empty recipient err = %v, want ErrInvalidInput", err)
	}

	smsName, emailName := svc.Channels()
	if smsName != "mock" || emailName != "mock" {
		t.Fatalf("channels = %q, %q", smsName, emailName)
	}
}

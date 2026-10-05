package service_test

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/notify"
	"github.com/AXmishell/axmipic/internal/secret"
	"github.com/AXmishell/axmipic/internal/service"
)

func newSecurityService(t *testing.T) *service.AccountService {
	t.Helper()
	svc, _ := newAccountService(t, true, 1<<20)
	cipher, err := secret.New([]byte("test-encryption-key"))
	if err != nil {
		t.Fatalf("secret.New: %v", err)
	}
	svc.SetCipher(cipher)
	return svc
}

func TestTOTPLoginFlow(t *testing.T) {
	svc := newSecurityService(t)
	ctx := context.Background()
	user, err := svc.RegisterCustomer(ctx, "totpuser", "password123")
	if err != nil {
		t.Fatalf("RegisterCustomer: %v", err)
	}
	principal := &auth.Principal{UserID: user.ID, Role: auth.RoleUser}

	setup, err := svc.SetupTOTP(ctx, principal)
	if err != nil {
		t.Fatalf("SetupTOTP: %v", err)
	}
	if setup.Secret == "" || setup.URI == "" {
		t.Fatalf("setup = %+v", setup)
	}

	code, err := auth.TOTPCode(setup.Secret, time.Now())
	if err != nil {
		t.Fatalf("TOTPCode: %v", err)
	}
	enabled, err := svc.EnableTOTP(ctx, principal, code)
	if err != nil {
		t.Fatalf("EnableTOTP: %v", err)
	}
	if !enabled.TOTPEnabled {
		t.Fatalf("expected totp enabled: %+v", enabled)
	}
	// 重复启用应报错。
	if _, err := svc.EnableTOTP(ctx, principal, code); !errors.Is(err, service.ErrTOTPAlreadyEnabled) {
		t.Fatalf("enable again err = %v, want ErrTOTPAlreadyEnabled", err)
	}

	// 登录应返回挑战而非会话。
	session, err := svc.LoginCustomer(ctx, "totpuser", "password123")
	if err != nil {
		t.Fatalf("LoginCustomer: %v", err)
	}
	if !session.TOTPRequired || session.Token != "" || session.ChallengeToken == "" {
		t.Fatalf("expected totp challenge, got %+v", session)
	}

	// 用挑战令牌与错误验证码登录失败。
	if _, err := svc.VerifyTOTPLogin(ctx, session.ChallengeToken, "000000"); !errors.Is(err, service.ErrInvalidTOTPCode) {
		t.Fatalf("wrong code err = %v, want ErrInvalidTOTPCode", err)
	}

	code, _ = auth.TOTPCode(setup.Secret, time.Now())
	final, err := svc.VerifyTOTPLogin(ctx, session.ChallengeToken, code)
	if err != nil {
		t.Fatalf("VerifyTOTPLogin: %v", err)
	}
	if final.Token == "" || final.User.ID != user.ID {
		t.Fatalf("final session = %+v", final)
	}

	// 关闭二次验证。
	code, _ = auth.TOTPCode(setup.Secret, time.Now())
	disabled, err := svc.DisableTOTP(ctx, principal, code, "")
	if err != nil {
		t.Fatalf("DisableTOTP: %v", err)
	}
	if disabled.TOTPEnabled {
		t.Fatalf("expected totp disabled: %+v", disabled)
	}

	// 关闭后登录直接返回会话。
	plain, err := svc.LoginCustomer(ctx, "totpuser", "password123")
	if err != nil || plain.TOTPRequired || plain.Token == "" {
		t.Fatalf("plain login = %+v, err = %v", plain, err)
	}
}

func TestEmailBindingFlow(t *testing.T) {
	svc := newSecurityService(t)
	mock := notify.NewMockSender("email")
	svc.SetNotifyService(service.NewNotifyService(nil, mock))

	ctx := context.Background()
	user, err := svc.RegisterCustomer(ctx, "mailuser", "password123")
	if err != nil {
		t.Fatalf("RegisterCustomer: %v", err)
	}
	principal := &auth.Principal{UserID: user.ID, Role: auth.RoleUser}

	if err := svc.SendEmailVerification(ctx, principal, "not-an-email"); !errors.Is(err, service.ErrInvalidEmail) {
		t.Fatalf("invalid email err = %v, want ErrInvalidEmail", err)
	}

	if err := svc.SendEmailVerification(ctx, principal, "User@Example.com"); err != nil {
		t.Fatalf("SendEmailVerification: %v", err)
	}
	if len(mock.Sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(mock.Sent))
	}
	code := extractCode(t, mock.Sent[0].Body)

	// 错误验证码。
	if _, err := svc.VerifyEmail(ctx, principal, "user@example.com", "000000", ""); !errors.Is(err, service.ErrEmailCodeInvalid) {
		t.Fatalf("wrong code err = %v, want ErrEmailCodeInvalid", err)
	}
	// 正确验证码绑定（首次绑定无需密码）。
	updated, err := svc.VerifyEmail(ctx, principal, "user@example.com", code, "")
	if err != nil {
		t.Fatalf("VerifyEmail: %v", err)
	}
	if updated.Email != "user@example.com" || !updated.EmailVerified {
		t.Fatalf("bound user = %+v", updated)
	}
	// 验证码一次性：再次使用应失败。
	if _, err := svc.VerifyEmail(ctx, principal, "user@example.com", code, ""); !errors.Is(err, service.ErrEmailCodeInvalid) {
		t.Fatalf("reuse code err = %v, want ErrEmailCodeInvalid", err)
	}

	// 邮箱已被占用（另一个用户尝试绑定同一邮箱）。
	other, _ := svc.RegisterCustomer(ctx, "otheruser", "password123")
	otherPrincipal := &auth.Principal{UserID: other.ID, Role: auth.RoleUser}
	if err := svc.SendEmailVerification(ctx, otherPrincipal, "user@example.com"); !errors.Is(err, service.ErrEmailInUse) {
		t.Fatalf("duplicate email err = %v, want ErrEmailInUse", err)
	}

	// 解绑需要正确密码。
	if _, err := svc.UnbindEmail(ctx, principal, "wrong-password"); !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("unbind wrong password err = %v, want ErrInvalidCredentials", err)
	}
	unbound, err := svc.UnbindEmail(ctx, principal, "password123")
	if err != nil {
		t.Fatalf("UnbindEmail: %v", err)
	}
	if unbound.Email != "" || unbound.EmailVerified {
		t.Fatalf("unbound user = %+v", unbound)
	}
}

var codePattern = regexp.MustCompile(`\b(\d{6})\b`)

func extractCode(t *testing.T, body string) string {
	t.Helper()
	match := codePattern.FindStringSubmatch(body)
	if len(match) < 2 {
		t.Fatalf("no code found in email body: %q", body)
	}
	return match[1]
}

func TestEmailRebindPolicy(t *testing.T) {
	svc := newSecurityService(t)
	mock := notify.NewMockSender("email")
	svc.SetNotifyService(service.NewNotifyService(nil, mock))

	ctx := context.Background()
	user, err := svc.RegisterCustomer(ctx, "rebinduser", "password123")
	if err != nil {
		t.Fatalf("RegisterCustomer: %v", err)
	}
	principal := &auth.Principal{UserID: user.ID, Role: auth.RoleUser}

	// 绑定初始邮箱 A（首次绑定无需密码）。
	if err := svc.SendEmailVerification(ctx, principal, "old@example.com"); err != nil {
		t.Fatalf("send A: %v", err)
	}
	codeA := extractCode(t, mock.Sent[len(mock.Sent)-1].Body)
	if _, err := svc.VerifyEmail(ctx, principal, "old@example.com", codeA, ""); err != nil {
		t.Fatalf("bind A: %v", err)
	}

	// 请求换绑到 B。
	if err := svc.SendEmailVerification(ctx, principal, "new@example.com"); err != nil {
		t.Fatalf("send B: %v", err)
	}
	codeB := extractCode(t, mock.Sent[len(mock.Sent)-1].Body)

	// 换绑必须提供正确密码。
	if _, err := svc.VerifyEmail(ctx, principal, "new@example.com", codeB, ""); !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("change without password err = %v, want ErrInvalidCredentials", err)
	}
	if _, err := svc.VerifyEmail(ctx, principal, "new@example.com", codeB, "wrong-password"); !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("change wrong password err = %v, want ErrInvalidCredentials", err)
	}
	updated, err := svc.VerifyEmail(ctx, principal, "new@example.com", codeB, "password123")
	if err != nil {
		t.Fatalf("change with password: %v", err)
	}
	if updated.Email != "new@example.com" || !updated.EmailVerified {
		t.Fatalf("rebound user = %+v", updated)
	}

	// 旧邮箱应收到变更通知。
	notified := false
	for _, msg := range mock.Sent {
		if msg.To == "old@example.com" && strings.Contains(msg.Subject, "变更") {
			notified = true
		}
	}
	if !notified {
		t.Fatalf("old email did not receive a change notice: %+v", mock.Sent)
	}

	// 发码频率限制：突发额度耗尽后应被拒绝。
	if err := svc.SendEmailVerification(ctx, principal, "third@example.com"); !errors.Is(err, service.ErrEmailRateLimited) {
		t.Fatalf("rate limit err = %v, want ErrEmailRateLimited", err)
	}
}

func TestChangePassword(t *testing.T) {
	svc := newSecurityService(t)
	ctx := context.Background()
	user, err := svc.RegisterCustomer(ctx, "pwuser", "password123")
	if err != nil {
		t.Fatalf("RegisterCustomer: %v", err)
	}
	principal := &auth.Principal{UserID: user.ID, Role: auth.RoleUser}

	// 当前密码错误。
	if err := svc.ChangePassword(ctx, principal, "wrong-password", "newpassword123"); !errors.Is(err, service.ErrInvalidPassword) {
		t.Fatalf("wrong current err = %v, want ErrInvalidPassword", err)
	}
	// 新密码过短。
	if err := svc.ChangePassword(ctx, principal, "password123", "short"); !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("short new err = %v, want ErrInvalidInput", err)
	}
	// 新密码与旧密码相同。
	if err := svc.ChangePassword(ctx, principal, "password123", "password123"); !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("same new err = %v, want ErrInvalidInput", err)
	}
	// 正常修改。
	if err := svc.ChangePassword(ctx, principal, "password123", "newpassword123"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if _, err := svc.LoginCustomer(ctx, "pwuser", "password123"); !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("old password login err = %v, want ErrInvalidCredentials", err)
	}
	if _, err := svc.LoginCustomer(ctx, "pwuser", "newpassword123"); err != nil {
		t.Fatalf("new password login: %v", err)
	}
}

func TestPasswordResetFlow(t *testing.T) {
	svc := newSecurityService(t)
	mock := notify.NewMockSender("email")
	svc.SetNotifyService(service.NewNotifyService(nil, mock))

	ctx := context.Background()
	user, err := svc.RegisterCustomer(ctx, "resetuser", "password123")
	if err != nil {
		t.Fatalf("RegisterCustomer: %v", err)
	}
	principal := &auth.Principal{UserID: user.ID, Role: auth.RoleUser}

	// 绑定并验证邮箱。
	if err := svc.SendEmailVerification(ctx, principal, "reset@example.com"); err != nil {
		t.Fatalf("SendEmailVerification: %v", err)
	}
	bindCode := extractCode(t, mock.Sent[len(mock.Sent)-1].Body)
	if _, err := svc.VerifyEmail(ctx, principal, "reset@example.com", bindCode, ""); err != nil {
		t.Fatalf("VerifyEmail: %v", err)
	}

	// 未注册的邮箱：不报错也不发信，避免枚举。
	before := len(mock.Sent)
	if err := svc.SendPasswordResetCode(ctx, "nobody@example.com"); err != nil {
		t.Fatalf("unknown email send err = %v, want nil", err)
	}
	if len(mock.Sent) != before {
		t.Fatalf("unknown email should not send, sent=%d", len(mock.Sent)-before)
	}

	// 请求重置验证码。
	if err := svc.SendPasswordResetCode(ctx, "reset@example.com"); err != nil {
		t.Fatalf("SendPasswordResetCode: %v", err)
	}
	resetCode := extractCode(t, mock.Sent[len(mock.Sent)-1].Body)

	// 错误验证码。
	if err := svc.ResetPassword(ctx, "reset@example.com", "000000", "brandnew123"); !errors.Is(err, service.ErrEmailCodeInvalid) {
		t.Fatalf("wrong reset code err = %v, want ErrEmailCodeInvalid", err)
	}
	// 未注册邮箱同样返回 ErrEmailCodeInvalid（不暴露是否存在）。
	if err := svc.ResetPassword(ctx, "nobody@example.com", resetCode, "brandnew123"); !errors.Is(err, service.ErrEmailCodeInvalid) {
		t.Fatalf("unknown email reset err = %v, want ErrEmailCodeInvalid", err)
	}
	// 正常重置。
	if err := svc.ResetPassword(ctx, "reset@example.com", resetCode, "brandnew123"); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	if _, err := svc.LoginCustomer(ctx, "resetuser", "brandnew123"); err != nil {
		t.Fatalf("login with reset password: %v", err)
	}
	// 验证码一次性。
	if err := svc.ResetPassword(ctx, "reset@example.com", resetCode, "another12345"); !errors.Is(err, service.ErrEmailCodeInvalid) {
		t.Fatalf("reuse reset code err = %v, want ErrEmailCodeInvalid", err)
	}
}

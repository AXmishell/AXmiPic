package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/mail"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/secret"
	"github.com/AXmishell/axmipic/internal/store"
)

// 账户服务返回的错误。
var (
	// ErrUserExists 表示用户名已被占用。
	ErrUserExists = errors.New("service: username already exists")
	// ErrInvalidCredentials 表示用户名或密码错误。
	ErrInvalidCredentials = errors.New("service: invalid username or password")
	// ErrInvalidPassword 表示当前密码不正确（用于已登录状态下的改密/校验）。
	ErrInvalidPassword = errors.New("service: current password is incorrect")
	// ErrRegistrationDisabled 表示注册功能已关闭。
	ErrRegistrationDisabled = errors.New("service: registration is disabled")
	// ErrTokenNotFound 表示某个 API 令牌不存在。
	ErrTokenNotFound = errors.New("service: token not found")
	// ErrInvalidChallenge 表示 TOTP 登录挑战令牌无效或已过期。
	ErrInvalidChallenge = errors.New("service: invalid or expired login challenge")
	// ErrTOTPUnavailable 表示服务未配置加密主密钥，无法启用 TOTP。
	ErrTOTPUnavailable = errors.New("service: totp is unavailable")
	// ErrTOTPAlreadyEnabled 表示二次验证已经处于启用状态。
	ErrTOTPAlreadyEnabled = errors.New("service: totp is already enabled")
	// ErrTOTPNotEnabled 表示二次验证尚未启用。
	ErrTOTPNotEnabled = errors.New("service: totp is not enabled")
	// ErrTOTPNotConfigured 表示尚未生成 TOTP 密钥。
	ErrTOTPNotConfigured = errors.New("service: totp has not been set up")
	// ErrInvalidTOTPCode 表示动态验证码错误。
	ErrInvalidTOTPCode = errors.New("service: invalid totp code")
	// ErrInvalidEmail 表示邮箱地址格式不合法。
	ErrInvalidEmail = errors.New("service: invalid email address")
	// ErrEmailInUse 表示邮箱已被其他账户绑定。
	ErrEmailInUse = errors.New("service: email already in use")
	// ErrEmailNotConfigured 表示邮件渠道未配置，无法发送验证码。
	ErrEmailNotConfigured = errors.New("service: email channel is not configured")
	// ErrEmailCodeInvalid 表示邮箱验证码错误或已过期。
	ErrEmailCodeInvalid = errors.New("service: invalid or expired email verification code")
	// ErrEmailRateLimited 表示邮箱验证码请求过于频繁。
	ErrEmailRateLimited = errors.New("service: too many email verification requests")
)

// emailCodeTTL 是邮箱验证码的有效期。
const emailCodeTTL = 10 * time.Minute

// emailCodeMaxAttempts 是单个验证码允许的最大尝试次数。
const emailCodeMaxAttempts = 5

// 邮箱验证码发送频率限制：每个账户每分钟至多 1 条（突发 2 条），每天至多 10 条。
const (
	emailCodePerMinute = 1
	emailCodeBurst     = 2
	emailCodeDailyMax  = 10
)

const (
	minUsernameLength = 3
	maxUsernameLength = 64
	minPasswordLength = 8
	// maxPasswordLength 与 bcrypt 的 72 字节输入上限保持一致；更长的输入会被
	// 拒绝，而不是作为内部错误暴露出来。
	maxPasswordLength = 72
)

// dummyPasswordHash 会在用户名不存在时用于比对，这样登录耗时就不会泄露
// 某个账户是否已注册。
var dummyPasswordHash = func() string {
	hash, err := auth.HashPassword("axmipic-nonexistent-account")
	if err != nil {
		return ""
	}
	return hash
}()

// UserDTO 是账户在 API 中的表示形式。
type UserDTO struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	Role       string `json:"role"`
	Disabled   bool   `json:"disabled"`
	UsedBytes  int64  `json:"used_bytes"`
	QuotaBytes int64  `json:"quota_bytes"`
	// RoleGroupID 是普通用户所属的角色组；管理员与未分配用户为空。
	RoleGroupID string `json:"role_group_id,omitempty"`
	// PlanExpiresAt 为已购套餐的到期时间；为空表示无套餐或永久有效。
	PlanExpiresAt *time.Time `json:"plan_expires_at,omitempty"`
	// Email 为已绑定的邮箱；EmailVerified 表示是否已通过验证。
	Email         string    `json:"email,omitempty"`
	EmailVerified bool      `json:"email_verified"`
	TOTPEnabled   bool      `json:"totp_enabled"`
	CreatedAt     time.Time `json:"created_at"`
}

// SessionDTO 由登录操作返回。当账号启用了 TOTP 时，Token 为空且
// TOTPRequired 为真，调用方需用 ChallengeToken 完成二次验证。
type SessionDTO struct {
	Token     string    `json:"token,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	User      UserDTO   `json:"user"`
	// TOTPRequired 表示还需完成 TOTP 二次验证。
	TOTPRequired bool `json:"totp_required,omitempty"`
	// ChallengeToken 为完成 TOTP 验证所需的短期令牌。
	ChallengeToken string `json:"challenge_token,omitempty"`
}

// TOTPSetupDTO 是开始配置 TOTP 时返回的信息。
type TOTPSetupDTO struct {
	// Secret 为 Base32 编码的密钥，供手动录入。
	Secret string `json:"secret"`
	// URI 为 otpauth:// 链接，供 Authenticator 扫描。
	URI string `json:"uri"`
}

// SecurityDTO 汇总账户的安全设置状态。
type SecurityDTO struct {
	Email          string `json:"email"`
	EmailVerified  bool   `json:"email_verified"`
	TOTPEnabled    bool   `json:"totp_enabled"`
	TOTPAvailable  bool   `json:"totp_available"`
	EmailAvailable bool   `json:"email_available"`
}

// TokenDTO 是 API 令牌在 API 中的表示形式。明文 Token 仅在令牌创建时
// 填充。
type TokenDTO struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Token      string     `json:"token,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// AccountService 管理账户、会话和 API 令牌。
type AccountService struct {
	repo              *store.Repository
	issuer            *auth.SessionIssuer
	allowRegistration atomic.Bool
	defaultQuotaBytes int64
	policies          *PolicyService
	cipher            *secret.Cipher
	notify            *NotifyService

	// emailLimiter 限制单账户的验证码发送频率（每分钟/突发）。每日上限与
	// 验证码本身持久化在数据库中，可跨重启与多实例生效。
	emailLimiter *auth.RateLimiter
}

// NewAccountService 构造一个 AccountService。
func NewAccountService(repo *store.Repository, issuer *auth.SessionIssuer, allowRegistration bool, defaultQuotaBytes int64) *AccountService {
	svc := &AccountService{
		repo:              repo,
		issuer:            issuer,
		defaultQuotaBytes: defaultQuotaBytes,
		emailLimiter:      auth.NewRateLimiter(emailCodePerMinute, emailCodeBurst),
	}
	svc.allowRegistration.Store(allowRegistration)
	return svc
}

// AllowRegistration 报告当前是否开放注册。
func (s *AccountService) AllowRegistration() bool {
	return s.allowRegistration.Load()
}

// SetAllowRegistration 在线切换是否开放注册；下次注册请求即生效。
func (s *AccountService) SetAllowRegistration(allow bool) {
	s.allowRegistration.Store(allow)
}

// SetCipher 安装加密主密钥，用于加密保存 TOTP 密钥。未安装时 TOTP 不可用。
func (s *AccountService) SetCipher(cipher *secret.Cipher) {
	s.cipher = cipher
}

// SetNotifyService 安装通知服务，用于发送邮箱验证码。
func (s *AccountService) SetNotifyService(notifySvc *NotifyService) {
	s.notify = notifySvc
}

// SetPolicyService 安装角色组/策略服务，使注册与账户视图能够使用策略。
func (s *AccountService) SetPolicyService(policies *PolicyService) {
	s.policies = policies
}

// registrationPolicy 解析新注册用户应加入的角色组及其配额。未配置策略服务
// 或解析失败时回退到配置的默认配额。
func (s *AccountService) registrationPolicy(ctx context.Context) (*string, int64) {
	if s.policies == nil {
		return nil, s.defaultQuotaBytes
	}
	effective, err := s.policies.ResolveDefault(ctx)
	if err != nil {
		return nil, s.defaultQuotaBytes
	}
	if effective.RoleGroupID == "" {
		return nil, effective.QuotaBytes
	}
	id := effective.RoleGroupID
	return &id, effective.QuotaBytes
}

// RegisterCustomer 创建一个新的普通（客户）账户。它是内部/测试用的低级入口，
// 不要求邮箱。面向 API 的注册应使用 RegisterCustomerWithEmail。
func (s *AccountService) RegisterCustomer(ctx context.Context, username, password string) (*UserDTO, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if err := validateCredentials(username, password); err != nil {
		return nil, err
	}
	if !s.AllowRegistration() {
		return nil, ErrRegistrationDisabled
	}
	return s.createCustomer(ctx, username, password, "", false)
}

// createCustomer 在完成占用检查后写入一个新客户账户。email 非空时会写入并
// 按 emailVerified 标记验证状态。
func (s *AccountService) createCustomer(ctx context.Context, username, password, email string, emailVerified bool) (*UserDTO, error) {
	if err := s.ensureLoginIdentifiersFree(ctx, store.RoleCustomer, username, email, ""); err != nil {
		return nil, err
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, err
	}
	roleGroupID, quotaBytes := s.registrationPolicy(ctx)
	customer := &store.Customer{
		ID:            uuid.NewString(),
		Username:      username,
		PasswordHash:  hash,
		QuotaBytes:    quotaBytes,
		RoleGroupID:   roleGroupID,
		Email:         email,
		EmailVerified: emailVerified,
	}
	if err := s.repo.CreateCustomer(ctx, customer); err != nil {
		// 并发的注册可能已经先插入了相同的用户名。
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, ErrUserExists
		}
		return nil, fmt.Errorf("register: create user: %w", err)
	}
	return toUserDTO(accountFromCustomer(customer)), nil
}

// ensureLoginIdentifiersFree 校验用户名与邮箱在给定账户表内不与任何账户的
// 用户名或邮箱冲突。登录时以同一字符串先查用户名再查邮箱，因此若用户名与
// 他人邮箱相同（反之亦然）就会产生登录歧义，必须在此拒绝。excludeID 用于
// 排除当前账户自身（改绑邮箱时）。
func (s *AccountService) ensureLoginIdentifiersFree(ctx context.Context, role store.AccountRole, username, email, excludeID string) error {
	if username != "" {
		if existing, err := s.repo.GetAccountByUsername(ctx, role, username); err == nil {
			if existing.ID != excludeID {
				return ErrUserExists
			}
		} else if !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("check username: %w", err)
		}
		// 用户名不能等于他人已绑定的邮箱。
		if existing, err := s.repo.GetAccountByEmail(ctx, role, username); err == nil {
			if existing.ID != excludeID {
				return fmt.Errorf("%w: username conflicts with an existing email", ErrUserExists)
			}
		} else if !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("check username as email: %w", err)
		}
	}
	if email != "" {
		inUse, err := s.repo.EmailInUse(ctx, email, excludeID)
		if err != nil {
			return err
		}
		if inUse {
			return ErrEmailInUse
		}
		// 邮箱不能等于他人用户名。
		if existing, err := s.repo.GetAccountByUsername(ctx, role, email); err == nil {
			if existing.ID != excludeID {
				return fmt.Errorf("%w: email conflicts with an existing username", ErrEmailInUse)
			}
		} else if !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("check email as username: %w", err)
		}
	}
	return nil
}

// SendRegistrationCode 向邮箱发送注册验证码（无需登录）。邮箱已被占用时返回
// ErrEmailInUse，便于前端即时提示。
func (s *AccountService) SendRegistrationCode(ctx context.Context, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if _, err := mail.ParseAddress(email); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidEmail, email)
	}
	if s.notify == nil {
		return ErrEmailNotConfigured
	}
	key := registrationKey(email)
	if !s.allowEmailSendForKey(ctx, key) {
		return ErrEmailRateLimited
	}
	inUse, err := s.repo.EmailInUse(ctx, email, "")
	if err != nil {
		return err
	}
	if inUse {
		return ErrEmailInUse
	}
	// 邮箱也不能已被他人用作登录用户名。
	if existing, err := s.repo.GetAccountByUsername(ctx, store.RoleCustomer, email); err == nil && existing.ID != "" {
		return ErrEmailInUse
	} else if err != nil && !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("check email as username: %w", err)
	}
	code, err := generateNumericCode()
	if err != nil {
		return err
	}
	body := fmt.Sprintf("你的 AXmiPic 注册验证码是：%s\n\n验证码 %d 分钟内有效，请勿转发给他人。", code, int(emailCodeTTL.Minutes()))
	if err := s.notify.SendEmail(ctx, email, "AXmiPic 注册验证码", body); err != nil {
		return err
	}
	if err := s.storeEmailCodeForKey(ctx, key, email, code); err != nil {
		return err
	}
	s.recordEmailSendForKey(ctx, key)
	return nil
}

// RegisterCustomerWithEmail 校验邮箱验证码后创建普通用户账户并标记邮箱已验证。
// 用户名或邮箱被占用时分别返回 ErrUserExists / ErrEmailInUse。
func (s *AccountService) RegisterCustomerWithEmail(ctx context.Context, username, password, email, code string) (*UserDTO, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	email = strings.ToLower(strings.TrimSpace(email))
	if err := validateCredentials(username, password); err != nil {
		return nil, err
	}
	if !s.AllowRegistration() {
		return nil, ErrRegistrationDisabled
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidEmail, email)
	}
	// 先做占用检查（用户名/邮箱互不冲突且各自唯一），避免因冲突消耗验证码。
	if err := s.ensureLoginIdentifiersFree(ctx, store.RoleCustomer, username, email, ""); err != nil {
		return nil, err
	}
	if !s.consumeEmailCodeForKey(ctx, registrationKey(email), email, code) {
		return nil, ErrEmailCodeInvalid
	}
	return s.createCustomer(ctx, username, password, email, true)
}

// registrationKey 返回注册验证码的存储键（按邮箱维度）。
func registrationKey(email string) string {
	return "register:" + strings.ToLower(strings.TrimSpace(email))
}

// GuestUsername 是内置访客账户的用户名。
const GuestUsername = "guest"

// EnsureGuestAccount 确保存在一个低权 Guest 账户，并把它归入 guestRoleGroupID。
// 当 allowGuestUpload 为 true 时，未登录访客将以该账户的身份上传。它返回创建
// 的账户（已存在时返回 nil）。密码被设为随机值，因此该账户不能通过登录进入。
func (s *AccountService) EnsureGuestAccount(ctx context.Context, guestRoleGroupID string, quotaBytes int64) (*store.Customer, error) {
	if existing, err := s.repo.GetAccountByUsername(ctx, store.RoleCustomer, GuestUsername); err == nil {
		if !existing.IsGuest {
			_ = s.repo.MarkCustomerGuest(ctx, existing.ID, true)
		}
		return nil, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("ensure guest: lookup: %w", err)
	}
	randomPassword, _, _, err := auth.GenerateAPIToken()
	if err != nil {
		return nil, fmt.Errorf("ensure guest: generate password: %w", err)
	}
	hash, err := auth.HashPassword(randomPassword)
	if err != nil {
		return nil, fmt.Errorf("ensure guest: hash password: %w", err)
	}
	var roleGroupID *string
	if strings.TrimSpace(guestRoleGroupID) != "" {
		roleGroupID = &guestRoleGroupID
	}
	customer := &store.Customer{
		ID:           uuid.NewString(),
		Username:     GuestUsername,
		PasswordHash: hash,
		QuotaBytes:   quotaBytes,
		RoleGroupID:  roleGroupID,
		IsGuest:      true,
	}
	if err := s.repo.CreateCustomer(ctx, customer); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, nil
		}
		return nil, fmt.Errorf("ensure guest: create: %w", err)
	}
	return customer, nil
}

// CreateGuestVisitor 为单个匿名访客创建一个独立的 Guest 账户，使用 Guest 角色组
// 的配额，并返回账户 id。调用方（API 层）负责把该 id 写入签名 cookie。
func (s *AccountService) CreateGuestVisitor(ctx context.Context) (string, error) {
	quotaBytes := s.defaultQuotaBytes
	var roleGroupID *string
	if s.policies != nil {
		if effective, err := s.policies.ResolveGuest(ctx); err == nil {
			quotaBytes = effective.QuotaBytes
			if effective.RoleGroupID != "" {
				id := effective.RoleGroupID
				roleGroupID = &id
			}
		}
	}
	randomPassword, _, _, err := auth.GenerateAPIToken()
	if err != nil {
		return "", fmt.Errorf("guest visitor: generate password: %w", err)
	}
	hash, err := auth.HashPassword(randomPassword)
	if err != nil {
		return "", fmt.Errorf("guest visitor: hash password: %w", err)
	}
	customer := &store.Customer{
		ID:           uuid.NewString(),
		Username:     GuestUsername + "_" + uuid.NewString()[:12],
		PasswordHash: hash,
		QuotaBytes:   quotaBytes,
		RoleGroupID:  roleGroupID,
		IsGuest:      true,
	}
	if err := s.repo.CreateCustomer(ctx, customer); err != nil {
		return "", fmt.Errorf("guest visitor: create: %w", err)
	}
	return customer.ID, nil
}

// GuestVisitorExists 报告给定 id 是否为一个现存的内置访客账户。用于在 cookie
// 被伪造或账户已清理时拒绝复用身份。
func (s *AccountService) GuestVisitorExists(ctx context.Context, id string) bool {
	if strings.TrimSpace(id) == "" {
		return false
	}
	account, err := s.repo.GetAccountByID(ctx, store.RoleCustomer, id)
	if err != nil {
		return false
	}
	return account.IsGuest
}

// GuestID 返回内置访客账户的 id；账户不存在时返回空字符串。
func (s *AccountService) GuestID(ctx context.Context) string {
	account, err := s.repo.GetAccountByUsername(ctx, store.RoleCustomer, GuestUsername)
	if err != nil {
		return ""
	}
	return account.ID
}

// RegisterAdmin 在 admins 表中创建一个管理员账户。
func (s *AccountService) RegisterAdmin(ctx context.Context, username, password string) (*UserDTO, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if err := validateCredentials(username, password); err != nil {
		return nil, err
	}
	if err := s.ensureLoginIdentifiersFree(ctx, store.RoleAdmin, username, "", ""); err != nil {
		return nil, err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, err
	}
	admin := &store.Admin{
		ID:           uuid.NewString(),
		Username:     username,
		PasswordHash: hash,
	}
	if err := s.repo.CreateAdmin(ctx, admin); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, ErrUserExists
		}
		return nil, fmt.Errorf("register admin: create admin: %w", err)
	}
	return toUserDTO(accountFromAdmin(admin)), nil
}

// LoginCustomer 针对 customers 表校验凭据，并签发一个会话令牌。
func (s *AccountService) LoginCustomer(ctx context.Context, username, password string) (*SessionDTO, error) {
	return s.login(ctx, store.RoleCustomer, username, password)
}

// LoginAdmin 针对 admins 表校验凭据，并签发一个会话令牌。
func (s *AccountService) LoginAdmin(ctx context.Context, username, password string) (*SessionDTO, error) {
	return s.login(ctx, store.RoleAdmin, username, password)
}

func (s *AccountService) login(ctx context.Context, role store.AccountRole, username, password string) (*SessionDTO, error) {
	identifier := strings.ToLower(strings.TrimSpace(username))
	account, err := s.repo.GetAccountByUsername(ctx, role, identifier)
	if err != nil && errors.Is(err, store.ErrNotFound) {
		// 用户名不存在时回退到邮箱登录。
		account, err = s.repo.GetAccountByEmail(ctx, role, identifier)
	}
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// 执行一次哑比对，使账户不存在与密码错误两种情况耗时相近，
			// 从而防止用户名枚举。
			auth.VerifyPassword(dummyPasswordHash, password)
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("login: lookup account: %w", err)
	}
	if !auth.VerifyPassword(account.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}
	if account.Disabled {
		return nil, ErrInvalidCredentials
	}
	if account.TOTPEnabled && account.TOTPSecret != "" {
		challenge, expiresAt, err := s.issuer.IssueChallenge(account.ID, string(account.Role))
		if err != nil {
			return nil, err
		}
		return &SessionDTO{TOTPRequired: true, ChallengeToken: challenge, ExpiresAt: expiresAt, User: *toUserDTO(account)}, nil
	}
	token, expiresAt, err := s.issuer.Issue(account.ID, string(account.Role))
	if err != nil {
		return nil, err
	}
	return &SessionDTO{Token: token, ExpiresAt: expiresAt, User: *toUserDTO(account)}, nil
}

// VerifyTOTPLogin 依据登录挑战令牌与动态码签发正式会话。
func (s *AccountService) VerifyTOTPLogin(ctx context.Context, challengeToken, code string) (*SessionDTO, error) {
	userID, role, err := s.issuer.ParseChallenge(strings.TrimSpace(challengeToken))
	if err != nil {
		return nil, ErrInvalidChallenge
	}
	account, err := s.repo.GetAccountByID(ctx, store.AccountRole(role), userID)
	if err != nil {
		return nil, ErrInvalidChallenge
	}
	if account.Disabled || !account.TOTPEnabled || account.TOTPSecret == "" {
		return nil, ErrInvalidChallenge
	}
	secret, err := s.decryptTOTPSecret(account.TOTPSecret)
	if err != nil {
		return nil, ErrInvalidChallenge
	}
	if !auth.VerifyTOTP(secret, code, time.Now()) {
		return nil, ErrInvalidTOTPCode
	}
	token, expiresAt, err := s.issuer.Issue(account.ID, string(account.Role))
	if err != nil {
		return nil, err
	}
	return &SessionDTO{Token: token, ExpiresAt: expiresAt, User: *toUserDTO(account)}, nil
}

// Security 返回账户当前的安全设置状态。
func (s *AccountService) Security(ctx context.Context, principal *auth.Principal) (*SecurityDTO, error) {
	account, err := s.repo.GetAccountByID(ctx, principal.StoreRole(), principal.UserID)
	if err != nil {
		return nil, fmt.Errorf("security: %w", err)
	}
	return &SecurityDTO{
		Email:          account.Email,
		EmailVerified:  account.EmailVerified,
		TOTPEnabled:    account.TOTPEnabled,
		TOTPAvailable:  s.cipher != nil,
		EmailAvailable: s.notify != nil,
	}, nil
}

// SetupTOTP 生成新的 TOTP 密钥并暂存（尚未启用），返回密钥与 otpauth 链接。
// 已启用时返回 ErrTOTPAlreadyEnabled。
func (s *AccountService) SetupTOTP(ctx context.Context, principal *auth.Principal) (*TOTPSetupDTO, error) {
	if s.cipher == nil {
		return nil, ErrTOTPUnavailable
	}
	account, err := s.repo.GetAccountByID(ctx, principal.StoreRole(), principal.UserID)
	if err != nil {
		return nil, fmt.Errorf("setup totp: %w", err)
	}
	if account.TOTPEnabled {
		return nil, ErrTOTPAlreadyEnabled
	}
	secret, err := auth.GenerateTOTPSecret()
	if err != nil {
		return nil, err
	}
	encrypted, err := s.cipher.Encrypt(secret)
	if err != nil {
		return nil, fmt.Errorf("setup totp: encrypt: %w", err)
	}
	if _, err := s.repo.UpdateAccountSecurity(ctx, principal.StoreRole(), principal.UserID, map[string]any{
		"totp_secret":  encrypted,
		"totp_enabled": false,
	}); err != nil {
		return nil, err
	}
	return &TOTPSetupDTO{Secret: secret, URI: auth.OTPAuthURI("AXmiPic", account.Username, secret)}, nil
}

// EnableTOTP 校验动态码后启用二次验证。
func (s *AccountService) EnableTOTP(ctx context.Context, principal *auth.Principal, code string) (*UserDTO, error) {
	if s.cipher == nil {
		return nil, ErrTOTPUnavailable
	}
	account, err := s.repo.GetAccountByID(ctx, principal.StoreRole(), principal.UserID)
	if err != nil {
		return nil, fmt.Errorf("enable totp: %w", err)
	}
	if account.TOTPEnabled {
		return nil, ErrTOTPAlreadyEnabled
	}
	if account.TOTPSecret == "" {
		return nil, ErrTOTPNotConfigured
	}
	secret, err := s.decryptTOTPSecret(account.TOTPSecret)
	if err != nil {
		return nil, ErrTOTPNotConfigured
	}
	if !auth.VerifyTOTP(secret, code, time.Now()) {
		return nil, ErrInvalidTOTPCode
	}
	updated, err := s.repo.UpdateAccountSecurity(ctx, principal.StoreRole(), principal.UserID, map[string]any{
		"totp_enabled": true,
	})
	if err != nil {
		return nil, err
	}
	return toUserDTO(updated), nil
}

// DisableTOTP 关闭二次验证。需要提供当前密码或有效的动态码之一。
func (s *AccountService) DisableTOTP(ctx context.Context, principal *auth.Principal, code, password string) (*UserDTO, error) {
	account, err := s.repo.GetAccountByID(ctx, principal.StoreRole(), principal.UserID)
	if err != nil {
		return nil, fmt.Errorf("disable totp: %w", err)
	}
	if !account.TOTPEnabled {
		return nil, ErrTOTPNotEnabled
	}
	verified := false
	if strings.TrimSpace(code) != "" {
		if secret, err := s.decryptTOTPSecret(account.TOTPSecret); err == nil {
			verified = auth.VerifyTOTP(secret, code, time.Now())
		}
	} else if strings.TrimSpace(password) != "" {
		verified = auth.VerifyPassword(account.PasswordHash, password)
	}
	if !verified {
		return nil, ErrInvalidTOTPCode
	}
	updated, err := s.repo.UpdateAccountSecurity(ctx, principal.StoreRole(), principal.UserID, map[string]any{
		"totp_secret":  "",
		"totp_enabled": false,
	})
	if err != nil {
		return nil, err
	}
	return toUserDTO(updated), nil
}

// SendEmailVerification 向目标邮箱发送 6 位验证码。验证通过前不会改动账户邮箱。
// 发送受频率限制：每分钟至多 1 条（突发 2 条），每天至多 10 条。
func (s *AccountService) SendEmailVerification(ctx context.Context, principal *auth.Principal, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if _, err := mail.ParseAddress(email); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidEmail, email)
	}
	if s.notify == nil {
		return ErrEmailNotConfigured
	}
	if !s.allowEmailSendForKey(ctx, emailCodeKey(principal)) {
		return ErrEmailRateLimited
	}
	if err := s.ensureLoginIdentifiersFree(ctx, principal.StoreRole(), "", email, principal.UserID); err != nil {
		return err
	}
	code, err := generateNumericCode()
	if err != nil {
		return err
	}
	body := fmt.Sprintf("你的 AXmiPic 邮箱验证码是：%s\n\n验证码 %d 分钟内有效，请勿转发给他人。", code, int(emailCodeTTL.Minutes()))
	if err := s.notify.SendEmail(ctx, email, "AXmiPic 邮箱验证码", body); err != nil {
		return err
	}
	if err := s.storeEmailCodeForKey(ctx, emailCodeKey(principal), email, code); err != nil {
		return err
	}
	s.recordEmailSendForKey(ctx, emailCodeKey(principal))
	return nil
}

// VerifyEmail 校验验证码并（换）绑定邮箱。若账户此前已绑定邮箱，则换绑必须
// 提供当前密码；换绑成功后会向旧邮箱发送一条变更通知。
func (s *AccountService) VerifyEmail(ctx context.Context, principal *auth.Principal, email, code, password string) (*UserDTO, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	account, err := s.repo.GetAccountByID(ctx, principal.StoreRole(), principal.UserID)
	if err != nil {
		return nil, fmt.Errorf("verify email: %w", err)
	}
	// 换绑到不同的邮箱时才要求当前密码，防止会话被盗后静默改绑。
	changing := account.EmailVerified && strings.TrimSpace(account.Email) != "" &&
		!strings.EqualFold(account.Email, email)
	if changing {
		if strings.TrimSpace(password) == "" {
			return nil, fmt.Errorf("%w: password is required to change the bound email", ErrInvalidCredentials)
		}
		if !auth.VerifyPassword(account.PasswordHash, password) {
			return nil, ErrInvalidCredentials
		}
	}
	if !s.consumeEmailCodeForKey(ctx, emailCodeKey(principal), email, code) {
		return nil, ErrEmailCodeInvalid
	}
	// 在写入前复查邮箱是否已被其他账户占用或用作他人用户名，缩小竞态窗口。
	if err := s.ensureLoginIdentifiersFree(ctx, principal.StoreRole(), "", email, principal.UserID); err != nil {
		return nil, err
	}
	updated, err := s.repo.UpdateAccountSecurity(ctx, principal.StoreRole(), principal.UserID, map[string]any{
		"email":          email,
		"email_verified": true,
	})
	if err != nil {
		return nil, err
	}
	// 换绑成功后通知旧邮箱，便于用户察觉异常变更。发送失败不影响结果。
	if changing && !strings.EqualFold(account.Email, email) {
		s.notifyEmailChange(account.Email, email)
	}
	return toUserDTO(updated), nil
}

// notifyEmailChange 向旧邮箱发送一条变更通知（尽力而为）。
func (s *AccountService) notifyEmailChange(oldEmail, newEmail string) {
	if s.notify == nil || strings.TrimSpace(oldEmail) == "" {
		return
	}
	body := fmt.Sprintf("你的 AXmiPic 账号邮箱已由 %s 变更为 %s。\n\n如果这不是你本人的操作，请立即修改密码并检查账号安全设置。", oldEmail, newEmail)
	if err := s.notify.SendEmail(context.Background(), oldEmail, "AXmiPic 邮箱变更通知", body); err != nil {
		slog.Default().Warn("failed to notify old email about change", slog.Any("error", err))
	}
}

// UnbindEmail 解绑邮箱，需要提供当前密码。
func (s *AccountService) UnbindEmail(ctx context.Context, principal *auth.Principal, password string) (*UserDTO, error) {
	account, err := s.repo.GetAccountByID(ctx, principal.StoreRole(), principal.UserID)
	if err != nil {
		return nil, fmt.Errorf("unbind email: %w", err)
	}
	if !auth.VerifyPassword(account.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}
	updated, err := s.repo.UpdateAccountSecurity(ctx, principal.StoreRole(), principal.UserID, map[string]any{
		"email":          "",
		"email_verified": false,
	})
	if err != nil {
		return nil, err
	}
	return toUserDTO(updated), nil
}

// ChangePassword 在验证当前密码后更新账户密码。它用于已登录用户主动改密。
func (s *AccountService) ChangePassword(ctx context.Context, principal *auth.Principal, currentPassword, newPassword string) error {
	account, err := s.repo.GetAccountByID(ctx, principal.StoreRole(), principal.UserID)
	if err != nil {
		return fmt.Errorf("change password: %w", err)
	}
	if !auth.VerifyPassword(account.PasswordHash, currentPassword) {
		return ErrInvalidPassword
	}
	if err := validateNewPassword(newPassword); err != nil {
		return err
	}
	if currentPassword == newPassword {
		return fmt.Errorf("%w: new password must differ from the current one", ErrInvalidInput)
	}
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	if _, err := s.repo.UpdateAccountSecurity(ctx, principal.StoreRole(), principal.UserID, map[string]any{
		"password_hash": hash,
	}); err != nil {
		return fmt.Errorf("change password: %w", err)
	}
	return nil
}

// SendPasswordResetCode 向某个已验证邮箱发送密码重置验证码。为避免账户枚举，
// 当邮箱未绑定任何已验证账户时同样返回成功，但不发送邮件。
func (s *AccountService) SendPasswordResetCode(ctx context.Context, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if _, err := mail.ParseAddress(email); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidEmail, email)
	}
	if s.notify == nil {
		return ErrEmailNotConfigured
	}
	key := passwordResetKey(email)
	if !s.allowEmailSendForKey(ctx, key) {
		return ErrEmailRateLimited
	}
	if _, _, err := s.repo.FindVerifiedAccountByEmail(ctx, email); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// 不泄露该邮箱是否已注册；仍然按频率限制计数以避免探测。
			s.recordEmailSendForKey(ctx, key)
			return nil
		}
		return err
	}
	code, err := generateNumericCode()
	if err != nil {
		return err
	}
	body := fmt.Sprintf("你的 AXmiPic 密码重置验证码是：%s\n\n验证码 %d 分钟内有效，请勿转发给他人。", code, int(emailCodeTTL.Minutes()))
	if err := s.notify.SendEmail(ctx, email, "AXmiPic 密码重置", body); err != nil {
		return err
	}
	if err := s.storeEmailCodeForKey(ctx, key, email, code); err != nil {
		return err
	}
	s.recordEmailSendForKey(ctx, key)
	return nil
}

// ResetPassword 校验邮箱验证码后设置新密码。邮箱不存在或验证码错误都返回同一
// 错误，避免账户枚举。
func (s *AccountService) ResetPassword(ctx context.Context, email, code, newPassword string) error {
	if err := validateNewPassword(newPassword); err != nil {
		return err
	}
	email = strings.ToLower(strings.TrimSpace(email))
	role, account, err := s.repo.FindVerifiedAccountByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrEmailCodeInvalid
		}
		return fmt.Errorf("reset password: %w", err)
	}
	if !s.consumeEmailCodeForKey(ctx, passwordResetKey(email), email, code) {
		return ErrEmailCodeInvalid
	}
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	if _, err := s.repo.UpdateAccountSecurity(ctx, role, account.ID, map[string]any{
		"password_hash": hash,
	}); err != nil {
		return fmt.Errorf("reset password: %w", err)
	}
	return nil
}

// validateNewPassword 校验新密码长度，与注册时保持一致。
func validateNewPassword(password string) error {
	if len(password) < minPasswordLength {
		return fmt.Errorf("%w: password must be at least %d characters", ErrInvalidInput, minPasswordLength)
	}
	if len(password) > maxPasswordLength {
		return fmt.Errorf("%w: password must be at most %d bytes", ErrInvalidInput, maxPasswordLength)
	}
	return nil
}

func (s *AccountService) decryptTOTPSecret(encrypted string) (string, error) {
	if s.cipher == nil {
		return "", ErrTOTPUnavailable
	}
	secret, err := s.cipher.Decrypt(encrypted)
	if err != nil {
		return "", fmt.Errorf("decrypt totp secret: %w", err)
	}
	return secret, nil
}

// storeEmailCodeForKey 按 key 持久化一条验证码（覆盖同 key 的旧验证码）。
func (s *AccountService) storeEmailCodeForKey(ctx context.Context, key, email, code string) error {
	return s.repo.UpsertEmailCode(ctx, key, email, code, time.Now().Add(emailCodeTTL))
}

// allowEmailSendForKey 判断某个 key（账户或重置邮箱）是否还能请求验证码：先查
// 数据库中的当日额度，再消耗内存中的每分钟频率令牌。
func (s *AccountService) allowEmailSendForKey(ctx context.Context, key string) bool {
	today := time.Now().Format("2006-01-02")
	count, err := s.repo.EmailDailyCount(ctx, key, today)
	if err == nil && count >= emailCodeDailyMax {
		return false
	}
	return s.emailLimiter.Allow("user:" + key)
}

// recordEmailSendForKey 在成功发送后累加某个 key 的当日发送次数。
func (s *AccountService) recordEmailSendForKey(ctx context.Context, key string) {
	today := time.Now().Format("2006-01-02")
	_, _ = s.repo.IncrementEmailDaily(ctx, key, today)
}

// consumeEmailCodeForKey 校验并消费某个 key 下的验证码，防止重放。校验失败会
// 增加尝试次数。
func (s *AccountService) consumeEmailCodeForKey(ctx context.Context, key, email, code string) bool {
	pending, err := s.repo.EmailCode(ctx, key)
	if err != nil {
		return false
	}
	if time.Now().After(pending.ExpiresAt) || pending.Email != email {
		_ = s.repo.DeleteEmailCode(ctx, key)
		return false
	}
	if pending.Attempts >= emailCodeMaxAttempts {
		_ = s.repo.DeleteEmailCode(ctx, key)
		return false
	}
	if subtle.ConstantTimeCompare([]byte(pending.Code), []byte(strings.TrimSpace(code))) == 1 {
		consumed, err := s.repo.DeleteEmailCodeIfMatches(ctx, key, pending.Code)
		return err == nil && consumed
	}
	_ = s.repo.SaveEmailCodeAttempts(ctx, key, pending.Attempts+1)
	return false
}

func emailCodeKey(principal *auth.Principal) string {
	return string(principal.Role) + ":" + principal.UserID
}

// passwordResetKey 返回重置密码验证码的存储键（按邮箱维度）。
func passwordResetKey(email string) string {
	return "reset:" + strings.ToLower(strings.TrimSpace(email))
}

// generateNumericCode 生成一个无前导零的 6 位数字验证码。
func generateNumericCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(900000))
	if err != nil {
		return "", fmt.Errorf("generate code: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()+100000), nil
}

// Me 返回某个主体的账户，并根据该主体的角色解析出正确的表。
func (s *AccountService) Me(ctx context.Context, principal *auth.Principal) (*UserDTO, error) {
	account, err := s.repo.GetAccountByID(ctx, principal.StoreRole(), principal.UserID)
	if err != nil {
		return nil, fmt.Errorf("me: %w", err)
	}
	return toUserDTO(account), nil
}

// allowedPreferenceKeys 限定可写入的偏好键及其取值校验。未知键会被忽略，
// 已知键的非法取值会报错，从而在保持可扩展的同时约束数据。
var allowedPreferenceKeys = map[string]func(any) (any, bool){
	"viewer_mode": func(v any) (any, bool) {
		mode, ok := v.(string)
		if !ok {
			return nil, false
		}
		switch mode {
		case "fit", "actual":
			return mode, true
		default:
			return nil, false
		}
	},
	"plaza_layout": func(v any) (any, bool) {
		layout, ok := v.(string)
		if !ok {
			return nil, false
		}
		switch layout {
		case "grid", "masonry":
			return layout, true
		default:
			return nil, false
		}
	},
	"images_layout": func(v any) (any, bool) {
		layout, ok := v.(string)
		if !ok {
			return nil, false
		}
		switch layout {
		case "grid", "masonry":
			return layout, true
		default:
			return nil, false
		}
	},
}

// Preferences 返回某个用户保存的界面偏好。无记录或内容损坏时返回空对象。
func (s *AccountService) Preferences(ctx context.Context, principal *auth.Principal) (map[string]any, error) {
	userID, err := preferenceOwner(principal)
	if err != nil {
		return nil, err
	}
	raw, err := s.repo.GetUserPreference(ctx, userID)
	if err != nil {
		return nil, err
	}
	prefs := map[string]any{}
	if raw == "" {
		return prefs, nil
	}
	if err := json.Unmarshal([]byte(raw), &prefs); err != nil {
		return map[string]any{}, nil
	}
	return prefs, nil
}

// UpdatePreferences 合并并保存用户的界面偏好，返回合并后的完整偏好。
func (s *AccountService) UpdatePreferences(ctx context.Context, principal *auth.Principal, patch map[string]any) (map[string]any, error) {
	current, err := s.Preferences(ctx, principal)
	if err != nil {
		return nil, err
	}
	for key, value := range patch {
		validate, ok := allowedPreferenceKeys[key]
		if !ok {
			continue
		}
		normalized, valid := validate(value)
		if !valid {
			return nil, fmt.Errorf("%w: invalid value for preference %q", ErrInvalidInput, key)
		}
		current[key] = normalized
	}
	encoded, err := json.Marshal(current)
	if err != nil {
		return nil, fmt.Errorf("update preferences: encode: %w", err)
	}
	userID, err := preferenceOwner(principal)
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpsertUserPreference(ctx, userID, string(encoded)); err != nil {
		return nil, err
	}
	return current, nil
}

// preferenceOwner 返回偏好归属的用户 id。
func preferenceOwner(principal *auth.Principal) (string, error) {
	if principal == nil || principal.UserID == "" {
		return "", fmt.Errorf("%w: authentication required", ErrForbidden)
	}
	return principal.UserID, nil
}

// CreateToken 为用户签发一个新的 API 令牌，并仅返回一次明文。
func (s *AccountService) CreateToken(ctx context.Context, userID, name string) (*TokenDTO, error) {
	plaintext, hash, prefix, err := auth.GenerateAPIToken()
	if err != nil {
		return nil, err
	}
	token := &store.APIToken{
		ID:        uuid.NewString(),
		UserID:    userID,
		Name:      strings.TrimSpace(name),
		TokenHash: hash,
		Prefix:    prefix,
	}
	if err := s.repo.CreateToken(ctx, token); err != nil {
		return nil, fmt.Errorf("create token: %w", err)
	}
	dto := toTokenDTO(token)
	dto.Token = plaintext
	return dto, nil
}

// ListTokens 返回某个用户的令牌，但不含机密信息。
func (s *AccountService) ListTokens(ctx context.Context, userID string) ([]TokenDTO, error) {
	tokens, err := s.repo.ListTokensByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	dtos := make([]TokenDTO, 0, len(tokens))
	for i := range tokens {
		dtos = append(dtos, *toTokenDTO(&tokens[i]))
	}
	return dtos, nil
}

// RevokeToken 删除某个用户的其中一个令牌。
func (s *AccountService) RevokeToken(ctx context.Context, userID, tokenID string) error {
	if err := s.repo.DeleteToken(ctx, userID, tokenID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrTokenNotFound
		}
		return fmt.Errorf("revoke token: %w", err)
	}
	return nil
}

// EnsureBootstrapAdmin 在尚无管理员存在时，根据 "username:password" 规格创建
// 一个管理员账户。当规格为空或管理员已存在时，它不执行任何操作。
func (s *AccountService) EnsureBootstrapAdmin(ctx context.Context, spec string) error {
	if strings.TrimSpace(spec) == "" {
		return nil
	}
	count, err := s.repo.CountAdmins(ctx)
	if err != nil {
		return fmt.Errorf("bootstrap admin: count admins: %w", err)
	}
	if count > 0 {
		return nil
	}
	username, password, ok := strings.Cut(spec, ":")
	if !ok || strings.TrimSpace(username) == "" || password == "" {
		return fmt.Errorf("%w: auth.bootstrap_admin must be \"username:password\"", ErrInvalidInput)
	}
	username = strings.ToLower(strings.TrimSpace(username))
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	admin := &store.Admin{
		ID:           uuid.NewString(),
		Username:     username,
		PasswordHash: hash,
	}
	if err := s.repo.CreateAdmin(ctx, admin); err != nil {
		return fmt.Errorf("bootstrap admin: create admin: %w", err)
	}
	return nil
}

func validateCredentials(username, password string) error {
	if len(username) < minUsernameLength || len(username) > maxUsernameLength {
		return fmt.Errorf("%w: username must be %d-%d characters", ErrInvalidInput, minUsernameLength, maxUsernameLength)
	}
	for _, r := range username {
		if !isUsernameRune(r) {
			return fmt.Errorf("%w: username may only contain letters, digits, '.', '_', '-', '+' and '@'", ErrInvalidInput)
		}
	}
	return validateNewPassword(password)
}

func isUsernameRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	case r == '.', r == '_', r == '-', r == '+', r == '@':
		return true
	default:
		return false
	}
}

func accountFromAdmin(admin *store.Admin) *store.Account {
	return &store.Account{
		ID:            admin.ID,
		Username:      admin.Username,
		PasswordHash:  admin.PasswordHash,
		Role:          store.RoleAdmin,
		Disabled:      admin.Disabled,
		Email:         admin.Email,
		EmailVerified: admin.EmailVerified,
		TOTPSecret:    admin.TOTPSecret,
		TOTPEnabled:   admin.TOTPEnabled,
		CreatedAt:     admin.CreatedAt,
		UpdatedAt:     admin.UpdatedAt,
	}
}

func accountFromCustomer(customer *store.Customer) *store.Account {
	return &store.Account{
		ID:            customer.ID,
		Username:      customer.Username,
		PasswordHash:  customer.PasswordHash,
		Role:          store.RoleCustomer,
		Disabled:      customer.Disabled,
		UsedBytes:     customer.UsedBytes,
		QuotaBytes:    customer.QuotaBytes,
		RoleGroupID:   customer.RoleGroupID,
		PlanExpiresAt: customer.PlanExpiresAt,
		Email:         customer.Email,
		EmailVerified: customer.EmailVerified,
		TOTPSecret:    customer.TOTPSecret,
		TOTPEnabled:   customer.TOTPEnabled,
		CreatedAt:     customer.CreatedAt,
		UpdatedAt:     customer.UpdatedAt,
	}
}

func toUserDTO(account *store.Account) *UserDTO {
	return &UserDTO{
		ID:            account.ID,
		Username:      account.Username,
		Role:          string(account.Role),
		Disabled:      account.Disabled,
		UsedBytes:     account.UsedBytes,
		QuotaBytes:    account.QuotaBytes,
		RoleGroupID:   storageIDValue(account.RoleGroupID),
		PlanExpiresAt: account.PlanExpiresAt,
		Email:         account.Email,
		EmailVerified: account.EmailVerified,
		TOTPEnabled:   account.TOTPEnabled,
		CreatedAt:     account.CreatedAt,
	}
}

func toTokenDTO(token *store.APIToken) *TokenDTO {
	return &TokenDTO{
		ID:         token.ID,
		Name:       token.Name,
		Prefix:     token.Prefix,
		LastUsedAt: token.LastUsedAt,
		CreatedAt:  token.CreatedAt,
	}
}

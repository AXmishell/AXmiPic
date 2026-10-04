package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/store"
)

// 账户服务返回的错误。
var (
	// ErrUserExists 表示用户名已被占用。
	ErrUserExists = errors.New("service: username already exists")
	// ErrInvalidCredentials 表示用户名或密码错误。
	ErrInvalidCredentials = errors.New("service: invalid username or password")
	// ErrRegistrationDisabled 表示注册功能已关闭。
	ErrRegistrationDisabled = errors.New("service: registration is disabled")
	// ErrTokenNotFound 表示某个 API 令牌不存在。
	ErrTokenNotFound = errors.New("service: token not found")
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
	RoleGroupID string    `json:"role_group_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// SessionDTO 由登录操作返回。
type SessionDTO struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      UserDTO   `json:"user"`
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
	allowRegistration bool
	defaultQuotaBytes int64
	policies          *PolicyService
}

// NewAccountService 构造一个 AccountService。
func NewAccountService(repo *store.Repository, issuer *auth.SessionIssuer, allowRegistration bool, defaultQuotaBytes int64) *AccountService {
	return &AccountService{
		repo:              repo,
		issuer:            issuer,
		allowRegistration: allowRegistration,
		defaultQuotaBytes: defaultQuotaBytes,
	}
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

// RegisterCustomer 创建一个新的普通（客户）账户。
func (s *AccountService) RegisterCustomer(ctx context.Context, username, password string) (*UserDTO, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if err := validateCredentials(username, password); err != nil {
		return nil, err
	}
	if !s.allowRegistration {
		return nil, ErrRegistrationDisabled
	}
	if _, err := s.repo.GetAccountByUsername(ctx, store.RoleCustomer, username); err == nil {
		return nil, ErrUserExists
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("register: lookup username: %w", err)
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, err
	}
	roleGroupID, quotaBytes := s.registrationPolicy(ctx)
	customer := &store.Customer{
		ID:           uuid.NewString(),
		Username:     username,
		PasswordHash: hash,
		QuotaBytes:   quotaBytes,
		RoleGroupID:  roleGroupID,
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

// RegisterAdmin 在 admins 表中创建一个管理员账户。
func (s *AccountService) RegisterAdmin(ctx context.Context, username, password string) (*UserDTO, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if err := validateCredentials(username, password); err != nil {
		return nil, err
	}
	if _, err := s.repo.GetAccountByUsername(ctx, store.RoleAdmin, username); err == nil {
		return nil, ErrUserExists
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("register admin: lookup username: %w", err)
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
	account, err := s.repo.GetAccountByUsername(ctx, role, strings.ToLower(strings.TrimSpace(username)))
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
	token, expiresAt, err := s.issuer.Issue(account.ID, string(account.Role))
	if err != nil {
		return nil, err
	}
	return &SessionDTO{Token: token, ExpiresAt: expiresAt, User: *toUserDTO(account)}, nil
}

// Me 返回某个主体的账户，并根据该主体的角色解析出正确的表。
func (s *AccountService) Me(ctx context.Context, principal *auth.Principal) (*UserDTO, error) {
	account, err := s.repo.GetAccountByID(ctx, principal.StoreRole(), principal.UserID)
	if err != nil {
		return nil, fmt.Errorf("me: %w", err)
	}
	return toUserDTO(account), nil
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
			return fmt.Errorf("%w: username may only contain letters, digits, '.', '_' and '-'", ErrInvalidInput)
		}
	}
	if len(password) < minPasswordLength {
		return fmt.Errorf("%w: password must be at least %d characters", ErrInvalidInput, minPasswordLength)
	}
	if len(password) > maxPasswordLength {
		return fmt.Errorf("%w: password must be at most %d bytes", ErrInvalidInput, maxPasswordLength)
	}
	return nil
}

func isUsernameRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	case r == '.', r == '_', r == '-':
		return true
	default:
		return false
	}
}

func accountFromAdmin(admin *store.Admin) *store.Account {
	return &store.Account{
		ID:           admin.ID,
		Username:     admin.Username,
		PasswordHash: admin.PasswordHash,
		Role:         store.RoleAdmin,
		Disabled:     admin.Disabled,
		CreatedAt:    admin.CreatedAt,
		UpdatedAt:    admin.UpdatedAt,
	}
}

func accountFromCustomer(customer *store.Customer) *store.Account {
	return &store.Account{
		ID:           customer.ID,
		Username:     customer.Username,
		PasswordHash: customer.PasswordHash,
		Role:         store.RoleCustomer,
		Disabled:     customer.Disabled,
		UsedBytes:    customer.UsedBytes,
		QuotaBytes:   customer.QuotaBytes,
		RoleGroupID:  customer.RoleGroupID,
		CreatedAt:    customer.CreatedAt,
		UpdatedAt:    customer.UpdatedAt,
	}
}

func toUserDTO(account *store.Account) *UserDTO {
	return &UserDTO{
		ID:          account.ID,
		Username:    account.Username,
		Role:        string(account.Role),
		Disabled:    account.Disabled,
		UsedBytes:   account.UsedBytes,
		QuotaBytes:  account.QuotaBytes,
		RoleGroupID: storageIDValue(account.RoleGroupID),
		CreatedAt:   account.CreatedAt,
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

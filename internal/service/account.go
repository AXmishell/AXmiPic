package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/axmipic/axmipic/internal/auth"
	"github.com/axmipic/axmipic/internal/store"
)

// Errors returned by the account service.
var (
	// ErrUserExists indicates the username is already taken.
	ErrUserExists = errors.New("service: username already exists")
	// ErrInvalidCredentials indicates a bad username or password.
	ErrInvalidCredentials = errors.New("service: invalid username or password")
	// ErrRegistrationDisabled indicates registration is turned off.
	ErrRegistrationDisabled = errors.New("service: registration is disabled")
	// ErrTokenNotFound indicates an API token does not exist.
	ErrTokenNotFound = errors.New("service: token not found")
)

const (
	minUsernameLength = 3
	maxUsernameLength = 64
	minPasswordLength = 8
)

// UserDTO is the API representation of an account.
type UserDTO struct {
	ID         string    `json:"id"`
	Username   string    `json:"username"`
	Role       string    `json:"role"`
	UsedBytes  int64     `json:"used_bytes"`
	QuotaBytes int64     `json:"quota_bytes"`
	CreatedAt  time.Time `json:"created_at"`
}

// SessionDTO is returned by login.
type SessionDTO struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      UserDTO   `json:"user"`
}

// TokenDTO is the API representation of an API token. The plaintext Token is
// populated only when the token is created.
type TokenDTO struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Token      string     `json:"token,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// AccountService manages accounts, sessions, and API tokens.
type AccountService struct {
	repo              *store.Repository
	issuer            *auth.SessionIssuer
	allowRegistration bool
	defaultQuotaBytes int64
}

// NewAccountService constructs an AccountService.
func NewAccountService(repo *store.Repository, issuer *auth.SessionIssuer, allowRegistration bool, defaultQuotaBytes int64) *AccountService {
	return &AccountService{
		repo:              repo,
		issuer:            issuer,
		allowRegistration: allowRegistration,
		defaultQuotaBytes: defaultQuotaBytes,
	}
}

// Register creates a new account.
func (s *AccountService) Register(ctx context.Context, username, password string) (*UserDTO, error) {
	username = strings.TrimSpace(username)
	if err := validateCredentials(username, password); err != nil {
		return nil, err
	}
	if !s.allowRegistration {
		return nil, ErrRegistrationDisabled
	}
	if _, err := s.repo.GetUserByUsername(ctx, username); err == nil {
		return nil, ErrUserExists
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("register: lookup username: %w", err)
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, err
	}
	user := &store.User{
		ID:           uuid.NewString(),
		Username:     username,
		PasswordHash: hash,
		Role:         string(auth.RoleUser),
		QuotaBytes:   s.defaultQuotaBytes,
	}
	if err := s.repo.CreateUser(ctx, user); err != nil {
		// A concurrent registration may have inserted the same username first.
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, ErrUserExists
		}
		return nil, fmt.Errorf("register: create user: %w", err)
	}
	return toUserDTO(user), nil
}

// Login verifies credentials and issues a session token.
func (s *AccountService) Login(ctx context.Context, username, password string) (*SessionDTO, error) {
	user, err := s.repo.GetUserByUsername(ctx, strings.TrimSpace(username))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("login: lookup user: %w", err)
	}
	if !auth.VerifyPassword(user.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}
	token, expiresAt, err := s.issuer.Issue(user.ID, user.Role)
	if err != nil {
		return nil, err
	}
	return &SessionDTO{Token: token, ExpiresAt: expiresAt, User: *toUserDTO(user)}, nil
}

// Me returns the account for a user id.
func (s *AccountService) Me(ctx context.Context, userID string) (*UserDTO, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("me: %w", err)
	}
	return toUserDTO(user), nil
}

// CreateToken issues a new API token for a user, returning the plaintext once.
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

// ListTokens returns a user's tokens without secrets.
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

// RevokeToken deletes one of a user's tokens.
func (s *AccountService) RevokeToken(ctx context.Context, userID, tokenID string) error {
	if err := s.repo.DeleteToken(ctx, userID, tokenID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrTokenNotFound
		}
		return fmt.Errorf("revoke token: %w", err)
	}
	return nil
}

// EnsureBootstrapAdmin creates an admin account from a "username:password"
// spec when no accounts exist yet. It is a no-op when the spec is empty or any
// account already exists.
func (s *AccountService) EnsureBootstrapAdmin(ctx context.Context, spec string) error {
	if strings.TrimSpace(spec) == "" {
		return nil
	}
	count, err := s.repo.CountUsers(ctx)
	if err != nil {
		return fmt.Errorf("bootstrap admin: count users: %w", err)
	}
	if count > 0 {
		return nil
	}
	username, password, ok := strings.Cut(spec, ":")
	if !ok || strings.TrimSpace(username) == "" || password == "" {
		return fmt.Errorf("%w: auth.bootstrap_admin must be \"username:password\"", ErrInvalidInput)
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	user := &store.User{
		ID:           uuid.NewString(),
		Username:     strings.TrimSpace(username),
		PasswordHash: hash,
		Role:         string(auth.RoleAdmin),
		QuotaBytes:   s.defaultQuotaBytes,
	}
	if err := s.repo.CreateUser(ctx, user); err != nil {
		return fmt.Errorf("bootstrap admin: create user: %w", err)
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

func toUserDTO(user *store.User) *UserDTO {
	return &UserDTO{
		ID:         user.ID,
		Username:   user.Username,
		Role:       user.Role,
		UsedBytes:  user.UsedBytes,
		QuotaBytes: user.QuotaBytes,
		CreatedAt:  user.CreatedAt,
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

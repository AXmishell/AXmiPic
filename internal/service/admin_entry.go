package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/config"
	"github.com/AXmishell/axmipic/internal/secret"
	"github.com/AXmishell/axmipic/internal/store"
)

// ErrAdminEntryManaged 表示安全入口当前由配置文件/环境变量强制覆盖，后台不能修改。
var ErrAdminEntryManaged = errors.New("service: admin entry is managed by configuration")

const (
	adminEntrySettingKey = "admin_entry"
	adminEntryTokenBytes = 32
	adminEntryTokenMin   = 16
	adminEntryTokenMax   = 128
)

// AdminEntryStatus 是管理员安全入口的对外状态。
type AdminEntryStatus struct {
	Enabled   bool      `json:"enabled"`
	URL       string    `json:"url,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type adminEntryStored struct {
	Enabled   bool      `json:"enabled"`
	Token     string    `json:"token,omitempty"`
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
}

type adminEntryState struct {
	enabled   bool
	token     string
	version   int
	updatedAt time.Time
}

// AdminEntryService 管理管理员安全入口的配置、入口校验与短期入口 Cookie。
type AdminEntryService struct {
	repo   *store.Repository
	cipher *secret.Cipher
	issuer *auth.AdminEntryIssuer
	logger *slog.Logger
	cfg    config.AdminEntryConfig

	mu    sync.RWMutex
	state adminEntryState
}

// NewAdminEntryService 构造管理员安全入口服务。
func NewAdminEntryService(
	repo *store.Repository,
	cipher *secret.Cipher,
	issuer *auth.AdminEntryIssuer,
	logger *slog.Logger,
	cfg config.AdminEntryConfig,
) *AdminEntryService {
	if logger == nil {
		logger = slog.Default()
	}
	return &AdminEntryService{
		repo:   repo,
		cipher: cipher,
		issuer: issuer,
		logger: logger,
		cfg:    cfg,
	}
}

// Bootstrap 从数据库加载安全入口设置，并应用配置/环境变量覆盖。
func (s *AdminEntryService) Bootstrap(ctx context.Context) error {
	var state adminEntryState
	raw, err := s.repo.GetSetting(ctx, adminEntrySettingKey)
	switch {
	case errors.Is(err, store.ErrNotFound):
		// 默认关闭。
	case err != nil:
		return fmt.Errorf("admin entry: load: %w", err)
	default:
		var stored adminEntryStored
		if err := json.Unmarshal([]byte(raw), &stored); err != nil {
			s.logger.Warn("ignoring corrupt admin entry setting", slog.Any("error", err))
		} else {
			token, decErr := s.decryptToken(stored.Token)
			if decErr != nil {
				s.logger.Warn("failed to decrypt admin entry token", slog.Any("error", decErr))
			} else {
				state = adminEntryState{
					enabled:   stored.Enabled && token != "",
					token:     token,
					version:   stored.Version,
					updatedAt: stored.UpdatedAt,
				}
			}
		}
	}

	if s.cfg.Override {
		token := strings.TrimSpace(s.cfg.Token)
		switch {
		case !s.cfg.Enabled:
			state = adminEntryState{}
			s.logger.Warn("admin entry disabled by configuration override")
		case token == "":
			return fmt.Errorf("admin entry: token is required when override is enabled")
		case !validAdminEntryToken(token):
			return fmt.Errorf("admin entry: invalid token in configuration")
		default:
			state = adminEntryState{
				enabled:   true,
				token:     token,
				version:   tokenVersion(token),
				updatedAt: time.Now(),
			}
			s.logger.Warn("admin entry is overridden by configuration")
		}
	}

	s.mu.Lock()
	s.state = state
	s.mu.Unlock()
	return nil
}

// Enabled 报告管理员安全入口是否开启。
func (s *AdminEntryService) Enabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.enabled
}

// Status 返回当前安全入口状态；url 由调用方提供站点根地址。
func (s *AdminEntryService) Status(baseURL string) AdminEntryStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	status := AdminEntryStatus{Enabled: s.state.enabled, UpdatedAt: s.state.updatedAt}
	if s.state.enabled && s.state.token != "" {
		status.URL = strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/admin/entry/" + s.state.token
	}
	return status
}

// ManagedByConfig 报告安全入口是否由配置/环境变量强制控制。
func (s *AdminEntryService) ManagedByConfig() bool { return s.cfg.Override }

// ValidateToken 使用常量时间比较校验安全入口 token。
func (s *AdminEntryService) ValidateToken(token string) bool {
	s.mu.RLock()
	expected := s.state.token
	enabled := s.state.enabled
	s.mu.RUnlock()
	if !enabled || expected == "" {
		return false
	}
	got := sha256.Sum256([]byte(token))
	want := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(got[:], want[:]) == 1
}

// Unlocked 报告请求是否持有与当前入口版本匹配的有效入口 Cookie。
func (s *AdminEntryService) Unlocked(r *http.Request) bool {
	s.mu.RLock()
	enabled := s.state.enabled
	version := s.state.version
	s.mu.RUnlock()
	if !enabled {
		return true
	}
	if s.issuer == nil {
		return false
	}
	cookie, err := r.Cookie(auth.AdminEntryCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}
	got, err := s.issuer.Parse(cookie.Value)
	return err == nil && got == version
}

// SetEntryCookie 在成功校验安全入口后写入短期入口 Cookie。
func (s *AdminEntryService) SetEntryCookie(w http.ResponseWriter, secure bool) error {
	if s.issuer == nil {
		return fmt.Errorf("admin entry: issuer is not configured")
	}
	s.mu.RLock()
	version := s.state.version
	s.mu.RUnlock()
	value, expiresAt, err := s.issuer.Issue(version)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     auth.AdminEntryCookieName,
		Value:    value,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(s.issuer.TTL().Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
	return nil
}

// ClearEntryCookie 清除入口 Cookie（登录成功后调用）。
func (s *AdminEntryService) ClearEntryCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.AdminEntryCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(1, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
}

// SetEnabled 开启或关闭安全入口。开启时若尚无 token 则生成一个。
func (s *AdminEntryService) SetEnabled(ctx context.Context, enabled bool) (string, error) {
	if s.cfg.Override {
		return "", ErrAdminEntryManaged
	}

	s.mu.RLock()
	current := s.state
	s.mu.RUnlock()

	token := current.token
	if enabled && token == "" {
		generated, err := generateAdminEntryToken()
		if err != nil {
			return "", err
		}
		token = generated
	}
	version := current.version + 1
	if version <= 0 {
		version = tokenVersion(token)
	}
	state := adminEntryState{
		enabled:   enabled,
		token:     token,
		version:   version,
		updatedAt: time.Now(),
	}
	if err := s.persist(ctx, state); err != nil {
		return "", err
	}
	s.mu.Lock()
	s.state = state
	s.mu.Unlock()
	return token, nil
}

// Rotate 生成新的安全入口 token 并立即生效，旧入口与旧入口 Cookie 全部失效。
func (s *AdminEntryService) Rotate(ctx context.Context) (string, error) {
	if s.cfg.Override {
		return "", ErrAdminEntryManaged
	}
	token, err := generateAdminEntryToken()
	if err != nil {
		return "", err
	}
	s.mu.RLock()
	current := s.state
	s.mu.RUnlock()
	state := adminEntryState{
		enabled:   true,
		token:     token,
		version:   current.version + 1,
		updatedAt: time.Now(),
	}
	if state.version <= 0 {
		state.version = tokenVersion(token)
	}
	if err := s.persist(ctx, state); err != nil {
		return "", err
	}
	s.mu.Lock()
	s.state = state
	s.mu.Unlock()
	return token, nil
}

func (s *AdminEntryService) persist(ctx context.Context, state adminEntryState) error {
	if s.cipher == nil {
		return fmt.Errorf("admin entry: encryption is not configured")
	}
	ciphertext := ""
	if state.token != "" {
		enc, err := s.cipher.EncryptWithAAD(state.token, "admin_entry:token")
		if err != nil {
			return fmt.Errorf("admin entry: encrypt token: %w", err)
		}
		ciphertext = enc
	}
	encoded, err := json.Marshal(adminEntryStored{
		Enabled:   state.enabled,
		Token:     ciphertext,
		Version:   state.version,
		UpdatedAt: state.updatedAt,
	})
	if err != nil {
		return fmt.Errorf("admin entry: encode: %w", err)
	}
	if err := s.repo.SetSetting(ctx, adminEntrySettingKey, string(encoded)); err != nil {
		return fmt.Errorf("admin entry: save: %w", err)
	}
	return nil
}

func (s *AdminEntryService) decryptToken(ciphertext string) (string, error) {
	if strings.TrimSpace(ciphertext) == "" {
		return "", nil
	}
	if s.cipher == nil {
		return "", fmt.Errorf("encryption is not configured")
	}
	return s.cipher.DecryptWithAAD(ciphertext, "admin_entry:token")
}

func generateAdminEntryToken() (string, error) {
	buf := make([]byte, adminEntryTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("admin entry: generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func validAdminEntryToken(token string) bool {
	if len(token) < adminEntryTokenMin || len(token) > adminEntryTokenMax {
		return false
	}
	for _, r := range token {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func tokenVersion(token string) int {
	sum := sha256.Sum256([]byte(token))
	return int(int32(binary.BigEndian.Uint32(sum[:4])))
}

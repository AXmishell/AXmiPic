package service

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AXmishell/axmipic/internal/store"
)

// 安装服务返回的错误。
var (
	// ErrAlreadyInstalled 表示锁文件已存在，无需再次安装。
	ErrAlreadyInstalled = errors.New("service: already installed")
	// ErrInstallDatabase 表示数据库连接或迁移失败。
	ErrInstallDatabase = errors.New("service: database initialization failed")
)

// InstallStatus 描述当前的安装状态。
type InstallStatus struct {
	Installed bool   `json:"installed"`
	LockFile  string `json:"lock_file"`
	// Reasons 说明为何判定为未安装（便于向导展示）。
	Reasons []string `json:"reasons,omitempty"`
}

// InstallInput 是安装向导提交的配置。
type InstallInput struct {
	SiteName string
	BaseURL  string

	DatabaseDriver string
	DatabaseDSN    string

	AdminUsername string
	AdminPassword string

	StorageDriver string
	StorageRoot   string

	AllowRegistration bool
	AllowGuestUpload  bool
}

// InstallService 负责安装状态判定与初始化。
type InstallService struct {
	lockFile   string
	configPath string
	disabled   bool
	// token 为安装授权令牌；非空时 POST /install 必须携带匹配的令牌。
	token string
}

// NewInstallService 构造一个 InstallService。token 为空且未禁用安装时，调用方
// 应生成一个随机令牌传入。禁用安装时不再校验令牌。
func NewInstallService(lockFile, configPath string, disabled bool, token string) *InstallService {
	return &InstallService{lockFile: lockFile, configPath: configPath, disabled: disabled, token: strings.TrimSpace(token)}
}

// Authorize 报告给定的安装令牌是否可用于执行初始化。禁用安装时不校验；未配置
// 令牌时放行（兼容旧部署与测试）。
func (s *InstallService) Authorize(token string) bool {
	if s.disabled || s.token == "" {
		return true
	}
	return subtle.ConstantTimeCompare([]byte(strings.TrimSpace(token)), []byte(s.token)) == 1
}

// LockFile 返回锁文件路径。
func (s *InstallService) LockFile() string { return s.lockFile }

// Status 返回当前安装状态。disabled 为 true 时始终视为已安装。
func (s *InstallService) Status() InstallStatus {
	if s.disabled {
		return InstallStatus{Installed: true, LockFile: s.lockFile}
	}
	status := InstallStatus{LockFile: s.lockFile}
	if s.lockFile == "" {
		status.Installed = true
		status.Reasons = append(status.Reasons, "install.lock_file 未配置，跳过安装检查")
		return status
	}
	if _, err := os.Stat(s.lockFile); err == nil {
		status.Installed = true
		return status
	} else if !errors.Is(err, os.ErrNotExist) {
		status.Reasons = append(status.Reasons, fmt.Sprintf("无法读取锁文件：%v", err))
		return status
	}
	status.Installed = false
	status.Reasons = append(status.Reasons, "未找到安装锁文件")
	return status
}

// IsInstalled 报告是否已完成安装。
func (s *InstallService) IsInstalled() bool {
	return s.Status().Installed
}

// AdoptExisting 在未检测到锁文件、但数据库中已存在管理员时，写入锁文件，从而
// 让升级到本版本的既有部署无需重新安装。它返回是否写入了锁文件。
func (s *InstallService) AdoptExisting(adminCount int64) (bool, error) {
	if s.disabled || s.lockFile == "" || adminCount <= 0 {
		return false, nil
	}
	if s.IsInstalled() {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(s.lockFile), 0o755); err != nil {
		return false, fmt.Errorf("install: create lock directory: %w", err)
	}
	contents := fmt.Sprintf("installed_at=%s\nadopted_from=existing-database\n", time.Now().Format(time.RFC3339))
	if err := os.WriteFile(s.lockFile, []byte(contents), 0o644); err != nil {
		return false, fmt.Errorf("install: write lock file: %w", err)
	}
	return true, nil
}

// Install 执行初始化：校验输入、写入配置、连接并迁移数据库、创建管理员、
// 播种角色组（默认组与 Guest 组）、创建 Guest 账户，最后写入锁文件。
//
// repoOpener 由调用方注入，用于按输入的数据库参数打开一个仓库（避免 service
// 层直接依赖具体的数据库驱动装配）。
func (s *InstallService) Install(ctx context.Context, in InstallInput, repoOpener func(driver, dsn string) (*store.Repository, error), beforeCreateAdmin func(ctx context.Context, repo *store.Repository, in InstallInput) error) error {
	if s.Status().Installed {
		return ErrAlreadyInstalled
	}
	if err := validateInstallInput(in); err != nil {
		return err
	}

	repo, err := repoOpener(in.DatabaseDriver, in.DatabaseDSN)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInstallDatabase, err)
	}
	defer func() { _ = repo.Close() }()

	if beforeCreateAdmin != nil {
		if err := beforeCreateAdmin(ctx, repo, in); err != nil {
			return err
		}
	}

	// 把向导采集的运行设置写入目标库（config.yaml 仅保留引导项）。
	if err := s.writeRuntimeSettings(ctx, repo, in); err != nil {
		return err
	}

	// 写入配置文件（尽力而为；失败不阻断，但会体现在错误里）。
	if err := s.writeConfig(in); err != nil {
		return err
	}
	// 创建锁文件。
	if err := s.writeLock(in); err != nil {
		return err
	}
	return nil
}

// writeRuntimeSettings 把安装向导采集的运行设置写入目标数据库，避免这些项以
// 明文形式留在 config.yaml 中（config 只保留引导项）。
func (s *InstallService) writeRuntimeSettings(ctx context.Context, repo *store.Repository, in InstallInput) error {
	authSetting, err := json.Marshal(map[string]any{
		"allow_registration": in.AllowRegistration,
		"require_auth":       !in.AllowGuestUpload,
		"allow_guest_upload": in.AllowGuestUpload,
	})
	if err != nil {
		return fmt.Errorf("install: encode auth settings: %w", err)
	}
	if err := repo.SetSetting(ctx, "auth", string(authSetting)); err != nil {
		return err
	}
	siteSetting, err := json.Marshal(map[string]any{
		"name":        strings.TrimSpace(in.SiteName),
		"description": "",
	})
	if err != nil {
		return fmt.Errorf("install: encode site settings: %w", err)
	}
	return repo.SetSetting(ctx, "site", string(siteSetting))
}

// validateInstallInput 校验向导输入。
func validateInstallInput(in InstallInput) error {
	if strings.TrimSpace(in.BaseURL) == "" {
		return fmt.Errorf("%w: base_url is required", ErrInvalidInput)
	}
	if !strings.HasPrefix(in.BaseURL, "http://") && !strings.HasPrefix(in.BaseURL, "https://") {
		return fmt.Errorf("%w: base_url must start with http:// or https://", ErrInvalidInput)
	}
	switch strings.ToLower(strings.TrimSpace(in.DatabaseDriver)) {
	case "", "sqlite", "postgres", "postgresql", "pgx", "mysql", "mariadb":
	default:
		return fmt.Errorf("%w: unsupported database driver %q", ErrInvalidInput, in.DatabaseDriver)
	}
	if strings.TrimSpace(in.DatabaseDSN) == "" {
		return fmt.Errorf("%w: database dsn is required", ErrInvalidInput)
	}
	username := strings.ToLower(strings.TrimSpace(in.AdminUsername))
	if len(username) < minUsernameLength || len(username) > maxUsernameLength {
		return fmt.Errorf("%w: admin username must be %d-%d characters", ErrInvalidInput, minUsernameLength, maxUsernameLength)
	}
	if len(in.AdminPassword) < minPasswordLength {
		return fmt.Errorf("%w: admin password must be at least %d characters", ErrInvalidInput, minPasswordLength)
	}
	if len(in.AdminPassword) > maxPasswordLength {
		return fmt.Errorf("%w: admin password must be at most %d bytes", ErrInvalidInput, maxPasswordLength)
	}
	switch strings.ToLower(strings.TrimSpace(in.StorageDriver)) {
	case "", "local", "s3", "qiniu":
	default:
		return fmt.Errorf("%w: unsupported storage driver %q", ErrInvalidInput, in.StorageDriver)
	}
	return nil
}

// writeLock 写入安装锁文件，并在其中记录安装时间与站点信息。
func (s *InstallService) writeLock(in InstallInput) error {
	if s.lockFile == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.lockFile), 0o755); err != nil {
		return fmt.Errorf("install: create lock directory: %w", err)
	}
	contents := fmt.Sprintf(
		"installed_at=%s\nsite_name=%s\nbase_url=%s\ndatabase_driver=%s\n",
		time.Now().Format(time.RFC3339),
		strings.TrimSpace(in.SiteName),
		strings.TrimSpace(in.BaseURL),
		strings.ToLower(strings.TrimSpace(in.DatabaseDriver)),
	)
	if err := os.WriteFile(s.lockFile, []byte(contents), 0o644); err != nil {
		return fmt.Errorf("install: write lock file: %w", err)
	}
	return nil
}

// writeConfig 把向导输入写入配置文件。它仅在 configPath 非空时执行。
// 生成的配置只包含引导项（监听、数据库、初始存储、安装、日志）；其余运行设置
// （权限开关、上传/处理/限流等）保存在数据库中，可在后台修改。
func (s *InstallService) writeConfig(in InstallInput) error {
	if strings.TrimSpace(s.configPath) == "" {
		return nil
	}
	driver := strings.ToLower(strings.TrimSpace(in.DatabaseDriver))
	if driver == "" {
		driver = "sqlite"
	}
	storageDriver := strings.ToLower(strings.TrimSpace(in.StorageDriver))
	if storageDriver == "" {
		storageDriver = "local"
	}
	root := strings.TrimSpace(in.StorageRoot)
	if root == "" {
		root = "./data/uploads"
	}
	content := fmt.Sprintf(`# 由 AXmiPic 安装向导生成于 %s
# 仅包含引导项；其余运行设置保存在数据库中，可在后台「系统设置」修改。
server:
  host: "0.0.0.0"
  port: 8080
  base_url: %q
  trust_proxy: false
  read_timeout_sec: 30
  write_timeout_sec: 30
  shutdown_timeout_sec: 10

database:
  driver: %q
  dsn: %q

storage:
  driver: %q
  local:
    root: %q

install:
  lock_file: %q
  config_path: %q

logging:
  level: "info"
`,
		time.Now().Format(time.RFC3339),
		strings.TrimSpace(in.BaseURL),
		driver,
		strings.TrimSpace(in.DatabaseDSN),
		storageDriver,
		root,
		s.lockFile,
		s.configPath,
	)
	if err := os.MkdirAll(filepath.Dir(s.configPath), 0o755); err != nil {
		return fmt.Errorf("install: create config directory: %w", err)
	}
	if err := os.WriteFile(s.configPath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("install: write config file: %w", err)
	}
	return nil
}

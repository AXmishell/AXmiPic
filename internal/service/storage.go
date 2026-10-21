package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/AXmishell/axmipic/internal/config"
	"github.com/AXmishell/axmipic/internal/secret"
	"github.com/AXmishell/axmipic/internal/storage"
	"github.com/AXmishell/axmipic/internal/store"
)

// 存储后端管理相关错误。
var (
	// ErrStorageNotFound 表示指定的存储后端不存在。
	ErrStorageNotFound = errors.New("service: storage backend not found")
	// ErrStorageInUse 表示存储后端上仍有图片，不能删除。
	ErrStorageInUse = errors.New("service: storage backend still has images")
	// ErrStorageConfig 表示存储配置无效。
	ErrStorageConfig = errors.New("service: invalid storage configuration")
)

// aadStorageSecrets 是存储后端密钥密文的 AAD 标识。
const aadStorageSecrets = "storage.secrets"

// StorageBackendDTO 是存储后端的对外表示。敏感字段以脱敏形式返回。
type StorageBackendDTO struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Driver    string         `json:"driver"`
	IsCurrent bool           `json:"is_current"`
	Settings  map[string]any `json:"settings"`
	// Secrets 仅包含「是否已设置」的布尔标记，绝不回传明文。
	Secrets    map[string]bool `json:"secrets_set"`
	ImageCount int64           `json:"image_count"`
	CreatedAt  string          `json:"created_at"`
}

// StorageInput 是新增或更新存储后端的输入。
type StorageInput struct {
	Name     string         `json:"name"`
	Driver   string         `json:"driver"`
	Settings map[string]any `json:"settings"`
	// Secrets 中的空字符串表示「保持不变」（更新时）或「未设置」。
	Secrets map[string]string `json:"secrets"`
}

// storageSecretKeys 定义各驱动需要加密保存的敏感字段。
var storageSecretKeys = map[string][]string{
	"s3":    {"access_key_id", "secret_access_key"},
	"qiniu": {"access_key", "secret_key"},
	"oss":   {"access_key_id", "access_key_secret"},
	"cos":   {"secret_id", "secret_key"},
}

// StorageService 管理可命名的存储后端，并在运行中热切换默认后端。
type StorageService struct {
	repo       *store.Repository
	manager    *storage.Manager
	cipher     *secret.Cipher
	baseURL    string
	fileConfig config.StorageConfig // 配置文件中的存储，作为无匹配后端时的兜底
}

// NewStorageService 构建 StorageService，并把配置文件的存储注册为兜底后端。
func NewStorageService(
	repo *store.Repository,
	manager *storage.Manager,
	cipher *secret.Cipher,
	baseURL string,
	fileConfig config.StorageConfig,
) *StorageService {
	return &StorageService{
		repo:       repo,
		manager:    manager,
		cipher:     cipher,
		baseURL:    baseURL,
		fileConfig: fileConfig,
	}
}

// storageCurrentKey 保存当前写入目标（单后端或存储池）。
const storageCurrentKey = "storage.current"

// storageCurrentTarget 是 storage.current 的落库结构。
type storageCurrentTarget struct {
	Kind string `json:"kind"` // backend | pool
	ID   string `json:"id"`
}

// Bootstrap 在启动时加载数据库中的存储后端与存储池，并激活当前写入目标；
// 若数据库尚无记录，则把配置文件中的存储作为兜底。
func (s *StorageService) Bootstrap(ctx context.Context) error {
	// 配置文件中的存储总是作为兜底，保证旧图片在配置被删除后仍可读取。
	if fallback, err := buildStorage(s.baseURL, s.fileConfig); err == nil {
		s.manager.SetFallback(fallback)
	}

	backends, err := s.repo.ListStorageBackends(ctx)
	if err != nil {
		return err
	}
	var legacyCurrentID string
	for i := range backends {
		b := &backends[i]
		cfg, err := s.toConfig(b)
		if err != nil {
			// 单条配置损坏不应阻止服务启动；跳过并继续。
			continue
		}
		if _, err := s.manager.Register(b.ID, b.Name, s.baseURL, cfg); err != nil {
			continue
		}
		if b.IsCurrent {
			legacyCurrentID = b.ID
		}
	}
	// 当前目标以 settings 为准；没有记录时兼容旧版 is_current 标记。
	target, hasTarget := s.loadCurrentTarget(ctx)
	backfillID := legacyCurrentID
	if hasTarget && target.Kind == "backend" {
		backfillID = target.ID
	}

	// 历史图片可能没有 storage_id。升级到存储池之前，把它们归入旧当前后端，
	// 否则池激活后无法确定旧对象的位置。
	if backfillID != "" {
		if count, countErr := s.repo.CountImagesWithoutStorage(ctx); countErr == nil && count > 0 {
			if moved, backfillErr := s.repo.BackfillImageStorage(ctx, backfillID); backfillErr != nil {
				slog.Default().Warn("failed to backfill image storage ids",
					slog.String("storage_id", backfillID), slog.Any("error", backfillErr))
			} else {
				slog.Default().Info("backfilled legacy image storage ids",
					slog.String("storage_id", backfillID), slog.Int64("images", moved))
			}
		}
	}

	// 加载存储池及其成员。
	pools, err := s.repo.ListStoragePools(ctx)
	if err != nil {
		return err
	}
	for i := range pools {
		p := &pools[i]
		members, memberErr := s.repo.ListStoragePoolMembers(ctx, p.ID)
		if memberErr != nil {
			slog.Default().Warn("failed to load storage pool members",
				slog.String("pool", p.ID), slog.Any("error", memberErr))
			continue
		}
		if err := s.manager.RegisterPool(p.ID, p.Name, p.Policy, poolMemberConfigs(members)); err != nil {
			slog.Default().Warn("failed to register storage pool",
				slog.String("pool", p.ID), slog.Any("error", err))
		}
	}

	switch {
	case hasTarget && target.Kind == "pool":
		if count, countErr := s.repo.CountImagesWithoutStorage(ctx); countErr == nil && count > 0 {
			slog.Default().Warn("refusing to activate storage pool with unassigned images",
				slog.String("pool", target.ID), slog.Int64("images", count))
			break
		}
		if err := s.manager.SetCurrentPool(target.ID); err != nil {
			slog.Default().Warn("failed to activate storage pool",
				slog.String("pool", target.ID), slog.Any("error", err))
		} else if err := s.repo.ClearCurrentStorageBackend(ctx); err != nil {
			slog.Default().Warn("failed to clear current storage backend flag", slog.Any("error", err))
		}
	case hasTarget && target.Kind == "backend":
		_ = s.manager.SetCurrent(target.ID)
	case legacyCurrentID != "":
		_ = s.manager.SetCurrent(legacyCurrentID)
	}
	return nil
}

// loadCurrentTarget 读取当前写入目标；记录缺失或损坏时返回 false。
func (s *StorageService) loadCurrentTarget(ctx context.Context) (storageCurrentTarget, bool) {
	raw, err := s.repo.GetSetting(ctx, storageCurrentKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		return storageCurrentTarget{}, false
	}
	var target storageCurrentTarget
	if err := json.Unmarshal([]byte(raw), &target); err != nil {
		return storageCurrentTarget{}, false
	}
	if target.Kind != "pool" && target.Kind != "backend" {
		return storageCurrentTarget{}, false
	}
	if strings.TrimSpace(target.ID) == "" {
		return storageCurrentTarget{}, false
	}
	return target, true
}

// saveCurrentTarget 持久化当前写入目标。
func (s *StorageService) saveCurrentTarget(ctx context.Context, target storageCurrentTarget) error {
	encoded, err := json.Marshal(target)
	if err != nil {
		return fmt.Errorf("storage: encode current target: %w", err)
	}
	return s.repo.SetSetting(ctx, storageCurrentKey, string(encoded))
}

// List 返回所有存储后端（含图片数量），供后台展示。
func (s *StorageService) List(ctx context.Context) ([]StorageBackendDTO, error) {
	backends, err := s.repo.ListStorageBackends(ctx)
	if err != nil {
		return nil, err
	}
	dtos := make([]StorageBackendDTO, 0, len(backends))
	for i := range backends {
		dto, err := s.toDTO(ctx, &backends[i])
		if err != nil {
			return nil, err
		}
		dtos = append(dtos, *dto)
	}
	return dtos, nil
}

// Create 新增一个存储后端。validate 参数控制是否测试连通性。
func (s *StorageService) Create(ctx context.Context, in StorageInput, makeCurrent bool) (*StorageBackendDTO, error) {
	cfg, settings, secrets, err := s.parseInput(in, nil)
	if err != nil {
		return nil, err
	}
	// 先构建实例，验证配置可用（连通性由 buildStorage 内部完成）。使用唯一
	// 临时 id，避免并发创建/更新时相互覆盖或误删。
	validateID := "__validate__-" + uuid.NewString()
	if _, err := s.manager.Register(validateID, in.Name, s.baseURL, cfg); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStorageConfig, err)
	}
	s.manager.Remove(validateID)

	secretsJSON, err := json.Marshal(secrets)
	if err != nil {
		return nil, fmt.Errorf("storage: encode secrets: %w", err)
	}
	encrypted, err := s.cipher.EncryptWithAAD(string(secretsJSON), aadStorageSecrets)
	if err != nil {
		return nil, fmt.Errorf("storage: encrypt secrets: %w", err)
	}
	settingsJSON, _ := json.Marshal(settings)

	b := &store.StorageBackend{
		ID:        uuid.NewString(),
		Name:      strings.TrimSpace(in.Name),
		Driver:    cfg.Driver,
		Settings:  string(settingsJSON),
		Secrets:   encrypted,
		IsCurrent: makeCurrent,
	}
	if err := s.repo.CreateStorageBackend(ctx, b); err != nil {
		return nil, err
	}
	if _, err := s.manager.Register(b.ID, b.Name, s.baseURL, cfg); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStorageConfig, err)
	}
	if makeCurrent {
		if err := s.SetCurrent(ctx, b.ID); err != nil {
			return nil, err
		}
	}
	return s.toDTO(ctx, b)
}

// Update 修改一个存储后端。secrets 中为空的字段保持原值。
func (s *StorageService) Update(ctx context.Context, id string, in StorageInput) (*StorageBackendDTO, error) {
	existing, err := s.repo.GetStorageBackend(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrStorageNotFound
		}
		return nil, err
	}
	cfg, settings, secretUpdates, err := s.parseInput(in, existing)
	if err != nil {
		return nil, err
	}
	validateID := "__validate__-" + uuid.NewString()
	if _, err := s.manager.Register(validateID, in.Name, s.baseURL, cfg); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStorageConfig, err)
	}
	s.manager.Remove(validateID)

	// 合并旧密钥与新密钥。
	merged := s.decryptSecrets(existing)
	for k, v := range secretUpdates {
		if strings.TrimSpace(v) != "" {
			merged[k] = v
		}
	}
	secretsJSON, _ := json.Marshal(merged)
	encrypted, err := s.cipher.EncryptWithAAD(string(secretsJSON), aadStorageSecrets)
	if err != nil {
		return nil, fmt.Errorf("storage: encrypt secrets: %w", err)
	}
	settingsJSON, _ := json.Marshal(settings)

	existing.Name = strings.TrimSpace(in.Name)
	existing.Driver = cfg.Driver
	existing.Settings = string(settingsJSON)
	existing.Secrets = encrypted
	if err := s.repo.UpdateStorageBackend(ctx, existing); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrStorageNotFound
		}
		return nil, err
	}
	if _, err := s.manager.Register(existing.ID, existing.Name, s.baseURL, cfg); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStorageConfig, err)
	}
	return s.toDTO(ctx, existing)
}

// SetCurrent 将指定后端设为当前默认（热切换，无需重启）。
func (s *StorageService) SetCurrent(ctx context.Context, id string) error {
	if _, err := s.repo.GetStorageBackend(ctx, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrStorageNotFound
		}
		return err
	}
	if err := s.repo.SetCurrentStorageBackend(ctx, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrStorageNotFound
		}
		return err
	}
	if err := s.saveCurrentTarget(ctx, storageCurrentTarget{Kind: "backend", ID: id}); err != nil {
		return err
	}
	return s.manager.SetCurrent(id)
}

// Delete 删除一个存储后端。若其上仍有图片则拒绝，避免旧图片失去归属。
func (s *StorageService) Delete(ctx context.Context, id string) error {
	if _, err := s.repo.GetStorageBackend(ctx, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrStorageNotFound
		}
		return err
	}
	memberships, err := s.repo.CountPoolMemberships(ctx, id)
	if err != nil {
		return err
	}
	if memberships > 0 {
		return fmt.Errorf("%w: backend belongs to %d storage pool(s)", ErrStorageInUse, memberships)
	}
	count, err := s.repo.CountImagesOnStorage(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: %d images", ErrStorageInUse, count)
	}
	if err := s.repo.DeleteStorageBackend(ctx, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrStorageNotFound
		}
		return err
	}
	s.manager.Remove(id)
	return nil
}

// toConfig 由数据库记录重建驱动的存储配置。
func (s *StorageService) toConfig(b *store.StorageBackend) (config.StorageConfig, error) {
	in := StorageInput{Name: b.Name, Driver: b.Driver}
	_ = json.Unmarshal([]byte(b.Settings), &in.Settings)
	in.Secrets = s.decryptSecrets(b)
	cfg, _, _, err := s.parseInput(in, b)
	return cfg, err
}

// parseInput 将通用输入解析为具体驱动配置，并拆分出非敏感与敏感字段。
func (s *StorageService) parseInput(in StorageInput, existing *store.StorageBackend) (
	cfg config.StorageConfig,
	settings map[string]any,
	secrets map[string]string,
	err error,
) {
	driver := strings.ToLower(strings.TrimSpace(in.Driver))
	if driver == "" {
		return cfg, nil, nil, fmt.Errorf("%w: driver is required", ErrStorageConfig)
	}
	get := func(key string) string {
		if in.Settings == nil {
			return ""
		}
		if v, ok := in.Settings[key]; ok {
			return fmt.Sprint(v)
		}
		return ""
	}
	getBool := func(key string) bool {
		v := get(key)
		return v == "true" || v == "1" || v == "yes"
	}
	getInt := func(key string) int {
		var n int
		_, _ = fmt.Sscanf(get(key), "%d", &n)
		return n
	}

	cfg.Driver = driver
	switch driver {
	case "local":
		root := get("root")
		if root == "" {
			root = s.fileConfig.Local.Root
		}
		cfg.Local = config.LocalStorageConfig{Root: root}
	case "s3":
		cfg.S3 = config.S3Config{
			Endpoint:         get("endpoint"),
			Region:           get("region"),
			Bucket:           get("bucket"),
			AccessKeyID:      in.Secrets["access_key_id"],
			SecretAccessKey:  in.Secrets["secret_access_key"],
			Secure:           getBool("secure"),
			UsePathStyle:     getBool("use_path_style"),
			PublicBaseURL:    get("public_base_url"),
			PresignExpirySec: getInt("presign_expiry_sec"),
		}
		if cfg.S3.Region == "" {
			cfg.S3.Region = "us-east-1"
		}
		if cfg.S3.PresignExpirySec <= 0 {
			cfg.S3.PresignExpirySec = 900
		}
	case "qiniu":
		cfg.Qiniu = config.QiniuConfig{
			AccessKey:        in.Secrets["access_key"],
			SecretKey:        in.Secrets["secret_key"],
			Bucket:           get("bucket"),
			Domain:           get("domain"),
			UploadHost:       get("upload_host"),
			Zone:             get("zone"),
			Private:          getBool("private"),
			UseHTTPS:         getBool("use_https"),
			PresignExpirySec: getInt("presign_expiry_sec"),
		}
		if cfg.Qiniu.PresignExpirySec <= 0 {
			cfg.Qiniu.PresignExpirySec = 3600
		}
	case "oss":
		cfg.OSS = config.OSSConfig{
			Endpoint:         get("endpoint"),
			Region:           get("region"),
			Bucket:           get("bucket"),
			AccessKeyID:      in.Secrets["access_key_id"],
			AccessKeySecret:  in.Secrets["access_key_secret"],
			Domain:           get("domain"),
			Private:          getBool("private"),
			UseHTTPS:         getBool("use_https"),
			PresignExpirySec: getInt("presign_expiry_sec"),
		}
		if cfg.OSS.PresignExpirySec <= 0 {
			cfg.OSS.PresignExpirySec = 900
		}
	case "cos":
		cfg.COS = config.COSConfig{
			Bucket:           get("bucket"),
			Region:           get("region"),
			SecretID:         in.Secrets["secret_id"],
			SecretKey:        in.Secrets["secret_key"],
			Domain:           get("domain"),
			Private:          getBool("private"),
			UseHTTPS:         getBool("use_https"),
			PresignExpirySec: getInt("presign_expiry_sec"),
		}
		if cfg.COS.PresignExpirySec <= 0 {
			cfg.COS.PresignExpirySec = 900
		}
	default:
		return cfg, nil, nil, fmt.Errorf("%w: unknown driver %q", ErrStorageConfig, driver)
	}

	// 非敏感字段回显给后台；敏感字段单独收集。
	settings = map[string]any{}
	secrets = map[string]string{}
	switch driver {
	case "local":
		settings["root"] = cfg.Local.Root
	case "s3":
		settings["endpoint"] = cfg.S3.Endpoint
		settings["region"] = cfg.S3.Region
		settings["bucket"] = cfg.S3.Bucket
		settings["secure"] = cfg.S3.Secure
		settings["use_path_style"] = cfg.S3.UsePathStyle
		settings["public_base_url"] = cfg.S3.PublicBaseURL
		settings["presign_expiry_sec"] = cfg.S3.PresignExpirySec
		secrets["access_key_id"] = cfg.S3.AccessKeyID
		secrets["secret_access_key"] = cfg.S3.SecretAccessKey
	case "qiniu":
		settings["bucket"] = cfg.Qiniu.Bucket
		settings["domain"] = cfg.Qiniu.Domain
		settings["upload_host"] = cfg.Qiniu.UploadHost
		settings["zone"] = cfg.Qiniu.Zone
		settings["private"] = cfg.Qiniu.Private
		settings["use_https"] = cfg.Qiniu.UseHTTPS
		settings["presign_expiry_sec"] = cfg.Qiniu.PresignExpirySec
		secrets["access_key"] = cfg.Qiniu.AccessKey
		secrets["secret_key"] = cfg.Qiniu.SecretKey
	case "oss":
		settings["endpoint"] = cfg.OSS.Endpoint
		settings["region"] = cfg.OSS.Region
		settings["bucket"] = cfg.OSS.Bucket
		settings["domain"] = cfg.OSS.Domain
		settings["private"] = cfg.OSS.Private
		settings["use_https"] = cfg.OSS.UseHTTPS
		settings["presign_expiry_sec"] = cfg.OSS.PresignExpirySec
		secrets["access_key_id"] = cfg.OSS.AccessKeyID
		secrets["access_key_secret"] = cfg.OSS.AccessKeySecret
	case "cos":
		settings["bucket"] = cfg.COS.Bucket
		settings["region"] = cfg.COS.Region
		settings["domain"] = cfg.COS.Domain
		settings["private"] = cfg.COS.Private
		settings["use_https"] = cfg.COS.UseHTTPS
		settings["presign_expiry_sec"] = cfg.COS.PresignExpirySec
		secrets["secret_id"] = cfg.COS.SecretID
		secrets["secret_key"] = cfg.COS.SecretKey
	}
	return cfg, settings, secrets, nil
}

// decryptSecrets 解开某个后端保存的敏感字段。解密失败时返回空集合。
func (s *StorageService) decryptSecrets(b *store.StorageBackend) map[string]string {
	plain, err := s.cipher.DecryptWithAAD(b.Secrets, aadStorageSecrets)
	if err != nil || plain == "" {
		return map[string]string{}
	}
	out := map[string]string{}
	_ = json.Unmarshal([]byte(plain), &out)
	return out
}

// toDTO 组装对外表示，敏感字段仅给出「是否已设置」。
func (s *StorageService) toDTO(ctx context.Context, b *store.StorageBackend) (*StorageBackendDTO, error) {
	settings := map[string]any{}
	_ = json.Unmarshal([]byte(b.Settings), &settings)
	secretsSet := map[string]bool{}
	for _, key := range storageSecretKeys[b.Driver] {
		secretsSet[key] = true
	}
	// 已设置但值为空的字段视为未设置。
	for key, value := range s.decryptSecrets(b) {
		secretsSet[key] = strings.TrimSpace(value) != ""
	}
	count, err := s.repo.CountImagesOnStorage(ctx, b.ID)
	if err != nil {
		return nil, err
	}
	return &StorageBackendDTO{
		ID:         b.ID,
		Name:       b.Name,
		Driver:     b.Driver,
		IsCurrent:  b.IsCurrent,
		Settings:   settings,
		Secrets:    secretsSet,
		ImageCount: count,
		CreatedAt:  b.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}

// buildStorage 依据配置构建一个 Storage 实例（供兜底使用）。
func buildStorage(baseURL string, cfg config.StorageConfig) (storage.Storage, error) {
	switch cfg.Driver {
	case "local":
		return storage.NewLocal(cfg.Local.Root, baseURL)
	case "s3":
		return storage.NewS3(cfg.S3)
	case "qiniu":
		return storage.NewQiniu(cfg.Qiniu)
	case "oss":
		return storage.NewOSS(cfg.OSS)
	case "cos":
		return storage.NewCOS(cfg.COS)
	default:
		return nil, fmt.Errorf("%w: unknown driver %q", ErrStorageConfig, cfg.Driver)
	}
}

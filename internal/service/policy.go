package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/store"
)

// 角色组与策略服务返回的错误。
var (
	// ErrRoleGroupNotFound 表示请求的角色组不存在。
	ErrRoleGroupNotFound = errors.New("service: role group not found")
	// ErrPolicyNotFound 表示请求的策略不存在。
	ErrPolicyNotFound = errors.New("service: policy not found")
	// ErrRoleGroupInUse 表示角色组仍被客户引用，无法删除。
	ErrRoleGroupInUse = errors.New("service: role group is still assigned to customers")
	// ErrPolicyInUse 表示策略仍被角色组引用，无法删除。
	ErrPolicyInUse = errors.New("service: policy is still attached to a role group")
)

// 功能开关名称。策略中的 feature 类型用这些名称声明允许的功能点。
const (
	FeaturePlaza         = "plaza"
	FeatureAlbums        = "albums"
	FeatureAPITokens     = "api_tokens"
	FeatureBatchUpload   = "batch_upload"
	FeaturePasteUpload   = "paste_upload"
	FeatureDragUpload    = "drag_upload"
	FeatureEmbedCode     = "embed_code"
	FeatureShare         = "share"
	FeatureSharePassword = "share_password"
)

// KnownFeatures 返回内置支持的全部功能开关名称。
func KnownFeatures() []string {
	return []string{
		FeaturePlaza,
		FeatureAlbums,
		FeatureAPITokens,
		FeatureBatchUpload,
		FeaturePasteUpload,
		FeatureDragUpload,
		FeatureEmbedCode,
		FeatureShare,
		FeatureSharePassword,
	}
}

// isValidPolicyType 报告某个策略类型是否被支持。
func isValidPolicyType(policyType string) bool {
	switch policyType {
	case store.PolicyTypeQuota, store.PolicyTypeUpload, store.PolicyTypeRate,
		store.PolicyTypeProcessing, store.PolicyTypeFeature:
		return true
	default:
		return false
	}
}

// 策略配置的原始表示。指针与空切片表示「继承默认值」，从而实现按类型的
// 可叠加覆盖。
type quotaConfig struct {
	QuotaMB *int64 `json:"quota_mb,omitempty"`
}

type uploadConfig struct {
	MaxSizeMB        *int64   `json:"max_size_mb,omitempty"`
	AllowedMIMETypes []string `json:"allowed_mime_types,omitempty"`
}

type rateConfig struct {
	UploadPerMinute *int `json:"upload_per_minute,omitempty"`
	UploadBurst     *int `json:"upload_burst,omitempty"`
	ImagePerMinute  *int `json:"image_per_minute,omitempty"`
	ImageBurst      *int `json:"image_burst,omitempty"`
}

type processingConfig struct {
	Enabled        *bool    `json:"enabled,omitempty"`
	MaxWidth       *int     `json:"max_width,omitempty"`
	MaxHeight      *int     `json:"max_height,omitempty"`
	DefaultQuality *int     `json:"default_quality,omitempty"`
	AllowedFormats []string `json:"allowed_formats,omitempty"`
}

type featureConfig struct {
	Features []string `json:"features,omitempty"`
}

// RateSettings 是解析后的速率限制设置。
type RateSettings struct {
	UploadPerMinute int `json:"upload_per_minute"`
	UploadBurst     int `json:"upload_burst"`
	ImagePerMinute  int `json:"image_per_minute"`
	ImageBurst      int `json:"image_burst"`
}

// ProcessingSettings 是解析后的图片处理设置。
type ProcessingSettings struct {
	Enabled        bool     `json:"enabled"`
	MaxWidth       int      `json:"max_width"`
	MaxHeight      int      `json:"max_height"`
	DefaultQuality int      `json:"default_quality"`
	AllowedFormats []string `json:"allowed_formats"`
}

// EffectivePolicies 是某个账户最终生效的、已解析的策略集合。
type EffectivePolicies struct {
	RoleGroupID      string             `json:"role_group_id,omitempty"`
	RoleGroupName    string             `json:"role_group_name,omitempty"`
	QuotaBytes       int64              `json:"quota_bytes"`
	UploadMaxBytes   int64              `json:"upload_max_bytes"`
	AllowedMIMETypes []string           `json:"allowed_mime_types"`
	Rate             RateSettings       `json:"rate"`
	Processing       ProcessingSettings `json:"processing"`
	Features         []string           `json:"features"`
}

// HasFeature 报告某个功能开关是否生效。
func (e *EffectivePolicies) HasFeature(name string) bool {
	for _, f := range e.Features {
		if f == name {
			return true
		}
	}
	return false
}

// PolicyDefaults 提供策略解析的兜底值，通常来自配置文件。
type PolicyDefaults struct {
	QuotaBytes       int64
	UploadMaxBytes   int64
	AllowedMIMETypes []string
	Rate             RateSettings
	Processing       ProcessingSettings
	Features         []string
}

// PolicyDTO 是策略在 API 中的表示形式。
type PolicyDTO struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	Description string          `json:"description"`
	Enabled     bool            `json:"enabled"`
	Settings    json.RawMessage `json:"settings"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// RoleGroupDTO 是角色组在 API 中的表示形式。
type RoleGroupDTO struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	Description   string      `json:"description"`
	IsDefault     bool        `json:"is_default"`
	CustomerCount int64       `json:"customer_count"`
	PolicyCount   int64       `json:"policy_count"`
	Policies      []PolicyDTO `json:"policies"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
}

// RoleGroupInput 是创建或更新角色组的输入。
type RoleGroupInput struct {
	Name        string
	Description string
	IsDefault   bool
}

// PolicyInput 是创建或更新策略的输入。
type PolicyInput struct {
	Name        string
	Type        string
	Description string
	Enabled     bool
	Settings    json.RawMessage
}

// PolicyService 管理角色组、策略以及两者的绑定，并把绑定解析为对某个
// 账户最终生效的策略集合。
type PolicyService struct {
	repo     *store.Repository
	defaults PolicyDefaults
}

// NewPolicyService 构造一个 PolicyService。
func NewPolicyService(repo *store.Repository, defaults PolicyDefaults) *PolicyService {
	if defaults.Features == nil {
		defaults.Features = KnownFeatures()
	}
	return &PolicyService{repo: repo, defaults: defaults}
}

// Defaults 返回解析所用的兜底策略。
func (s *PolicyService) Defaults() PolicyDefaults { return s.defaults }

// ListRoleGroups 返回全部角色组（含策略）。
func (s *PolicyService) ListRoleGroups(ctx context.Context) ([]RoleGroupDTO, error) {
	groups, err := s.repo.ListRoleGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("list role groups: %w", err)
	}
	dtos := make([]RoleGroupDTO, 0, len(groups))
	for i := range groups {
		dto, err := s.roleGroupDTO(ctx, &groups[i].RoleGroup, groups[i].CustomerCount, groups[i].PolicyCount)
		if err != nil {
			return nil, err
		}
		dtos = append(dtos, *dto)
	}
	return dtos, nil
}

// GetRoleGroup 返回单个角色组（含策略）。
func (s *PolicyService) GetRoleGroup(ctx context.Context, id string) (*RoleGroupDTO, error) {
	group, err := s.repo.GetRoleGroupByID(ctx, id)
	if err != nil {
		return nil, mapRoleGroupError(err)
	}
	count, err := s.repo.CountCustomersInRoleGroup(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get role group: %w", err)
	}
	policies, err := s.repo.ListPoliciesByRoleGroup(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get role group: %w", err)
	}
	return s.roleGroupDTO(ctx, group, count, int64(len(policies)))
}

// CreateRoleGroup 创建一个角色组。
func (s *PolicyService) CreateRoleGroup(ctx context.Context, in RoleGroupInput) (*RoleGroupDTO, error) {
	name, description, err := validateRoleGroupInput(in)
	if err != nil {
		return nil, err
	}
	group := &store.RoleGroup{
		ID:          uuid.NewString(),
		Name:        name,
		Description: description,
		IsDefault:   in.IsDefault,
	}
	if group.IsDefault {
		if err := s.clearDefault(ctx); err != nil {
			return nil, err
		}
	}
	if err := s.repo.CreateRoleGroup(ctx, group); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, fmt.Errorf("%w: role group name already exists", ErrInvalidInput)
		}
		return nil, fmt.Errorf("create role group: %w", err)
	}
	return s.roleGroupDTO(ctx, group, 0, 0)
}

// UpdateRoleGroup 修改角色组的名称、简介与默认标记。
func (s *PolicyService) UpdateRoleGroup(ctx context.Context, id string, in RoleGroupInput) (*RoleGroupDTO, error) {
	name, description, err := validateRoleGroupInput(in)
	if err != nil {
		return nil, err
	}
	isDefault := in.IsDefault
	updated, err := s.repo.UpdateRoleGroup(ctx, id, store.RoleGroupUpdate{
		Name:        &name,
		Description: &description,
		IsDefault:   &isDefault,
	})
	if err != nil {
		return nil, mapRoleGroupError(err)
	}
	return s.GetRoleGroup(ctx, updated.ID)
}

// DeleteRoleGroup 删除角色组；仍被客户引用时返回 ErrRoleGroupInUse。
func (s *PolicyService) DeleteRoleGroup(ctx context.Context, id string) error {
	if err := s.repo.DeleteRoleGroup(ctx, id); err != nil {
		return mapRoleGroupError(err)
	}
	return nil
}

// SetDefaultRoleGroup 把某个角色组标记为默认组，并清除其他组的默认标记。
func (s *PolicyService) SetDefaultRoleGroup(ctx context.Context, id string) (*RoleGroupDTO, error) {
	yes := true
	if _, err := s.repo.UpdateRoleGroup(ctx, id, store.RoleGroupUpdate{IsDefault: &yes}); err != nil {
		return nil, mapRoleGroupError(err)
	}
	return s.GetRoleGroup(ctx, id)
}

// ListPolicies 返回全部策略，可按类型过滤。
func (s *PolicyService) ListPolicies(ctx context.Context, policyType string) ([]PolicyDTO, error) {
	if policyType != "" && !isValidPolicyType(policyType) {
		return nil, fmt.Errorf("%w: unknown policy type %q", ErrInvalidInput, policyType)
	}
	policies, err := s.repo.ListPolicies(ctx, policyType)
	if err != nil {
		return nil, fmt.Errorf("list policies: %w", err)
	}
	dtos := make([]PolicyDTO, 0, len(policies))
	for i := range policies {
		dtos = append(dtos, *policyToDTO(&policies[i]))
	}
	return dtos, nil
}

// GetPolicy 返回单个策略。
func (s *PolicyService) GetPolicy(ctx context.Context, id string) (*PolicyDTO, error) {
	policy, err := s.repo.GetPolicyByID(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrPolicyNotFound
		}
		return nil, fmt.Errorf("get policy: %w", err)
	}
	return policyToDTO(policy), nil
}

// CreatePolicy 创建一个策略。
func (s *PolicyService) CreatePolicy(ctx context.Context, in PolicyInput) (*PolicyDTO, error) {
	name, policyType, description, settings, err := s.validatePolicyInput(in)
	if err != nil {
		return nil, err
	}
	policy := &store.Policy{
		ID:          uuid.NewString(),
		Name:        name,
		Type:        policyType,
		Description: description,
		Enabled:     in.Enabled,
		Settings:    settings,
	}
	if err := s.repo.CreatePolicy(ctx, policy); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, fmt.Errorf("%w: policy name already exists", ErrInvalidInput)
		}
		return nil, fmt.Errorf("create policy: %w", err)
	}
	return policyToDTO(policy), nil
}

// UpdatePolicy 修改策略。
func (s *PolicyService) UpdatePolicy(ctx context.Context, id string, in PolicyInput) (*PolicyDTO, error) {
	name, policyType, description, settings, err := s.validatePolicyInput(in)
	if err != nil {
		return nil, err
	}
	updated, err := s.repo.UpdatePolicy(ctx, id, store.PolicyUpdate{
		Name:        &name,
		Type:        &policyType,
		Description: &description,
		Enabled:     &in.Enabled,
		Settings:    &settings,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrPolicyNotFound
		}
		return nil, fmt.Errorf("update policy: %w", err)
	}
	s.resyncQuotaForPolicy(ctx, id)
	return policyToDTO(updated), nil
}

// DeletePolicy 删除策略；仍被角色组引用时返回 ErrPolicyInUse。
func (s *PolicyService) DeletePolicy(ctx context.Context, id string) error {
	if err := s.repo.DeletePolicy(ctx, id); err != nil {
		switch {
		case errors.Is(err, store.ErrPolicyInUse):
			return ErrPolicyInUse
		case errors.Is(err, store.ErrNotFound):
			return ErrPolicyNotFound
		default:
			return fmt.Errorf("delete policy: %w", err)
		}
	}
	return nil
}

// AttachPolicy 把策略绑定到角色组。同类型策略会替换旧绑定。
func (s *PolicyService) AttachPolicy(ctx context.Context, roleGroupID, policyID string) (*RoleGroupDTO, error) {
	if _, err := s.repo.GetRoleGroupByID(ctx, roleGroupID); err != nil {
		return nil, mapRoleGroupError(err)
	}
	if _, err := s.repo.GetPolicyByID(ctx, policyID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrPolicyNotFound
		}
		return nil, fmt.Errorf("attach policy: %w", err)
	}
	if err := s.repo.AttachPolicy(ctx, roleGroupID, policyID); err != nil {
		return nil, fmt.Errorf("attach policy: %w", err)
	}
	s.resyncQuotaForPolicy(ctx, policyID)
	return s.GetRoleGroup(ctx, roleGroupID)
}

// DetachPolicy 解除角色组与策略的绑定。
func (s *PolicyService) DetachPolicy(ctx context.Context, roleGroupID, policyID string) (*RoleGroupDTO, error) {
	if err := s.repo.DetachPolicy(ctx, roleGroupID, policyID); err != nil {
		return nil, fmt.Errorf("detach policy: %w", err)
	}
	return s.GetRoleGroup(ctx, roleGroupID)
}

// ResolveForRoleGroupID 解析某个角色组生效的策略。
func (s *PolicyService) ResolveForRoleGroupID(ctx context.Context, roleGroupID string) (*EffectivePolicies, error) {
	group, err := s.repo.GetRoleGroupByID(ctx, roleGroupID)
	if err != nil {
		return nil, mapRoleGroupError(err)
	}
	return s.resolveGroup(ctx, group)
}

// ResolveDefault 解析默认角色组生效的策略；没有默认组时返回内置默认值。
func (s *PolicyService) ResolveDefault(ctx context.Context) (*EffectivePolicies, error) {
	group, err := s.repo.GetDefaultRoleGroup(ctx)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return s.basePolicies("", ""), nil
		}
		return nil, fmt.Errorf("resolve default policies: %w", err)
	}
	return s.resolveGroup(ctx, group)
}

// ResolveForCustomer 解析某个客户生效的策略。customerID 为空（访客/管理员）
// 时返回默认策略。
func (s *PolicyService) ResolveForCustomer(ctx context.Context, customerID string) (*EffectivePolicies, error) {
	if strings.TrimSpace(customerID) == "" {
		return s.ResolveDefault(ctx)
	}
	account, err := s.repo.GetAccountByID(ctx, store.RoleCustomer, customerID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return s.ResolveDefault(ctx)
		}
		return nil, fmt.Errorf("resolve customer policies: %w", err)
	}
	if account.RoleGroupID != nil && *account.RoleGroupID != "" {
		group, err := s.repo.GetRoleGroupByID(ctx, *account.RoleGroupID)
		if err == nil {
			return s.resolveGroup(ctx, group)
		}
		if !errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("resolve customer policies: %w", err)
		}
	}
	return s.ResolveDefault(ctx)
}

// ResolveGuest 解析 Guest 访客角色组生效的策略；没有 Guest 组时回退默认组。
func (s *PolicyService) ResolveGuest(ctx context.Context) (*EffectivePolicies, error) {
	groups, err := s.repo.ListRoleGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve guest policies: %w", err)
	}
	for i := range groups {
		if groups[i].RoleGroup.Name == GuestRoleGroupName {
			group, err := s.repo.GetRoleGroupByID(ctx, groups[i].RoleGroup.ID)
			if err != nil {
				return nil, fmt.Errorf("resolve guest policies: %w", err)
			}
			return s.resolveGroup(ctx, group)
		}
	}
	return s.ResolveDefault(ctx)
}

// UploadLimitsFor 实现 UploadPolicyResolver：返回某个主体生效的上传限制。
// 访客使用 Guest 角色组策略。
func (s *PolicyService) UploadLimitsFor(ctx context.Context, principal *auth.Principal) (UploadLimits, error) {
	var effective *EffectivePolicies
	var err error
	if principal.IsGuest() {
		effective, err = s.ResolveGuest(ctx)
	} else {
		effective, err = s.ResolveForCustomer(ctx, principal.UserID)
	}
	if err != nil {
		return UploadLimits{}, err
	}
	return UploadLimits{
		MaxSizeBytes:     effective.UploadMaxBytes,
		AllowedMIMETypes: effective.AllowedMIMETypes,
	}, nil
}

// RateLimitsFor 返回某个主体生效的上传与图片读取速率限制。访客与匿名请求使用
// Guest 角色组策略，其余按账户所属角色组解析。
func (s *PolicyService) RateLimitsFor(ctx context.Context, principal *auth.Principal) (RateSettings, error) {
	var effective *EffectivePolicies
	var err error
	if principal.IsGuest() {
		effective, err = s.ResolveGuest(ctx)
	} else {
		effective, err = s.ResolveForCustomer(ctx, principal.UserID)
	}
	if err != nil {
		return RateSettings{}, fmt.Errorf("resolve rate limits: %w", err)
	}
	return effective.Rate, nil
}

// SeedDefaults 确保至少存在一个默认角色组，并按配置创建每个类型的初始策略。
// 它是幂等的：检测到已有角色组时不再新建。
func (s *PolicyService) SeedDefaults(ctx context.Context) error {
	groups, err := s.repo.ListRoleGroups(ctx)
	if err != nil {
		return fmt.Errorf("seed role groups: %w", err)
	}
	if len(groups) == 0 {
		group := &store.RoleGroup{
			ID:          uuid.NewString(),
			Name:        "默认角色组",
			Description: "新注册用户默认加入的角色组，可在「角色策略」中调整。",
			IsDefault:   true,
		}
		if err := s.repo.CreateRoleGroup(ctx, group); err != nil {
			return fmt.Errorf("seed role groups: %w", err)
		}
		seedPolicies := []PolicyInput{
			{Name: "默认配额", Type: store.PolicyTypeQuota, Description: "账户存储空间上限", Enabled: true,
				Settings: json.RawMessage(mustJSON(quotaConfig{QuotaMB: int64Ptr(s.defaults.QuotaBytes >> 20)}))},
			{Name: "默认上传限制", Type: store.PolicyTypeUpload, Description: "单文件大小与允许的媒体类型", Enabled: true,
				Settings: json.RawMessage(mustJSON(uploadConfig{
					MaxSizeMB:        int64Ptr(s.defaults.UploadMaxBytes >> 20),
					AllowedMIMETypes: s.defaults.AllowedMIMETypes,
				}))},
			{Name: "默认速率限制", Type: store.PolicyTypeRate, Description: "上传与图片读取的每分钟限额", Enabled: true,
				Settings: json.RawMessage(mustJSON(rateConfig{
					UploadPerMinute: intPtr(s.defaults.Rate.UploadPerMinute),
					UploadBurst:     intPtr(s.defaults.Rate.UploadBurst),
					ImagePerMinute:  intPtr(s.defaults.Rate.ImagePerMinute),
					ImageBurst:      intPtr(s.defaults.Rate.ImageBurst),
				}))},
			{Name: "默认图片处理", Type: store.PolicyTypeProcessing, Description: "即时变换的尺寸、质量与格式", Enabled: true,
				Settings: json.RawMessage(mustJSON(processingConfig{
					Enabled:        boolPtr(s.defaults.Processing.Enabled),
					MaxWidth:       intPtr(s.defaults.Processing.MaxWidth),
					MaxHeight:      intPtr(s.defaults.Processing.MaxHeight),
					DefaultQuality: intPtr(s.defaults.Processing.DefaultQuality),
					AllowedFormats: s.defaults.Processing.AllowedFormats,
				}))},
			{Name: "默认功能开关", Type: store.PolicyTypeFeature, Description: "允许使用的功能点集合", Enabled: true,
				Settings: json.RawMessage(mustJSON(featureConfig{Features: s.defaults.Features}))},
		}
		for _, in := range seedPolicies {
			dto, err := s.CreatePolicy(ctx, in)
			if err != nil {
				return fmt.Errorf("seed role groups: %w", err)
			}
			if err := s.repo.AttachPolicy(ctx, group.ID, dto.ID); err != nil {
				return fmt.Errorf("seed role groups: %w", err)
			}
		}
		return nil
	}

	// 已有角色组但没有默认组时，把最早创建的一个提升为默认。
	for i := range groups {
		if groups[i].RoleGroup.IsDefault {
			return nil
		}
	}
	yes := true
	if _, err := s.repo.UpdateRoleGroup(ctx, groups[0].RoleGroup.ID, store.RoleGroupUpdate{IsDefault: &yes}); err != nil {
		return fmt.Errorf("seed role groups: %w", err)
	}
	return nil
}

// GuestRoleGroupName 是访客角色组的固定名称。
const GuestRoleGroupName = "Guest 访客"

// SeedGuestRoleGroup 创建（或返回已存在的）Guest 访客角色组，并绑定一组受限
// 策略：更小的上传体积与配额，且关闭广场、分享、令牌等功能开关。它是幂等的。
func (s *PolicyService) SeedGuestRoleGroup(ctx context.Context, quotaBytes, uploadMaxBytes int64) (*store.RoleGroup, error) {
	if existing, err := s.repo.ListRoleGroups(ctx); err == nil {
		for i := range existing {
			if existing[i].RoleGroup.Name == GuestRoleGroupName {
				return &existing[i].RoleGroup, nil
			}
		}
	} else {
		return nil, fmt.Errorf("seed guest role group: %w", err)
	}

	group := &store.RoleGroup{
		ID:          uuid.NewString(),
		Name:        GuestRoleGroupName,
		Description: "未登录访客使用的低权角色组，仅允许有限上传。",
		IsDefault:   false,
	}
	if err := s.repo.CreateRoleGroup(ctx, group); err != nil {
		return nil, fmt.Errorf("seed guest role group: %w", err)
	}
	guestFeatures := []string{FeatureBatchUpload, FeaturePasteUpload, FeatureEmbedCode}
	policies := []PolicyInput{
		{Name: "访客配额", Type: store.PolicyTypeQuota, Enabled: true,
			Settings: json.RawMessage(mustJSON(quotaConfig{QuotaMB: int64Ptr(quotaBytes >> 20)}))},
		{Name: "访客上传限制", Type: store.PolicyTypeUpload, Enabled: true,
			Settings: json.RawMessage(mustJSON(uploadConfig{
				MaxSizeMB:        int64Ptr(uploadMaxBytes >> 20),
				AllowedMIMETypes: s.defaults.AllowedMIMETypes,
			}))},
		{Name: "访客功能开关", Type: store.PolicyTypeFeature, Enabled: true,
			Settings: json.RawMessage(mustJSON(featureConfig{Features: guestFeatures}))},
	}
	for _, in := range policies {
		dto, err := s.CreatePolicy(ctx, in)
		if err != nil {
			return nil, fmt.Errorf("seed guest role group: %w", err)
		}
		if err := s.repo.AttachPolicy(ctx, group.ID, dto.ID); err != nil {
			return nil, fmt.Errorf("seed guest role group: %w", err)
		}
	}
	return group, nil
}

// CheckDefaultRoleGroup 在注册时可用：返回默认角色组（如存在）。
func (s *PolicyService) defaultGroup(ctx context.Context) (*store.RoleGroup, error) {
	group, err := s.repo.GetDefaultRoleGroup(ctx)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return group, nil
}

// AdoptUnassigned 把尚未分配角色组的历史客户归入默认组，返回更新的行数。
func (s *PolicyService) AdoptUnassigned(ctx context.Context) (int64, error) {
	group, err := s.defaultGroup(ctx)
	if err != nil {
		return 0, fmt.Errorf("adopt unassigned: %w", err)
	}
	if group == nil {
		return 0, nil
	}
	updated, err := s.repo.MigrateCustomerRoleGroups(ctx, group.ID)
	if err != nil {
		return 0, fmt.Errorf("adopt unassigned: %w", err)
	}
	return updated, nil
}

// resolveGroup 把一个角色组解析为生效策略。
func (s *PolicyService) resolveGroup(ctx context.Context, group *store.RoleGroup) (*EffectivePolicies, error) {
	effective := s.basePolicies(group.ID, group.Name)
	policies, err := s.repo.ListPoliciesByRoleGroup(ctx, group.ID)
	if err != nil {
		return nil, fmt.Errorf("resolve policies: %w", err)
	}
	for i := range policies {
		if !policies[i].Enabled {
			continue
		}
		if err := s.applyPolicy(effective, &policies[i]); err != nil {
			return nil, err
		}
	}
	return effective, nil
}

// basePolicies 返回以配置为兜底的默认策略。
func (s *PolicyService) basePolicies(roleGroupID, roleGroupName string) *EffectivePolicies {
	features := append([]string(nil), s.defaults.Features...)
	sort.Strings(features)
	return &EffectivePolicies{
		RoleGroupID:      roleGroupID,
		RoleGroupName:    roleGroupName,
		QuotaBytes:       s.defaults.QuotaBytes,
		UploadMaxBytes:   s.defaults.UploadMaxBytes,
		AllowedMIMETypes: append([]string(nil), s.defaults.AllowedMIMETypes...),
		Rate:             s.defaults.Rate,
		Processing:       s.defaults.Processing,
		Features:         features,
	}
}

// applyPolicy 把一条策略合并进 effective。
func (s *PolicyService) applyPolicy(effective *EffectivePolicies, policy *store.Policy) error {
	raw := policy.Settings
	if strings.TrimSpace(raw) == "" {
		raw = "{}"
	}
	switch policy.Type {
	case store.PolicyTypeQuota:
		var cfg quotaConfig
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			return fmt.Errorf("resolve policy %q: %w", policy.Name, err)
		}
		if cfg.QuotaMB != nil {
			effective.QuotaBytes = *cfg.QuotaMB << 20
		}
	case store.PolicyTypeUpload:
		var cfg uploadConfig
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			return fmt.Errorf("resolve policy %q: %w", policy.Name, err)
		}
		if cfg.MaxSizeMB != nil {
			effective.UploadMaxBytes = *cfg.MaxSizeMB << 20
		}
		if len(cfg.AllowedMIMETypes) > 0 {
			effective.AllowedMIMETypes = append([]string(nil), cfg.AllowedMIMETypes...)
		}
	case store.PolicyTypeRate:
		var cfg rateConfig
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			return fmt.Errorf("resolve policy %q: %w", policy.Name, err)
		}
		if cfg.UploadPerMinute != nil {
			effective.Rate.UploadPerMinute = *cfg.UploadPerMinute
		}
		if cfg.UploadBurst != nil {
			effective.Rate.UploadBurst = *cfg.UploadBurst
		}
		if cfg.ImagePerMinute != nil {
			effective.Rate.ImagePerMinute = *cfg.ImagePerMinute
		}
		if cfg.ImageBurst != nil {
			effective.Rate.ImageBurst = *cfg.ImageBurst
		}
	case store.PolicyTypeProcessing:
		var cfg processingConfig
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			return fmt.Errorf("resolve policy %q: %w", policy.Name, err)
		}
		if cfg.Enabled != nil {
			effective.Processing.Enabled = *cfg.Enabled
		}
		if cfg.MaxWidth != nil {
			effective.Processing.MaxWidth = *cfg.MaxWidth
		}
		if cfg.MaxHeight != nil {
			effective.Processing.MaxHeight = *cfg.MaxHeight
		}
		if cfg.DefaultQuality != nil {
			effective.Processing.DefaultQuality = *cfg.DefaultQuality
		}
		if len(cfg.AllowedFormats) > 0 {
			effective.Processing.AllowedFormats = append([]string(nil), cfg.AllowedFormats...)
		}
	case store.PolicyTypeFeature:
		var cfg featureConfig
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			return fmt.Errorf("resolve policy %q: %w", policy.Name, err)
		}
		features := append([]string(nil), cfg.Features...)
		sort.Strings(features)
		effective.Features = features
	}
	return nil
}

// resyncQuotaForPolicy 在配额策略变化后，把生效配额写回引用该策略的角色组
// 下的所有客户，避免逐个上传时重复解析。
func (s *PolicyService) resyncQuotaForPolicy(ctx context.Context, policyID string) {
	groupIDs, err := s.repo.ListRoleGroupIDsByPolicy(ctx, policyID)
	if err != nil {
		return
	}
	def, _ := s.defaultGroup(ctx)
	for _, groupID := range groupIDs {
		effective, err := s.ResolveForRoleGroupID(ctx, groupID)
		if err != nil {
			continue
		}
		includeUnassigned := def != nil && def.ID == groupID
		_, _ = s.repo.SetCustomersQuotaByRoleGroup(ctx, groupID, effective.QuotaBytes, includeUnassigned)
	}
}

// clearDefault 清除所有角色组的默认标记。
func (s *PolicyService) clearDefault(ctx context.Context) error {
	return s.repo.ClearDefaultRoleGroups(ctx)
}

// roleGroupDTO 组装角色组的对外表示。
func (s *PolicyService) roleGroupDTO(ctx context.Context, group *store.RoleGroup, customers, policyCount int64) (*RoleGroupDTO, error) {
	policies, err := s.repo.ListPoliciesByRoleGroup(ctx, group.ID)
	if err != nil {
		return nil, fmt.Errorf("role group dto: %w", err)
	}
	policyDTOs := make([]PolicyDTO, 0, len(policies))
	for i := range policies {
		policyDTOs = append(policyDTOs, *policyToDTO(&policies[i]))
	}
	return &RoleGroupDTO{
		ID:            group.ID,
		Name:          group.Name,
		Description:   group.Description,
		IsDefault:     group.IsDefault,
		CustomerCount: customers,
		PolicyCount:   policyCount,
		Policies:      policyDTOs,
		CreatedAt:     group.CreatedAt,
		UpdatedAt:     group.UpdatedAt,
	}, nil
}

// validateRoleGroupInput 规范化并校验角色组输入。
func validateRoleGroupInput(in RoleGroupInput) (string, string, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || len([]rune(name)) > 64 {
		return "", "", fmt.Errorf("%w: role group name must be 1-64 characters", ErrInvalidInput)
	}
	description := strings.TrimSpace(in.Description)
	if len([]rune(description)) > 255 {
		return "", "", fmt.Errorf("%w: role group description must be at most 255 characters", ErrInvalidInput)
	}
	return name, description, nil
}

// validatePolicyInput 规范化并校验策略输入，返回规范化的设置 JSON。
func (s *PolicyService) validatePolicyInput(in PolicyInput) (string, string, string, string, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || len([]rune(name)) > 64 {
		return "", "", "", "", fmt.Errorf("%w: policy name must be 1-64 characters", ErrInvalidInput)
	}
	policyType := strings.TrimSpace(in.Type)
	if !isValidPolicyType(policyType) {
		return "", "", "", "", fmt.Errorf("%w: unknown policy type %q", ErrInvalidInput, in.Type)
	}
	description := strings.TrimSpace(in.Description)
	if len([]rune(description)) > 255 {
		return "", "", "", "", fmt.Errorf("%w: policy description must be at most 255 characters", ErrInvalidInput)
	}
	settings, err := normalizePolicySettings(policyType, in.Settings)
	if err != nil {
		return "", "", "", "", err
	}
	return name, policyType, description, settings, nil
}

// normalizePolicySettings 解析并校验类型相关的设置，返回规范化后的 JSON。
func normalizePolicySettings(policyType string, raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		raw = json.RawMessage("{}")
	}
	switch policyType {
	case store.PolicyTypeQuota:
		var cfg quotaConfig
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return "", fmt.Errorf("%w: invalid quota settings: %v", ErrInvalidInput, err)
		}
		if cfg.QuotaMB != nil && *cfg.QuotaMB < 0 {
			return "", fmt.Errorf("%w: quota_mb must not be negative", ErrInvalidInput)
		}
		return mustJSON(cfg), nil
	case store.PolicyTypeUpload:
		var cfg uploadConfig
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return "", fmt.Errorf("%w: invalid upload settings: %v", ErrInvalidInput, err)
		}
		if cfg.MaxSizeMB != nil && *cfg.MaxSizeMB < 0 {
			return "", fmt.Errorf("%w: max_size_mb must not be negative", ErrInvalidInput)
		}
		return mustJSON(cfg), nil
	case store.PolicyTypeRate:
		var cfg rateConfig
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return "", fmt.Errorf("%w: invalid rate settings: %v", ErrInvalidInput, err)
		}
		for _, v := range []*int{cfg.UploadPerMinute, cfg.UploadBurst, cfg.ImagePerMinute, cfg.ImageBurst} {
			if v != nil && *v < 0 {
				return "", fmt.Errorf("%w: rate limits must not be negative", ErrInvalidInput)
			}
		}
		return mustJSON(cfg), nil
	case store.PolicyTypeProcessing:
		var cfg processingConfig
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return "", fmt.Errorf("%w: invalid processing settings: %v", ErrInvalidInput, err)
		}
		if cfg.MaxWidth != nil && *cfg.MaxWidth < 1 {
			return "", fmt.Errorf("%w: max_width must be at least 1", ErrInvalidInput)
		}
		if cfg.MaxHeight != nil && *cfg.MaxHeight < 1 {
			return "", fmt.Errorf("%w: max_height must be at least 1", ErrInvalidInput)
		}
		if cfg.DefaultQuality != nil && (*cfg.DefaultQuality < 1 || *cfg.DefaultQuality > 100) {
			return "", fmt.Errorf("%w: default_quality must be between 1 and 100", ErrInvalidInput)
		}
		return mustJSON(cfg), nil
	case store.PolicyTypeFeature:
		var cfg featureConfig
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return "", fmt.Errorf("%w: invalid feature settings: %v", ErrInvalidInput, err)
		}
		for _, f := range cfg.Features {
			if !knownFeature(f) {
				return "", fmt.Errorf("%w: unknown feature %q", ErrInvalidInput, f)
			}
		}
		return mustJSON(cfg), nil
	default:
		return "", fmt.Errorf("%w: unknown policy type %q", ErrInvalidInput, policyType)
	}
}

func knownFeature(name string) bool {
	for _, f := range KnownFeatures() {
		if f == name {
			return true
		}
	}
	return false
}

// policyToDTO 组装策略的对外表示。
func policyToDTO(policy *store.Policy) *PolicyDTO {
	settings := json.RawMessage(policy.Settings)
	if len(settings) == 0 {
		settings = json.RawMessage("{}")
	}
	return &PolicyDTO{
		ID:          policy.ID,
		Name:        policy.Name,
		Type:        policy.Type,
		Description: policy.Description,
		Enabled:     policy.Enabled,
		Settings:    settings,
		CreatedAt:   policy.CreatedAt,
		UpdatedAt:   policy.UpdatedAt,
	}
}

// mapRoleGroupError 把 store 层的角色组错误映射为 service 层错误。
func mapRoleGroupError(err error) error {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return ErrRoleGroupNotFound
	case errors.Is(err, store.ErrRoleGroupInUse):
		return ErrRoleGroupInUse
	default:
		return fmt.Errorf("role group: %w", err)
	}
}

// mustJSON 把值序列化为 JSON；失败时返回 "{}"（值均为内部构造，不应失败）。
func mustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func intPtr(v int) *int       { return &v }
func int64Ptr(v int64) *int64 { return &v }
func boolPtr(v bool) *bool    { return &v }

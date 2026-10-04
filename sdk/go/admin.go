package axmipic

import (
	"context"
)

// UserUpdate 是账户可选的变更字段。
type UserUpdate struct {
	Disabled    *bool   `json:"disabled,omitempty"`
	RoleGroupID *string `json:"role_group_id,omitempty"`
}

// RoleGroupInput 是创建或更新角色组的输入。
type RoleGroupInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsDefault   bool   `json:"is_default"`
}

// PolicyInput 是创建或更新策略的输入。
type PolicyInput struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	Settings    any    `json:"settings"`
}

// StorageInput 是创建或更新存储后端的输入。
type StorageInput struct {
	Name     string            `json:"name"`
	Driver   string            `json:"driver"`
	Settings map[string]any    `json:"settings"`
	Secrets  map[string]string `json:"secrets"`
	Activate bool              `json:"activate,omitempty"`
}

// NotifyTest 是一条测试通知请求。
type NotifyTest struct {
	Channel string `json:"channel"`
	To      string `json:"to"`
	Subject string `json:"subject,omitempty"`
	Body    string `json:"body"`
}

// ImagingDrivers 描述可用的图片处理驱动。
type ImagingDrivers struct {
	Available []string `json:"available"`
	Active    string   `json:"active"`
}

// RuntimeInfo 描述实例的运行时与运行环境信息（不含密钥）。
type RuntimeInfo struct {
	SiteName          string   `json:"site_name"`
	BaseURL           string   `json:"base_url"`
	DatabaseDriver    string   `json:"database_driver"`
	StorageDriver     string   `json:"storage_driver"`
	Processor         string   `json:"processor"`
	Formats           []string `json:"formats"`
	AllowRegistration bool     `json:"allow_registration"`
	RequireAuth       bool     `json:"require_auth"`
	AllowGuestUpload  bool     `json:"allow_guest_upload"`
	GuestQuotaMB      int      `json:"guest_quota_mb"`
	GuestUploadMaxMB  int      `json:"guest_upload_max_mb"`
	DefaultQuotaMB    int      `json:"default_quota_mb"`
	UploadMaxMB       int      `json:"upload_max_mb"`
	TrustProxy        bool     `json:"trust_proxy"`
	SessionTTLHours   int      `json:"session_ttl_hours"`
	InstallLockFile   string   `json:"install_lock_file"`
	Installed         bool     `json:"installed"`
	GoVersion         string   `json:"go_version"`
	Platform          string   `json:"platform"`
}

// AdminStats 返回实例统计。
func (c *Client) AdminStats(ctx context.Context) (*Stats, error) {
	var out Stats
	if err := c.get(ctx, "/api/v1/admin/stats", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminListCustomers 返回普通用户。
func (c *Client) AdminListCustomers(ctx context.Context) ([]User, error) {
	var out []User
	if err := c.get(ctx, "/api/v1/admin/customers", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// AdminListAdmins 返回管理员。
func (c *Client) AdminListAdmins(ctx context.Context) ([]User, error) {
	var out []User
	if err := c.get(ctx, "/api/v1/admin/admins", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// AdminCreateAdmin 新建管理员。
func (c *Client) AdminCreateAdmin(ctx context.Context, username, password string) (*User, error) {
	var out User
	in := Credentials{Username: username, Password: password}
	if err := c.postJSON(ctx, "/api/v1/admin/admins", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminUpdateCustomer 修改普通用户。
func (c *Client) AdminUpdateCustomer(ctx context.Context, id string, in UserUpdate) (*User, error) {
	var out User
	if err := c.patchJSON(ctx, "/api/v1/admin/customers/"+id, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminUpdateAdmin 修改管理员。
func (c *Client) AdminUpdateAdmin(ctx context.Context, id string, in UserUpdate) (*User, error) {
	var out User
	if err := c.patchJSON(ctx, "/api/v1/admin/admins/"+id, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminDeleteCustomer 删除普通用户。
func (c *Client) AdminDeleteCustomer(ctx context.Context, id string) error {
	return c.delete(ctx, "/api/v1/admin/customers/"+id, nil)
}

// AdminDeleteAdmin 删除管理员。
func (c *Client) AdminDeleteAdmin(ctx context.Context, id string) error {
	return c.delete(ctx, "/api/v1/admin/admins/"+id, nil)
}

// ---- 存储 ----

// AdminListStorage 返回存储后端列表。
func (c *Client) AdminListStorage(ctx context.Context) ([]StorageBackend, error) {
	var out []StorageBackend
	if err := c.get(ctx, "/api/v1/admin/storage", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// AdminCreateStorage 新建存储后端。
func (c *Client) AdminCreateStorage(ctx context.Context, in StorageInput) (*StorageBackend, error) {
	var out StorageBackend
	if err := c.postJSON(ctx, "/api/v1/admin/storage", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminGetStorage 返回存储后端详情。
func (c *Client) AdminGetStorage(ctx context.Context, id string) (*StorageBackend, error) {
	var out StorageBackend
	if err := c.get(ctx, "/api/v1/admin/storage/"+id, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminUpdateStorage 修改存储后端。
func (c *Client) AdminUpdateStorage(ctx context.Context, id string, in StorageInput) (*StorageBackend, error) {
	var out StorageBackend
	if err := c.putJSON(ctx, "/api/v1/admin/storage/"+id, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminDeleteStorage 删除存储后端。
func (c *Client) AdminDeleteStorage(ctx context.Context, id string) error {
	return c.delete(ctx, "/api/v1/admin/storage/"+id, nil)
}

// AdminActivateStorage 切换默认存储后端。
func (c *Client) AdminActivateStorage(ctx context.Context, id string) error {
	return c.postJSON(ctx, "/api/v1/admin/storage/"+id+"/activate", map[string]any{}, nil)
}

// ---- 角色组与策略 ----

// AdminListRoleGroups 返回全部角色组。
func (c *Client) AdminListRoleGroups(ctx context.Context) ([]RoleGroup, error) {
	var out []RoleGroup
	if err := c.get(ctx, "/api/v1/admin/role-groups", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// AdminCreateRoleGroup 创建角色组。
func (c *Client) AdminCreateRoleGroup(ctx context.Context, in RoleGroupInput) (*RoleGroup, error) {
	var out RoleGroup
	if err := c.postJSON(ctx, "/api/v1/admin/role-groups", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminUpdateRoleGroup 修改角色组。
func (c *Client) AdminUpdateRoleGroup(ctx context.Context, id string, in RoleGroupInput) (*RoleGroup, error) {
	var out RoleGroup
	if err := c.putJSON(ctx, "/api/v1/admin/role-groups/"+id, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminDeleteRoleGroup 删除角色组。
func (c *Client) AdminDeleteRoleGroup(ctx context.Context, id string) error {
	return c.delete(ctx, "/api/v1/admin/role-groups/"+id, nil)
}

// AdminAttachPolicy 把策略绑定到角色组。
func (c *Client) AdminAttachPolicy(ctx context.Context, roleGroupID, policyID string) (*RoleGroup, error) {
	var out RoleGroup
	in := map[string]string{"policy_id": policyID}
	if err := c.postJSON(ctx, "/api/v1/admin/role-groups/"+roleGroupID+"/policies", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminDetachPolicy 解除角色组与策略的绑定。
func (c *Client) AdminDetachPolicy(ctx context.Context, roleGroupID, policyID string) (*RoleGroup, error) {
	var out RoleGroup
	if err := c.delete(ctx, "/api/v1/admin/role-groups/"+roleGroupID+"/policies/"+policyID, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminListPolicies 返回策略，可按类型过滤。
func (c *Client) AdminListPolicies(ctx context.Context, policyType string) ([]Policy, error) {
	var out []Policy
	path := encodeQuery("/api/v1/admin/policies", map[string]string{"type": policyType})
	if err := c.get(ctx, path, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// AdminCreatePolicy 创建策略。
func (c *Client) AdminCreatePolicy(ctx context.Context, in PolicyInput) (*Policy, error) {
	var out Policy
	if err := c.postJSON(ctx, "/api/v1/admin/policies", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminUpdatePolicy 修改策略。
func (c *Client) AdminUpdatePolicy(ctx context.Context, id string, in PolicyInput) (*Policy, error) {
	var out Policy
	if err := c.putJSON(ctx, "/api/v1/admin/policies/"+id, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminDeletePolicy 删除策略。
func (c *Client) AdminDeletePolicy(ctx context.Context, id string) error {
	return c.delete(ctx, "/api/v1/admin/policies/"+id, nil)
}

// ---- 通知、安全与驱动 ----

// AdminNotifyChannels 返回已配置的通知渠道。
func (c *Client) AdminNotifyChannels(ctx context.Context) (map[string]string, error) {
	var out map[string]string
	if err := c.get(ctx, "/api/v1/admin/notify/channels", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// AdminTestNotify 发送一条测试通知。
func (c *Client) AdminTestNotify(ctx context.Context, in NotifyTest) error {
	return c.postJSON(ctx, "/api/v1/admin/notify/test", in, nil)
}

// AdminSecurity 返回当前启用的安全扫描器。
func (c *Client) AdminSecurity(ctx context.Context) (map[string]string, error) {
	var out map[string]string
	if err := c.get(ctx, "/api/v1/admin/security", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// AdminImagingDrivers 返回可用的图片处理驱动。
func (c *Client) AdminImagingDrivers(ctx context.Context) (*ImagingDrivers, error) {
	var out ImagingDrivers
	if err := c.get(ctx, "/api/v1/admin/imaging/drivers", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminRuntimeInfo 返回实例运行环境信息。
func (c *Client) AdminRuntimeInfo(ctx context.Context) (*RuntimeInfo, error) {
	var out RuntimeInfo
	if err := c.get(ctx, "/api/v1/admin/runtime", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

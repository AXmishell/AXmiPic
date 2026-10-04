package store

import "time"

// Image 是单个已存储对象的持久化元数据。
type Image struct {
	ID     string  `gorm:"primaryKey;size:36"`
	Key    string  `gorm:"uniqueIndex;size:255;not null"`
	UserID *string `gorm:"index;size:36"`
	// StorageID 指向对象所在的存储后端；为空时回退到当前默认后端。
	StorageID *string `gorm:"index;size:36"`
	// AlbumID 指向所属相册；为空表示未归入任何相册。
	AlbumID *string `gorm:"index;size:36"`
	// Permission 为图片可见性：public（可出现在图片广场）或 private（默认）。
	Permission string `gorm:"size:16;not null;default:'private'"`
	// OriginalName 是上传时的原始文件名；Filename 是重命名后的存储文件名
	// （即 Key 的最后一段）；Hash 是内容的 sha256 十六进制摘要。
	OriginalName string `gorm:"size:255;not null;default:''"`
	Filename     string `gorm:"size:255;not null;default:''"`
	Hash         string `gorm:"size:64;index;not null;default:''"`
	URL          string `gorm:"size:512;not null"`
	Size         int64  `gorm:"not null"`
	MimeType     string `gorm:"size:100;not null"`
	Width        int
	Height       int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// TableName 返回存储 Image 的表名。
func (Image) TableName() string {
	return "images"
}

// 图片可见性取值。
const (
	// PermissionPrivate 表示图片仅本人可见（默认）。
	PermissionPrivate = "private"
	// PermissionPublic 表示图片可出现在公开的图片广场。
	PermissionPublic = "public"
)

// Album 是用户创建的相册，用于归类图片。
type Album struct {
	ID     string  `gorm:"primaryKey;size:36"`
	UserID *string `gorm:"index;size:36"`
	Name   string  `gorm:"size:100;not null"`
	Intro  string  `gorm:"size:255;not null;default:''"`
	// Permission 为相册可见性：public（无需登录即可浏览）或 private（默认）。
	Permission string `gorm:"size:16;not null;default:'private'"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// TableName 返回存储 Album 的表名。
func (Album) TableName() string {
	return "albums"
}

// PendingUpload 跟踪尚未确认的预签名直传上传。该记录在签发预签名请求时创建，
// 并在确认步骤中被消费，从而防止客户端认领任意键。
type PendingUpload struct {
	Key    string  `gorm:"primaryKey;size:255"`
	UserID *string `gorm:"index;size:36"`
	// StorageID 记录该直传目标所在的存储后端。
	StorageID *string   `gorm:"index;size:36"`
	MimeType  string    `gorm:"size:100;not null"`
	MaxSize   int64     `gorm:"not null"`
	ExpiresAt time.Time `gorm:"index;not null"`
	CreatedAt time.Time
}

// TableName 返回存储 PendingUpload 的表名。
func (PendingUpload) TableName() string {
	return "pending_uploads"
}

// Admin 是存储在其自身表中的特权账户。
type Admin struct {
	ID           string `gorm:"primaryKey;size:36"`
	Username     string `gorm:"uniqueIndex;size:64;not null"`
	PasswordHash string `gorm:"size:100;not null"`
	Disabled     bool   `gorm:"not null;default:false"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// TableName 返回存储 Admin 的表名。
func (Admin) TableName() string {
	return "admins"
}

// Customer 是存储在其自身表中的普通（非特权）账户。
type Customer struct {
	ID           string `gorm:"primaryKey;size:36"`
	Username     string `gorm:"uniqueIndex;size:64;not null"`
	PasswordHash string `gorm:"size:100;not null"`
	Disabled     bool   `gorm:"not null;default:false"`
	UsedBytes    int64  `gorm:"not null;default:0"`
	QuotaBytes   int64  `gorm:"not null;default:0"` // 0 表示不限额
	// RoleGroupID 指向用户所属的角色组；为空表示使用默认角色组。
	RoleGroupID *string `gorm:"index;size:36"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TableName 返回存储 Customer 的表名。
func (Customer) TableName() string {
	return "customers"
}

// UserUpdate 携带可选的账户字段更改。Nil 字段将被忽略。RoleGroupID 仅对
// 客户有意义：非 nil 时字符串值指向新角色组，空字符串表示清空（回退默认组）。
type UserUpdate struct {
	Disabled    *bool
	RoleGroupID *string
	QuotaBytes  *int64
}

// ImageStats 汇总图像数量和总字节数。
type ImageStats struct {
	Count      int64
	TotalBytes int64
}

// APIToken 是长期有效的程序化凭据。仅存储哈希值。
type APIToken struct {
	ID         string `gorm:"primaryKey;size:36"`
	UserID     string `gorm:"index;size:36;not null"`
	Name       string `gorm:"size:100"`
	TokenHash  string `gorm:"uniqueIndex;size:64;not null"`
	Prefix     string `gorm:"size:16;not null"`
	LastUsedAt *time.Time
	ExpiresAt  *time.Time
	CreatedAt  time.Time
}

// TableName 返回存储 APIToken 的表名。
func (APIToken) TableName() string {
	return "api_tokens"
}

// StorageBackend 是管理员在后台配置的、可命名的存储后端。
// S3 / 七牛的密钥以密文保存；非敏感参数以 JSON 存入 Settings。
type StorageBackend struct {
	ID        string `gorm:"primaryKey;size:36"`
	Name      string `gorm:"size:100;not null"`
	Driver    string `gorm:"size:16;not null"`   // local | s3 | qiniu
	Settings  string `gorm:"type:text;not null"` // JSON 形式的非敏感参数
	Secrets   string `gorm:"type:text;not null"` // 加密后的敏感参数（JSON）
	IsCurrent bool   `gorm:"not null;default:false"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName 返回存储 StorageBackend 的表名。
func (StorageBackend) TableName() string {
	return "storage_backends"
}

// RoleGroup 是管理员定义的一组权限与策略集合，可分配给普通用户，从而
// 实现按角色分级的资源与功能控制。
type RoleGroup struct {
	ID          string `gorm:"primaryKey;size:36"`
	Name        string `gorm:"uniqueIndex;size:64;not null"`
	Description string `gorm:"size:255;not null;default:''"`
	// IsDefault 标记新注册用户默认加入的角色组；全库至多一个。
	IsDefault bool `gorm:"not null;default:false"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName 返回存储 RoleGroup 的表名。
func (RoleGroup) TableName() string {
	return "role_groups"
}

// 策略类型。一个角色组可绑定多个不同类型的策略，按类型合并为最终生效值。
const (
	// PolicyTypeQuota 控制存储配额。
	PolicyTypeQuota = "quota"
	// PolicyTypeUpload 控制上传大小与允许的媒体类型。
	PolicyTypeUpload = "upload"
	// PolicyTypeRate 控制各类接口的速率限制。
	PolicyTypeRate = "rate"
	// PolicyTypeProcessing 控制图片处理能力。
	PolicyTypeProcessing = "processing"
	// PolicyTypeFeature 控制功能开关与权限点。
	PolicyTypeFeature = "feature"
)

// Policy 是一个可复用的、带类型的策略定义。多个角色组可以引用同一策略。
type Policy struct {
	ID          string `gorm:"primaryKey;size:36"`
	Name        string `gorm:"uniqueIndex;size:64;not null"`
	Type        string `gorm:"size:32;not null"`
	Description string `gorm:"size:255;not null;default:''"`
	Enabled     bool   `gorm:"not null;default:true"`
	// Settings 为类型相关的 JSON 配置。
	Settings  string `gorm:"type:text;not null;default:'{}'"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName 返回存储 Policy 的表名。
func (Policy) TableName() string {
	return "policies"
}

// RoleGroupPolicy 关联角色组与策略。
type RoleGroupPolicy struct {
	RoleGroupID string `gorm:"primaryKey;size:36"`
	PolicyID    string `gorm:"primaryKey;size:36"`
}

// TableName 返回存储 RoleGroupPolicy 的表名。
func (RoleGroupPolicy) TableName() string {
	return "role_group_policies"
}

// 分享目标类型。
const (
	// ShareTargetImage 表示分享单张图片。
	ShareTargetImage = "image"
	// ShareTargetAlbum 表示分享整个相册。
	ShareTargetAlbum = "album"
)

// Share 是一个可对外访问的分享链接，可指向一张图片或一个相册，并可选地
// 要求密码、限制有效期或访问次数。
type Share struct {
	ID string `gorm:"primaryKey;size:36"`
	// Token 是出现在公开链接中的不可猜测标识。
	Token      string `gorm:"uniqueIndex;size:64;not null"`
	UserID     string `gorm:"index;size:36;not null"`
	TargetType string `gorm:"size:16;not null"` // image | album
	TargetID   string `gorm:"index;size:36;not null"`
	// PasswordHash 为空表示无需密码。
	PasswordHash string     `gorm:"size:100;not null;default:''"`
	ExpiresAt    *time.Time `gorm:"index"`
	// MaxViews 为 0 表示不限次数。
	MaxViews  int64 `gorm:"not null;default:0"`
	ViewCount int64 `gorm:"not null;default:0"`
	Disabled  bool  `gorm:"not null;default:false"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName 返回存储 Share 的表名。
func (Share) TableName() string {
	return "shares"
}

// Announcement 是站点公告。
type Announcement struct {
	ID        string `gorm:"primaryKey;size:36"`
	Title     string `gorm:"size:200;not null"`
	Content   string `gorm:"type:text;not null;default:''"`
	Level     string `gorm:"size:16;not null;default:'info'"` // info | success | warning | danger
	Pinned    bool   `gorm:"not null;default:false"`
	Published bool   `gorm:"not null;default:true"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName 返回存储 Announcement 的表名。
func (Announcement) TableName() string {
	return "announcements"
}

// 举报状态。
const (
	// ReportPending 表示举报待处理。
	ReportPending = "pending"
	// ReportResolved 表示举报已处理。
	ReportResolved = "resolved"
	// ReportRejected 表示举报被驳回。
	ReportRejected = "rejected"
)

// Report 是用户对某张图片提交的举报。
type Report struct {
	ID       string  `gorm:"primaryKey;size:36"`
	ImageID  *string `gorm:"index;size:36"`
	Reporter *string `gorm:"index;size:36"`
	Reason   string  `gorm:"size:100;not null"`
	Detail   string  `gorm:"size:1000;not null;default:''"`
	Status   string  `gorm:"size:16;not null;default:'pending'"`
	// HandlerNote 为处理备注。
	HandlerNote string `gorm:"size:1000;not null;default:''"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TableName 返回存储 Report 的表名。
func (Report) TableName() string {
	return "reports"
}

// Page 是管理员维护的独立页面，可通过 slug 公开访问。
type Page struct {
	ID        string `gorm:"primaryKey;size:36"`
	Slug      string `gorm:"uniqueIndex;size:100;not null"`
	Title     string `gorm:"size:200;not null"`
	Content   string `gorm:"type:text;not null;default:''"`
	Published bool   `gorm:"not null;default:true"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName 返回存储 Page 的表名。
func (Page) TableName() string {
	return "pages"
}

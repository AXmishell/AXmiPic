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
	// Email 为已绑定的邮箱（未验证时仅作暂存）；空字符串表示未绑定。
	Email         string `gorm:"size:255;not null;default:''"`
	EmailVerified bool   `gorm:"not null;default:false"`
	// TOTPSecret 为 TOTP 密钥的密文；TOTPEnabled 表示二次验证是否已启用。
	TOTPSecret  string `gorm:"type:text;not null;default:''"`
	TOTPEnabled bool   `gorm:"not null;default:false"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
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
	// Email 为已绑定的邮箱（未验证时仅作暂存）；空字符串表示未绑定。
	Email         string `gorm:"size:255;not null;default:''"`
	EmailVerified bool   `gorm:"not null;default:false"`
	// TOTPSecret 为 TOTP 密钥的密文；TOTPEnabled 表示二次验证是否已启用。
	TOTPSecret  string `gorm:"type:text;not null;default:''"`
	TOTPEnabled bool   `gorm:"not null;default:false"`
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

// Setting 是一条运行时可修改的键值设置。敏感值（如 SMTP 密码）在写入前
// 由服务层加密，数据库只保存密文。
type Setting struct {
	Key       string `gorm:"primaryKey;size:64"`
	Value     string `gorm:"type:text;not null;default:''"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName 返回存储 Setting 的表名。
func (Setting) TableName() string {
	return "settings"
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

// Plan 是可供用户购买或订阅的套餐。套餐携带一份覆盖默认角色组策略的策略
// 描述，购买后应用到用户所属的角色组。
type Plan struct {
	ID          string `gorm:"primaryKey;size:36"`
	Name        string `gorm:"uniqueIndex;size:64;not null"`
	Description string `gorm:"size:255;not null;default:''"`
	// PriceCents 为价格，单位为分。
	PriceCents int64 `gorm:"not null;default:0"`
	// DurationDays 为有效期天数；0 表示永久。
	DurationDays int `gorm:"not null;default:0"`
	// QuotaMB 为套餐附带的存储配额（MiB）；0 表示不限。
	QuotaMB int64 `gorm:"not null;default:0"`
	// RoleGroupID 为购买后应用的角色组；为空表示仅调整配额。
	RoleGroupID *string `gorm:"size:36;null"`
	Active      bool    `gorm:"not null;default:true"`
	// SortOrder 控制展示顺序。
	SortOrder int `gorm:"not null;default:0"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName 返回存储 Plan 的表名。
func (Plan) TableName() string {
	return "plans"
}

// 订单状态。
const (
	// OrderPending 表示订单待支付。
	OrderPending = "pending"
	// OrderPaid 表示订单已支付。
	OrderPaid = "paid"
	// OrderCancelled 表示订单已取消。
	OrderCancelled = "cancelled"
)

// Order 是用户对某个套餐的购买订单。
type Order struct {
	ID     string `gorm:"primaryKey;size:36"`
	UserID string `gorm:"index;size:36;not null"`
	PlanID string `gorm:"index;size:36;not null"`
	// PlanSnapshot 记录下单时的套餐名称与价格，避免后续改价影响历史订单。
	PlanName    string `gorm:"size:64;not null"`
	AmountCents int64  `gorm:"not null;default:0"`
	// CouponID 为使用的优惠券；为空表示未使用。
	CouponID *string `gorm:"index;size:36"`
	// DiscountCents 为优惠券抵扣的金额（分）。
	DiscountCents int64  `gorm:"not null;default:0"`
	Status        string `gorm:"size:16;not null;default:'pending'"`
	// Provider 为支付渠道：manual、alipay、wechat、mock 等。
	Provider string `gorm:"size:16;not null;default:'manual'"`
	// TradeNo 为支付渠道返回的流水号。
	TradeNo   string `gorm:"size:128;not null;default:''"`
	PaidAt    *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName 返回存储 Order 的表名。
func (Order) TableName() string {
	return "orders"
}

// 优惠券类型。
const (
	// CouponFixed 表示固定金额减免（分）。
	CouponFixed = "fixed"
	// CouponPercent 表示按比例减免（PercentOff 为百分比扣减，例如 20 表示减 20%）。
	CouponPercent = "percent"
)

// Coupon 是管理员创建的优惠券。
type Coupon struct {
	ID   string `gorm:"primaryKey;size:36"`
	Code string `gorm:"uniqueIndex;size:64;not null"`
	// Type 为 fixed 或 percent。
	Type string `gorm:"size:16;not null"`
	// Value 对 fixed 为减免金额（分），对 percent 为折扣百分比。
	Value int64 `gorm:"not null;default:0"`
	// MinAmountCents 为使用门槛（分）；0 表示无门槛。
	MinAmountCents int64 `gorm:"not null;default:0"`
	// MaxUses 为最大使用次数；0 表示不限。
	MaxUses int64 `gorm:"not null;default:0"`
	Used    int64 `gorm:"not null;default:0"`
	// PerUserLimit 为每用户可用次数；0 表示不限。
	PerUserLimit int64      `gorm:"not null;default:0"`
	ExpiresAt    *time.Time `gorm:"index"`
	Active       bool       `gorm:"not null;default:true"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// TableName 返回存储 Coupon 的表名。
func (Coupon) TableName() string {
	return "coupons"
}

// CouponRedemption 记录一次优惠券使用，用于约束每用户使用次数。
type CouponRedemption struct {
	ID        string `gorm:"primaryKey;size:36"`
	CouponID  string `gorm:"index;size:36;not null"`
	UserID    string `gorm:"index;size:36;not null"`
	OrderID   string `gorm:"index;size:36;not null"`
	CreatedAt time.Time
}

// TableName 返回存储 CouponRedemption 的表名。
func (CouponRedemption) TableName() string {
	return "coupon_redemptions"
}

// 工单状态。
const (
	// TicketOpen 表示工单待处理。
	TicketOpen = "open"
	// TicketAnswered 表示已回复。
	TicketAnswered = "answered"
	// TicketClosed 表示已关闭。
	TicketClosed = "closed"
)

// Ticket 是用户提交的工单。
type Ticket struct {
	ID      string `gorm:"primaryKey;size:36"`
	UserID  string `gorm:"index;size:36;not null"`
	Subject string `gorm:"size:200;not null"`
	// Category 为工单分类。
	Category string `gorm:"size:32;not null;default:''"`
	Status   string `gorm:"size:16;not null;default:'open'"`
	// Priority 为优先级：low、normal、high。
	Priority  string `gorm:"size:16;not null;default:'normal'"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName 返回存储 Ticket 的表名。
func (Ticket) TableName() string {
	return "tickets"
}

// TicketMessage 是工单中的一条消息。
type TicketMessage struct {
	ID       string `gorm:"primaryKey;size:36"`
	TicketID string `gorm:"index;size:36;not null"`
	AuthorID string `gorm:"size:36;not null;default:''"`
	// AuthorRole 为 author 或 admin。
	AuthorRole string `gorm:"size:16;not null;default:'author'"`
	Body       string `gorm:"type:text;not null;default:''"`
	CreatedAt  time.Time
}

// TableName 返回存储 TicketMessage 的表名。
func (TicketMessage) TableName() string {
	return "ticket_messages"
}

package axmipic

import (
	"encoding/json"
	"time"
)

// User 是账户在 API 中的表示形式。
type User struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	Role        string    `json:"role"`
	Disabled    bool      `json:"disabled"`
	UsedBytes   int64     `json:"used_bytes"`
	QuotaBytes  int64     `json:"quota_bytes"`
	RoleGroupID string    `json:"role_group_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// Session 由登录操作返回。
type Session struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      User      `json:"user"`
}

// Token 是 API 令牌的表示形式。明文 Token 仅在创建时返回。
type Token struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Token      string     `json:"token,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// UploadPolicy 是当前账户生效的上传限制。
type UploadPolicy struct {
	RoleGroupID      string   `json:"role_group_id,omitempty"`
	RoleGroupName    string   `json:"role_group_name,omitempty"`
	QuotaBytes       int64    `json:"quota_bytes"`
	UploadMaxBytes   int64    `json:"upload_max_bytes"`
	AllowedMIMETypes []string `json:"allowed_mime_types"`
	Rate             struct {
		UploadPerMinute int `json:"upload_per_minute"`
		UploadBurst     int `json:"upload_burst"`
		ImagePerMinute  int `json:"image_per_minute"`
		ImageBurst      int `json:"image_burst"`
	} `json:"rate"`
	Processing struct {
		Enabled        bool     `json:"enabled"`
		MaxWidth       int      `json:"max_width"`
		MaxHeight      int      `json:"max_height"`
		DefaultQuality int      `json:"default_quality"`
		AllowedFormats []string `json:"allowed_formats"`
	} `json:"processing"`
	Features []string `json:"features"`
}

// HasFeature 报告某个功能开关是否生效。
func (p UploadPolicy) HasFeature(name string) bool {
	for _, f := range p.Features {
		if f == name {
			return true
		}
	}
	return false
}

// Image 是已存储图片的表示形式。
type Image struct {
	ID            string    `json:"id"`
	Key           string    `json:"key"`
	StorageID     string    `json:"storage_id,omitempty"`
	AlbumID       string    `json:"album_id,omitempty"`
	Permission    string    `json:"permission"`
	OwnerUsername string    `json:"owner_username,omitempty"`
	OriginalName  string    `json:"original_name"`
	Filename      string    `json:"filename"`
	Hash          string    `json:"hash"`
	URL           string    `json:"url"`
	Size          int64     `json:"size"`
	MimeType      string    `json:"mime_type"`
	Width         int       `json:"width"`
	Height        int       `json:"height"`
	CreatedAt     time.Time `json:"created_at"`
}

// ImageList 是图片的分页集合。
type ImageList struct {
	Items    []Image `json:"items"`
	Total    int64   `json:"total"`
	Page     int     `json:"page"`
	PageSize int     `json:"page_size"`
}

// Album 是相册的表示形式。
type Album struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Intro         string    `json:"intro"`
	Permission    string    `json:"permission"`
	OwnerID       string    `json:"owner_id,omitempty"`
	OwnerUsername string    `json:"owner_username,omitempty"`
	ImageCount    int64     `json:"image_count"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// PublicProfile 是用户的公开资料。
type PublicProfile struct {
	ID               string    `json:"id"`
	Username         string    `json:"username"`
	JoinedAt         time.Time `json:"joined_at"`
	PublicImageCount int64     `json:"public_image_count"`
	PublicAlbums     []Album   `json:"public_albums"`
}

// PresignResult 告知客户端如何把对象直传到存储。
type PresignResult struct {
	Key       string            `json:"key"`
	URL       string            `json:"url"`
	UploadURL string            `json:"upload_url"`
	Method    string            `json:"method"`
	Fields    map[string]string `json:"fields,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	ExpiresAt time.Time         `json:"expires_at"`
}

// Share 是分享链接的表示形式。
type Share struct {
	ID          string     `json:"id"`
	Token       string     `json:"token"`
	URL         string     `json:"url"`
	TargetType  string     `json:"target_type"`
	TargetID    string     `json:"target_id"`
	HasPassword bool       `json:"has_password"`
	Disabled    bool       `json:"disabled"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	MaxViews    int64      `json:"max_views"`
	ViewCount   int64      `json:"view_count"`
	CreatedAt   time.Time  `json:"created_at"`
}

// SharePayload 是通过密码校验后返回的分享内容。
type SharePayload struct {
	Share  Share   `json:"share"`
	Image  *Image  `json:"image,omitempty"`
	Album  *Album  `json:"album,omitempty"`
	Images []Image `json:"images,omitempty"`
}

// Announcement 是站内公告。
type Announcement struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Level     string    `json:"level"`
	Pinned    bool      `json:"pinned"`
	Published bool      `json:"published"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Report 是举报的表示形式。
type Report struct {
	ID           string    `json:"id"`
	ImageID      string    `json:"image_id,omitempty"`
	ReporterID   string    `json:"reporter_id,omitempty"`
	ReporterName string    `json:"reporter_name,omitempty"`
	Reason       string    `json:"reason"`
	Detail       string    `json:"detail"`
	Status       string    `json:"status"`
	HandlerNote  string    `json:"handler_note"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Page 是管理员维护的独立页面。
type Page struct {
	ID        string    `json:"id"`
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Published bool      `json:"published"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Plan 是套餐。
type Plan struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	PriceCents   int64     `json:"price_cents"`
	DurationDays int       `json:"duration_days"`
	QuotaMB      int64     `json:"quota_mb"`
	RoleGroupID  string    `json:"role_group_id,omitempty"`
	Active       bool      `json:"active"`
	SortOrder    int       `json:"sort_order"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Coupon 是优惠券。
type Coupon struct {
	ID             string     `json:"id"`
	Code           string     `json:"code"`
	Type           string     `json:"type"`
	Value          int64      `json:"value"`
	MinAmountCents int64      `json:"min_amount_cents"`
	MaxUses        int64      `json:"max_uses"`
	Used           int64      `json:"used"`
	PerUserLimit   int64      `json:"per_user_limit"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	Active         bool       `json:"active"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// Order 是套餐订单。
type Order struct {
	ID            string     `json:"id"`
	UserID        string     `json:"user_id"`
	PlanID        string     `json:"plan_id"`
	PlanName      string     `json:"plan_name"`
	AmountCents   int64      `json:"amount_cents"`
	DiscountCents int64      `json:"discount_cents"`
	Status        string     `json:"status"`
	Provider      string     `json:"provider"`
	TradeNo       string     `json:"trade_no,omitempty"`
	PayURL        string     `json:"pay_url,omitempty"`
	PaidAt        *time.Time `json:"paid_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// TicketMessage 是工单中的一条消息。
type TicketMessage struct {
	ID         string    `json:"id"`
	AuthorID   string    `json:"author_id"`
	AuthorRole string    `json:"author_role"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}

// Ticket 是工单。
type Ticket struct {
	ID        string          `json:"id"`
	UserID    string          `json:"user_id"`
	Username  string          `json:"username,omitempty"`
	Subject   string          `json:"subject"`
	Category  string          `json:"category"`
	Status    string          `json:"status"`
	Priority  string          `json:"priority"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	Messages  []TicketMessage `json:"messages,omitempty"`
}

// Policy 是角色组策略。
type Policy struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	Description string          `json:"description"`
	Enabled     bool            `json:"enabled"`
	Settings    json.RawMessage `json:"settings"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// RoleGroup 是角色组。
type RoleGroup struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	IsDefault     bool      `json:"is_default"`
	CustomerCount int64     `json:"customer_count"`
	PolicyCount   int64     `json:"policy_count"`
	Policies      []Policy  `json:"policies"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// StorageBackend 是存储后端。
type StorageBackend struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Driver     string          `json:"driver"`
	IsCurrent  bool            `json:"is_current"`
	Settings   map[string]any  `json:"settings"`
	SecretsSet map[string]bool `json:"secrets_set"`
	ImageCount int64           `json:"image_count"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// Stats 是实例级统计。
type Stats struct {
	Admins        int64    `json:"admins"`
	Customers     int64    `json:"customers"`
	Users         int64    `json:"users"`
	Images        int64    `json:"images"`
	TotalBytes    int64    `json:"total_bytes"`
	StorageDriver string   `json:"storage_driver"`
	Processor     string   `json:"processor"`
	Formats       []string `json:"formats"`
}

export interface ApiEnvelope<T> {
  code: number
  message: string
  data: T
}

export type UserRole = 'customer' | 'admin'

/** 图片排序方式。 */
export type ImageOrder = 'newest' | 'earliest' | 'largest' | 'smallest'

/** 图片可见性：public 可出现在图片广场，private 仅本人可见（默认）。 */
export type ImagePermission = 'public' | 'private'

export interface User {
  id: string
  username: string
  role: UserRole
  used_bytes: number
  quota_bytes: number
  role_group_id?: string
  /** 已购套餐的到期时间；为空表示无套餐或永久有效。 */
  plan_expires_at?: string
  /** 已绑定的邮箱。 */
  email?: string
  /** 邮箱是否已通过验证。 */
  email_verified?: boolean
  /** 是否已启用 TOTP 二次验证。 */
  totp_enabled?: boolean
  created_at: string
  disabled?: boolean
}

export interface ImageItem {
  id: string
  key: string
  url: string
  /** 列表缩略图 URL；后端未提供时回退到 url。 */
  thumbnail?: string
  original_name: string
  filename: string
  hash: string
  size: number
  mime_type: string
  width: number
  height: number
  created_at: string
  album_id?: string
  permission: ImagePermission
  /** 图片所有者的用户名，仅在图片广场/公开相册等公开列表中返回。 */
  owner_username?: string
}

/** 相册。 */
export interface Album {
  id: string
  name: string
  intro: string
  permission: ImagePermission
  owner_id?: string
  owner_username?: string
  image_count: number
  created_at: string
  updated_at: string
}

/** 用户的公开资料。 */
export interface PublicProfile {
  id: string
  username: string
  joined_at: string
  public_image_count: number
  public_albums: Album[]
}

export interface Token {
  id: string
  name: string
  prefix: string
  token?: string
  last_used_at?: string
  created_at: string
}

export interface CreatedToken extends Token {
  token: string
}

export interface PageData<T> {
  items: T[]
  total: number
  page: number
  page_size: number
  /** 游标分页的下一页游标；为空表示没有更多。 */
  next_cursor?: string
}

export interface AdminStats {
  admins: number
  customers: number
  users: number
  images: number
  total_bytes: number
  storage_driver: string
  processor: string
  formats: string[]
}

export interface LoginResult {
  token?: string
  expires_at?: string
  user: User
  /** 为真时表示需先完成 TOTP 二次验证。 */
  totp_required?: boolean
  /** 完成 TOTP 验证所需的短期令牌。 */
  challenge_token?: string
}

export interface Credentials {
  username: string
  password: string
}

/** 注册请求体：邮箱需先通过验证码验证。 */
export interface RegisterPayload {
  username: string
  email: string
  code: string
  password: string
}

export interface UserUpdate {
  disabled?: boolean
  /** 客户所属角色组；空字符串表示回退默认组。 */
  role_group_id?: string
}

/** 策略类型：分别控制配额、上传、速率、图片处理与功能开关。 */
export type PolicyType = 'quota' | 'upload' | 'rate' | 'processing' | 'feature'

export interface Policy {
  id: string
  name: string
  type: PolicyType
  description: string
  enabled: boolean
  settings: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface PolicyInput {
  name: string
  type: PolicyType
  description: string
  enabled: boolean
  settings: Record<string, unknown>
}

export interface RoleGroup {
  id: string
  name: string
  description: string
  is_default: boolean
  customer_count: number
  policy_count: number
  policies: Policy[]
  created_at: string
  updated_at: string
}

export interface RoleGroupInput {
  name: string
  description: string
  is_default: boolean
}

/** 某个账户最终生效的策略集合。 */
export interface EffectivePolicies {
  role_group_id?: string
  role_group_name?: string
  quota_bytes: number
  upload_max_bytes: number
  allowed_mime_types: string[]
  rate: {
    upload_per_minute: number
    upload_burst: number
    image_per_minute: number
    image_burst: number
  }
  processing: {
    enabled: boolean
    max_width: number
    max_height: number
    default_quality: number
    allowed_formats: string[]
  }
  features: string[]
}

/** 用户界面偏好（跨设备同步）。 */
export interface UserPreferences {
  /** 查看器默认显示模式：fit=适应窗口，actual=原始像素 1:1。 */
  viewer_mode?: 'fit' | 'actual'
  /** 图片广场布局：grid=网格，masonry=瀑布流。 */
  plaza_layout?: 'grid' | 'masonry'
  /** 图片管理布局：grid=网格，masonry=瀑布流。 */
  images_layout?: 'grid' | 'masonry'
}

export type StorageDriver = 'local' | 's3' | 'qiniu'

export interface StorageBackend {
  id: string
  name: string
  driver: StorageDriver
  is_current: boolean
  settings: Record<string, unknown>
  secrets_set: Record<string, boolean>
  image_count: number
  created_at: string
}

export interface StorageBackendInput {
  name: string
  driver: StorageDriver
  settings: Record<string, unknown>
  /** 敏感字段。更新时为空的字段表示保持原值。 */
  secrets: Record<string, string>
  activate?: boolean
}

/** 分享目标类型。 */
export type ShareTargetType = 'image' | 'album'

export interface Share {
  id: string
  token: string
  url: string
  target_type: ShareTargetType
  target_id: string
  has_password: boolean
  disabled: boolean
  expires_at?: string
  max_views: number
  view_count: number
  created_at: string
}

export interface ShareInput {
  target_type: ShareTargetType
  target_id: string
  password?: string
  expires_in_hours?: number
  max_views?: number
}

export interface SharePayload {
  share: Share
  image?: ImageItem
  album?: Album
  images?: ImageItem[]
}

/** 公告。 */
export type AnnouncementLevel = 'info' | 'success' | 'warning' | 'danger'

export interface Announcement {
  id: string
  title: string
  content: string
  level: AnnouncementLevel
  pinned: boolean
  published: boolean
  created_at: string
  updated_at: string
}

export interface AnnouncementInput {
  title: string
  content: string
  level: AnnouncementLevel
  pinned: boolean
  published: boolean
}

/** 举报。 */
export type ReportStatus = 'pending' | 'resolved' | 'rejected'

export interface Report {
  id: string
  image_id?: string
  reporter_id?: string
  reporter_name?: string
  reason: string
  detail: string
  status: ReportStatus
  handler_note: string
  created_at: string
  updated_at: string
}

export interface ReportInput {
  image_id?: string
  reason: string
  detail?: string
}

/** 独立页面。 */
export interface Page {
  id: string
  slug: string
  title: string
  content: string
  published: boolean
  created_at: string
  updated_at: string
}

export interface PageInput {
  slug: string
  title: string
  content: string
  published: boolean
}

/** 套餐。 */
export interface Plan {
  id: string
  name: string
  description: string
  price_cents: number
  duration_days: number
  quota_mb: number
  role_group_id?: string
  active: boolean
  sort_order: number
  created_at: string
  updated_at: string
}

export interface PlanInput {
  name: string
  description: string
  price_cents: number
  duration_days: number
  quota_mb: number
  role_group_id?: string
  active: boolean
  sort_order: number
}

/** 优惠券。 */
export type CouponType = 'fixed' | 'percent'

export interface Coupon {
  id: string
  code: string
  type: CouponType
  value: number
  min_amount_cents: number
  max_uses: number
  used: number
  per_user_limit: number
  expires_at?: string
  active: boolean
  created_at: string
  updated_at: string
}

export interface CouponInput {
  code: string
  type: CouponType
  value: number
  min_amount_cents: number
  max_uses: number
  per_user_limit: number
  expires_at?: string | null
  active: boolean
}

/** 订单。 */
export type OrderStatus = 'pending' | 'paid' | 'cancelled'

export interface Order {
  id: string
  user_id: string
  plan_id: string
  plan_name: string
  amount_cents: number
  discount_cents: number
  status: OrderStatus
  provider: string
  trade_no?: string
  pay_url?: string
  paid_at?: string
  created_at: string
  updated_at: string
}

/** 工单。 */
export type TicketStatus = 'open' | 'answered' | 'closed'
export type TicketPriority = 'low' | 'normal' | 'high'

export interface TicketMessage {
  id: string
  author_id: string
  author_role: 'author' | 'admin'
  body: string
  created_at: string
}

export interface Ticket {
  id: string
  user_id: string
  username?: string
  subject: string
  category: string
  status: TicketStatus
  priority: TicketPriority
  created_at: string
  updated_at: string
  messages?: TicketMessage[]
}

export interface TicketInput {
  subject: string
  category: string
  body: string
  priority: TicketPriority
}

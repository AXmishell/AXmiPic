/** AXmiPic API 的公共类型定义。 */

export interface User {
  id: string
  username: string
  role: 'customer' | 'admin'
  disabled: boolean
  used_bytes: number
  quota_bytes: number
  role_group_id?: string
  created_at: string
}

export interface Session {
  token: string
  expires_at: string
  user: User
}

export interface Token {
  id: string
  name: string
  prefix: string
  token?: string
  last_used_at?: string
  created_at: string
}

export interface UploadPolicy {
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

export interface ImageItem {
  id: string
  key: string
  storage_id?: string
  album_id?: string
  permission: 'public' | 'private'
  owner_username?: string
  original_name: string
  filename: string
  hash: string
  url: string
  size: number
  mime_type: string
  width: number
  height: number
  created_at: string
}

export interface PageData<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

export interface Album {
  id: string
  name: string
  intro: string
  permission: 'public' | 'private'
  owner_id?: string
  owner_username?: string
  image_count: number
  created_at: string
  updated_at: string
}

export interface PublicProfile {
  id: string
  username: string
  joined_at: string
  public_image_count: number
  public_albums: Album[]
}

export interface PresignResult {
  key: string
  url: string
  upload_url: string
  method: string
  fields?: Record<string, string>
  headers?: Record<string, string>
  expires_at: string
}

export interface Share {
  id: string
  token: string
  url: string
  target_type: 'image' | 'album'
  target_id: string
  has_password: boolean
  disabled: boolean
  expires_at?: string
  max_views: number
  view_count: number
  created_at: string
}

export interface SharePayload {
  share: Share
  image?: ImageItem
  album?: Album
  images?: ImageItem[]
}

export interface Announcement {
  id: string
  title: string
  content: string
  level: 'info' | 'success' | 'warning' | 'danger'
  pinned: boolean
  published: boolean
  created_at: string
  updated_at: string
}

export interface Report {
  id: string
  image_id?: string
  reporter_id?: string
  reporter_name?: string
  reason: string
  detail: string
  status: 'pending' | 'resolved' | 'rejected'
  handler_note: string
  created_at: string
  updated_at: string
}

export interface Page {
  id: string
  slug: string
  title: string
  content: string
  published: boolean
  created_at: string
  updated_at: string
}

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

export interface Coupon {
  id: string
  code: string
  type: 'fixed' | 'percent'
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

export interface Order {
  id: string
  user_id: string
  plan_id: string
  plan_name: string
  amount_cents: number
  discount_cents: number
  status: 'pending' | 'paid' | 'cancelled'
  provider: string
  trade_no?: string
  pay_url?: string
  paid_at?: string
  created_at: string
  updated_at: string
}

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
  status: 'open' | 'answered' | 'closed'
  priority: 'low' | 'normal' | 'high'
  created_at: string
  updated_at: string
  messages?: TicketMessage[]
}

export interface Policy {
  id: string
  name: string
  type: 'quota' | 'upload' | 'rate' | 'processing' | 'feature'
  description: string
  enabled: boolean
  settings: Record<string, unknown>
  created_at: string
  updated_at: string
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

export interface StorageBackend {
  id: string
  name: string
  driver: 'local' | 's3' | 'qiniu'
  is_current: boolean
  settings: Record<string, unknown>
  secrets_set: Record<string, boolean>
  image_count: number
  created_at: string
}

export interface Stats {
  admins: number
  customers: number
  users: number
  images: number
  total_bytes: number
  storage_driver: string
  processor: string
  formats: string[]
}

export interface CouponValidation {
  coupon: Coupon
  discount_cents: number
}

export interface ImagingDrivers {
  available: string[]
  active: string
}

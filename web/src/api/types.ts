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
  created_at: string
  disabled?: boolean
}

export interface ImageItem {
  id: string
  key: string
  url: string
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
}

/** 相册。 */
export interface Album {
  id: string
  name: string
  intro: string
  image_count: number
  created_at: string
  updated_at: string
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
  token: string
  expires_at: string
  user: User
}

export interface Credentials {
  username: string
  password: string
}

export interface UserUpdate {
  disabled?: boolean
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

export interface ApiEnvelope<T> {
  code: number
  message: string
  data: T
}

export type UserRole = 'customer' | 'admin'

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
  size: number
  mime_type: string
  width: number
  height: number
  created_at: string
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

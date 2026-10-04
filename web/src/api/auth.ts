import { request } from './client'
import type { Credentials, EffectivePolicies, LoginResult, User } from './types'

export function login(payload: Credentials): Promise<LoginResult> {
  return request<LoginResult>({ method: 'POST', url: '/auth/login', data: payload })
}

export function adminLogin(payload: Credentials): Promise<LoginResult> {
  return request<LoginResult>({ method: 'POST', url: '/admin/auth/login', data: payload })
}

export function register(payload: Credentials): Promise<User> {
  return request<User>({ method: 'POST', url: '/auth/register', data: payload })
}

export function fetchMe(): Promise<User> {
  return request<User>({ method: 'GET', url: '/auth/me' })
}

/** 当前账户最终生效的角色策略。 */
export function fetchPolicies(): Promise<EffectivePolicies> {
  return request<EffectivePolicies>({ method: 'GET', url: '/auth/policies' })
}

/** 安装状态。 */
export interface InstallStatus {
  installed: boolean
  lock_file?: string
  reasons?: string[]
}

/** 安装向导提交的配置。 */
export interface InstallInput {
  site_name: string
  base_url: string
  database_driver: string
  database_dsn: string
  admin_username: string
  admin_password: string
  storage_driver: string
  storage_root: string
  allow_registration: boolean
  allow_guest_upload: boolean
}

/** 读取安装状态（无需登录）。 */
export function fetchInstallStatus(): Promise<InstallStatus> {
  return request<InstallStatus>({ method: 'GET', url: '/install/status' })
}

/** 执行安装初始化。 */
export function runInstall(input: InstallInput): Promise<{ installed: boolean }> {
  return request<{ installed: boolean }>({ method: 'POST', url: '/install', data: input })
}

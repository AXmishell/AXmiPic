import { request } from './client'
import type { Credentials, EffectivePolicies, LoginResult, RegisterPayload, User } from './types'

export function login(payload: Credentials): Promise<LoginResult> {
  return request<LoginResult>({ method: 'POST', url: '/auth/login', data: payload })
}

export function adminLogin(payload: Credentials): Promise<LoginResult> {
  return request<LoginResult>({ method: 'POST', url: '/admin/auth/login', data: payload })
}

/** 向邮箱发送注册验证码（无需登录）。 */
export function sendRegisterCode(email: string): Promise<{ status: string }> {
  return request<{ status: string }>({ method: 'POST', url: '/auth/register/code', data: { email } })
}

export function register(payload: RegisterPayload): Promise<User> {
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

/** 执行安装初始化。token 为启动日志中输出的安装令牌。 */
export function runInstall(input: InstallInput, token: string): Promise<{ installed: boolean }> {
  return request<{ installed: boolean }>({
    method: 'POST',
    url: '/install',
    data: input,
    headers: token ? { 'X-Install-Token': token } : undefined,
  })
}

// ---- 二次验证（TOTP）与邮箱绑定 ----

/** 账户安全设置状态。 */
export interface SecurityInfo {
  email: string
  email_verified: boolean
  totp_enabled: boolean
  /** 服务端是否具备启用 TOTP 的条件（已配置加密主密钥）。 */
  totp_available: boolean
  /** 服务端是否已配置邮件渠道。 */
  email_available: boolean
}

/** 开始配置 TOTP 时返回的密钥与 otpauth 链接。 */
export interface TOTPSetup {
  secret: string
  uri: string
}

/** 读取当前账户的安全设置状态。 */
export function fetchSecurity(): Promise<SecurityInfo> {
  return request<SecurityInfo>({ method: 'GET', url: '/auth/security' })
}

/** 生成新的 TOTP 密钥（尚未启用）。 */
export function setupTOTP(): Promise<TOTPSetup> {
  return request<TOTPSetup>({ method: 'POST', url: '/auth/totp/setup' })
}

/** 校验动态码并启用二次验证。 */
export function enableTOTP(code: string): Promise<User> {
  return request<User>({ method: 'POST', url: '/auth/totp/enable', data: { code } })
}

/** 关闭二次验证（动态码或密码二选一）。 */
export function disableTOTP(payload: { code?: string; password?: string }): Promise<User> {
  return request<User>({ method: 'POST', url: '/auth/totp/disable', data: payload })
}

/** 登录时完成 TOTP 二次验证并换取会话。 */
export function verifyTOTPLogin(challengeToken: string, code: string): Promise<LoginResult> {
  return request<LoginResult>({
    method: 'POST',
    url: '/auth/totp/verify',
    data: { challenge_token: challengeToken, code },
  })
}

/** 向目标邮箱发送验证码。 */
export function sendEmailCode(email: string): Promise<{ status: string }> {
  return request<{ status: string }>({ method: 'POST', url: '/auth/email/code', data: { email } })
}

/** 校验验证码并绑定邮箱；换绑已绑定的邮箱时需提供当前密码。 */
export function verifyEmail(email: string, code: string, password?: string): Promise<User> {
  return request<User>({ method: 'POST', url: '/auth/email/verify', data: { email, code, password } })
}

/** 解绑邮箱（需当前密码）。 */
export function unbindEmail(password: string): Promise<User> {
  return request<User>({ method: 'POST', url: '/auth/email/unbind', data: { password } })
}

/** 修改当前账户密码（需当前密码）。 */
export function changePassword(currentPassword: string, newPassword: string): Promise<{ status: string }> {
  return request<{ status: string }>({
    method: 'POST',
    url: '/auth/password',
    data: { current_password: currentPassword, new_password: newPassword },
  })
}

/** 向邮箱发送密码重置验证码（无需登录）。 */
export function sendPasswordResetCode(email: string): Promise<{ status: string }> {
  return request<{ status: string }>({ method: 'POST', url: '/auth/password/reset/code', data: { email } })
}

/** 校验验证码并重置密码（无需登录）。 */
export function resetPassword(email: string, code: string, newPassword: string): Promise<{ status: string }> {
  return request<{ status: string }>({
    method: 'POST',
    url: '/auth/password/reset',
    data: { email, code, new_password: newPassword },
  })
}

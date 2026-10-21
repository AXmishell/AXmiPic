import { request } from './client'

export interface AdminEntryInfo {
  enabled: boolean
  url?: string
  updated_at?: string
}

export interface AdminEntryAuthPayload {
  current_password: string
  totp_code?: string
}

/** 读取管理员安全入口配置。 */
export function getAdminEntry(): Promise<AdminEntryInfo> {
  return request<AdminEntryInfo>({ method: 'GET', url: '/admin/security/admin-entry' })
}

/** 开启或关闭管理员安全入口。 */
export function updateAdminEntry(
  enabled: boolean,
  payload: AdminEntryAuthPayload,
): Promise<AdminEntryInfo> {
  return request<AdminEntryInfo>({
    method: 'PUT',
    url: '/admin/security/admin-entry',
    data: { enabled, ...payload },
  })
}

/** 重新生成管理员安全入口 token。 */
export function rotateAdminEntry(payload: AdminEntryAuthPayload): Promise<AdminEntryInfo> {
  return request<AdminEntryInfo>({
    method: 'POST',
    url: '/admin/security/admin-entry/rotate',
    data: payload,
  })
}

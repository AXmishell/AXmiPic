import { request } from './client'
import type { AdminStats, Credentials, User, UserUpdate } from './types'

export function fetchStats(): Promise<AdminStats> {
  return request<AdminStats>({ method: 'GET', url: '/admin/stats' })
}

/** 普通账号。 */
export function listCustomers(): Promise<User[]> {
  return request<User[]>({ method: 'GET', url: '/admin/customers' })
}

/** 特权账号。 */
export function listAdmins(): Promise<User[]> {
  return request<User[]>({ method: 'GET', url: '/admin/admins' })
}

export function createAdmin(payload: Credentials): Promise<User> {
  return request<User>({ method: 'POST', url: '/admin/admins', data: payload })
}

export function updateCustomer(id: string, payload: UserUpdate): Promise<User> {
  return request<User>({ method: 'PATCH', url: `/admin/customers/${id}`, data: payload })
}

export function updateAdmin(id: string, payload: UserUpdate): Promise<User> {
  return request<User>({ method: 'PATCH', url: `/admin/admins/${id}`, data: payload })
}

export function deleteCustomer(id: string): Promise<void> {
  return request<void>({ method: 'DELETE', url: `/admin/customers/${id}` })
}

export function deleteAdmin(id: string): Promise<void> {
  return request<void>({ method: 'DELETE', url: `/admin/admins/${id}` })
}

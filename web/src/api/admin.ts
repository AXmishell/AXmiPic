import { request } from './client'
import type { AdminStats, User, UserUpdate } from './types'

export function fetchStats(): Promise<AdminStats> {
  return request<AdminStats>({ method: 'GET', url: '/admin/stats' })
}

export function listUsers(): Promise<User[]> {
  return request<User[]>({ method: 'GET', url: '/admin/users' })
}

export function updateUser(id: number, payload: UserUpdate): Promise<User> {
  return request<User>({ method: 'PATCH', url: `/admin/users/${id}`, data: payload })
}

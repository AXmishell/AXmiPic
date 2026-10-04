import { request } from './client'
import type {
  AdminStats,
  Credentials,
  Policy,
  PolicyInput,
  RoleGroup,
  RoleGroupInput,
  StorageBackend,
  StorageBackendInput,
  User,
  UserUpdate,
} from './types'

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

/** 存储后端配置。 */
export function listStorage(): Promise<StorageBackend[]> {
  return request<StorageBackend[]>({ method: 'GET', url: '/admin/storage' })
}

export function createStorage(payload: StorageBackendInput): Promise<StorageBackend> {
  return request<StorageBackend>({ method: 'POST', url: '/admin/storage', data: payload })
}

export function updateStorage(id: string, payload: StorageBackendInput): Promise<StorageBackend> {
  return request<StorageBackend>({ method: 'PUT', url: `/admin/storage/${id}`, data: payload })
}

export function deleteStorage(id: string): Promise<void> {
  return request<void>({ method: 'DELETE', url: `/admin/storage/${id}` })
}

export function activateStorage(id: string): Promise<void> {
  return request<void>({ method: 'POST', url: `/admin/storage/${id}/activate` })
}

/** 角色组。 */
export function listRoleGroups(): Promise<RoleGroup[]> {
  return request<RoleGroup[]>({ method: 'GET', url: '/admin/role-groups' })
}

export function createRoleGroup(payload: RoleGroupInput): Promise<RoleGroup> {
  return request<RoleGroup>({ method: 'POST', url: '/admin/role-groups', data: payload })
}

export function updateRoleGroup(id: string, payload: RoleGroupInput): Promise<RoleGroup> {
  return request<RoleGroup>({ method: 'PUT', url: `/admin/role-groups/${id}`, data: payload })
}

export function deleteRoleGroup(id: string): Promise<void> {
  return request<void>({ method: 'DELETE', url: `/admin/role-groups/${id}` })
}

export function attachPolicy(roleGroupId: string, policyId: string): Promise<RoleGroup> {
  return request<RoleGroup>({
    method: 'POST',
    url: `/admin/role-groups/${roleGroupId}/policies`,
    data: { policy_id: policyId },
  })
}

export function detachPolicy(roleGroupId: string, policyId: string): Promise<RoleGroup> {
  return request<RoleGroup>({
    method: 'DELETE',
    url: `/admin/role-groups/${roleGroupId}/policies/${policyId}`,
  })
}

/** 策略。 */
export function listPolicies(type?: string): Promise<Policy[]> {
  return request<Policy[]>({ method: 'GET', url: '/admin/policies', params: type ? { type } : undefined })
}

export function createPolicy(payload: PolicyInput): Promise<Policy> {
  return request<Policy>({ method: 'POST', url: '/admin/policies', data: payload })
}

export function updatePolicy(id: string, payload: PolicyInput): Promise<Policy> {
  return request<Policy>({ method: 'PUT', url: `/admin/policies/${id}`, data: payload })
}

export function deletePolicy(id: string): Promise<void> {
  return request<void>({ method: 'DELETE', url: `/admin/policies/${id}` })
}

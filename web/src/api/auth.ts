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

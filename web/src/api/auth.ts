import { request } from './client'
import type { Credentials, LoginResult, User } from './types'

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

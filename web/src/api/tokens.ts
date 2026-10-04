import { request } from './client'
import type { CreatedToken, Token } from './types'

export function listTokens(): Promise<Token[]> {
  return request<Token[]>({ method: 'GET', url: '/tokens' })
}

export function createToken(name: string): Promise<CreatedToken> {
  return request<CreatedToken>({ method: 'POST', url: '/tokens', data: { name } })
}

export function revokeToken(id: string): Promise<void> {
  return request<void>({ method: 'DELETE', url: `/tokens/${id}` })
}

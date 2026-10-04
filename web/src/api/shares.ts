import { request } from './client'
import type { Share, ShareInput, SharePayload } from './types'

/** 为图片或相册创建分享链接。 */
export function createShare(input: ShareInput): Promise<Share> {
  return request<Share>({ method: 'POST', url: '/shares', data: input })
}

/** 当前账号创建的分享。 */
export function listShares(): Promise<Share[]> {
  return request<Share[]>({ method: 'GET', url: '/shares' })
}

export function revokeShare(id: string): Promise<void> {
  return request<void>({ method: 'DELETE', url: `/shares/${id}` })
}

/** 分享的公开元信息（无需登录）。 */
export function shareInfo(token: string): Promise<Share> {
  return request<Share>({ method: 'GET', url: `/shares/${token}` })
}

/** 校验密码并获取分享内容。 */
export function shareAccess(token: string, password = ''): Promise<SharePayload> {
  return request<SharePayload>({
    method: 'POST',
    url: `/shares/${token}/access`,
    data: { password },
  })
}

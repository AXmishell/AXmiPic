import { request } from './client'
import type { Album, ImagePermission, PageData, PublicProfile, ImageItem } from './types'

/** 创建或更新相册的输入。 */
export interface AlbumInput {
  name: string
  intro: string
  permission: ImagePermission
}

export function listAlbums(): Promise<Album[]> {
  return request<Album[]>({ method: 'GET', url: '/albums' })
}

/** 公开相册列表（无需登录），可按所有者过滤。 */
export function listPublicAlbums(userId?: string): Promise<Album[]> {
  return request<Album[]>({
    method: 'GET',
    url: '/plaza/albums',
    params: userId ? { user_id: userId } : undefined,
  })
}

export function getAlbum(id: string): Promise<Album> {
  return request<Album>({ method: 'GET', url: `/albums/${id}` })
}

/** 相册中的图片；公开相册无需登录即可浏览。 */
export function listAlbumImages(id: string, page = 1, pageSize = 24): Promise<PageData<ImageItem>> {
  return request<PageData<ImageItem>>({
    method: 'GET',
    url: `/albums/${id}/images`,
    params: { page, page_size: pageSize },
  })
}

export function createAlbum(input: AlbumInput): Promise<Album> {
  return request<Album>({ method: 'POST', url: '/albums', data: input })
}

export function updateAlbum(id: string, input: AlbumInput): Promise<Album> {
  return request<Album>({ method: 'PATCH', url: `/albums/${id}`, data: input })
}

export function deleteAlbum(id: string): Promise<void> {
  return request<void>({ method: 'DELETE', url: `/albums/${id}` })
}

/** 获取某个用户的公开资料。 */
export function fetchPublicProfile(userId: string): Promise<PublicProfile> {
  return request<PublicProfile>({ method: 'GET', url: `/users/${userId}` })
}

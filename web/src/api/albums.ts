import { request } from './client'
import type { Album } from './types'

/** 创建或更新相册的输入。 */
export interface AlbumInput {
  name: string
  intro: string
}

export function listAlbums(): Promise<Album[]> {
  return request<Album[]>({ method: 'GET', url: '/albums' })
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

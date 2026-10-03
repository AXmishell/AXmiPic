import { request } from './client'
import type { ImageItem, PageData } from './types'

export function listImages(page: number, pageSize: number): Promise<PageData<ImageItem>> {
  return request<PageData<ImageItem>>({
    method: 'GET',
    url: '/images',
    params: { page, page_size: pageSize },
  })
}

export function deleteImage(id: number): Promise<void> {
  return request<void>({ method: 'DELETE', url: `/images/${id}` })
}

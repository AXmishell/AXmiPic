import { request } from './client'
import type { ImageItem, PageData } from './types'

/** 图片列表排序方式。 */
export type ImageOrder = 'newest' | 'earliest' | 'largest' | 'smallest'

export function listImages(
  page: number,
  pageSize: number,
  order: ImageOrder = 'newest',
  keyword = '',
): Promise<PageData<ImageItem>> {
  return request<PageData<ImageItem>>({
    method: 'GET',
    url: '/images',
    params: { page, page_size: pageSize, order, keyword: keyword || undefined },
  })
}

/** 通过后台界面上传一张图片；文件以 multipart 形式提交。 */
export function uploadImage(file: File): Promise<ImageItem> {
  const form = new FormData()
  form.append('file', file)
  return request<ImageItem>({
    method: 'POST',
    url: '/upload',
    data: form,
    headers: { 'Content-Type': 'multipart/form-data' },
  })
}

/** 重命名图片（修改展示用的原文件名）。 */
export function renameImage(id: string, name: string): Promise<ImageItem> {
  return request<ImageItem>({ method: 'PATCH', url: `/images/${id}`, data: { name } })
}

export function deleteImage(id: string): Promise<void> {
  return request<void>({ method: 'DELETE', url: `/images/${id}` })
}

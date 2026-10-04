import { request } from './client'
import type { ImageItem, PageData } from './types'

export function listImages(page: number, pageSize: number): Promise<PageData<ImageItem>> {
  return request<PageData<ImageItem>>({
    method: 'GET',
    url: '/images',
    params: { page, page_size: pageSize },
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

export function deleteImage(id: string): Promise<void> {
  return request<void>({ method: 'DELETE', url: `/images/${id}` })
}

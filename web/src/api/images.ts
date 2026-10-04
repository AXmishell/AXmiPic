import { request } from './client'
import type { ImageItem, ImageOrder, ImagePermission, PageData } from './types'

export type { ImageOrder } from './types'

/** 图片列表查询参数。 */
export interface ListImagesParams {
  page: number
  pageSize: number
  order?: ImageOrder
  keyword?: string
  albumId?: string
  permission?: ImagePermission
  /** 图片广场按作者过滤。 */
  userId?: string
}

/** 将查询参数转换为后端使用的下划线命名。 */
function listParams(params: ListImagesParams): Record<string, unknown> {
  return {
    page: params.page,
    page_size: params.pageSize,
    order: params.order,
    keyword: params.keyword || undefined,
    album_id: params.albumId || undefined,
    permission: params.permission || undefined,
    user_id: params.userId || undefined,
  }
}

export function listImages(params: ListImagesParams): Promise<PageData<ImageItem>> {
  return request<PageData<ImageItem>>({ method: 'GET', url: '/images', params: listParams(params) })
}

/** 图片广场：跨用户的公开图片。 */
export function listPlaza(params: ListImagesParams): Promise<PageData<ImageItem>> {
  return request<PageData<ImageItem>>({ method: 'GET', url: '/plaza', params: listParams(params) })
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

/** 批量更新图片的可见性或所属相册。 */
export interface BatchImageUpdate {
  ids: string[]
  permission?: ImagePermission
  /** 目标相册 id；与 clearAlbum 互斥。 */
  albumId?: string
  /** 将图片移出相册。 */
  clearAlbum?: boolean
}

export function batchUpdateImages(update: BatchImageUpdate): Promise<{ updated: number }> {
  return request<{ updated: number }>({
    method: 'POST',
    url: '/images/batch',
    data: {
      ids: update.ids,
      permission: update.permission,
      album_id: update.albumId,
      clear_album: update.clearAlbum || undefined,
    },
  })
}

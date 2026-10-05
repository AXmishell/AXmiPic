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
export function uploadImage(
  file: File,
  onProgress?: (percent: number) => void,
): Promise<ImageItem> {
  const form = new FormData()
  form.append('file', file)
  return request<ImageItem>({
    method: 'POST',
    url: '/upload',
    data: form,
    headers: { 'Content-Type': 'multipart/form-data' },
    onUploadProgress: (event) => {
      if (onProgress && event.total) {
        onProgress(Math.min(100, Math.round((event.loaded / event.total) * 100)))
      }
    },
  })
}

/** 重命名图片（修改展示用的原文件名）。 */
export function renameImage(id: string, name: string): Promise<ImageItem> {
  return request<ImageItem>({ method: 'PATCH', url: `/images/${id}`, data: { name } })
}

/** 即时图片处理的参数。 */
export interface TransformParams {
  width?: number
  height?: number
  fit?: 'contain' | 'cover' | 'fill'
  quality?: number
  format?: string
  rotate?: 0 | 90 | 180 | 270
  flip?: '' | 'h' | 'v' | 'hv'
  grayscale?: boolean
  blur?: number
  sharpen?: number
  enlarge?: boolean
  watermark?: string
  watermarkPosition?: string
  watermarkOpacity?: number
  watermarkSize?: number
  watermarkColor?: string
}

/**
 * 在图片 URL 上附加即时处理参数。后端 `/i/*` 接口会按查询串实时变换并缓存。
 */
export function transformUrl(baseUrl: string, params: TransformParams): string {
  const query = new URLSearchParams()
  if (params.width) query.set('w', String(params.width))
  if (params.height) query.set('h', String(params.height))
  if (params.fit && params.fit !== 'contain') query.set('fit', params.fit)
  if (params.quality) query.set('q', String(params.quality))
  if (params.format) query.set('f', params.format)
  if (params.rotate) query.set('r', String(params.rotate))
  if (params.flip) query.set('flip', params.flip)
  if (params.grayscale) query.set('gray', '1')
  if (params.blur) query.set('blur', String(params.blur))
  if (params.sharpen) query.set('sharpen', String(params.sharpen))
  if (params.enlarge) query.set('enlarge', '1')
  if (params.watermark) {
    query.set('wm', params.watermark)
    if (params.watermarkPosition) query.set('wm_pos', params.watermarkPosition)
    if (params.watermarkOpacity) query.set('wm_opacity', String(params.watermarkOpacity))
    if (params.watermarkSize) query.set('wm_size', String(params.watermarkSize))
    if (params.watermarkColor) query.set('wm_color', params.watermarkColor)
  }
  const suffix = query.toString()
  if (!suffix) return baseUrl
  return baseUrl.includes('?') ? `${baseUrl}&${suffix}` : `${baseUrl}?${suffix}`
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

export function batchUpdateImages(update: BatchImageUpdate): Promise<BatchImageResult> {
  return request<BatchImageResult>({
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

/** 批量更新的结果；public 操作时 blocked 为因 AI 审核未通过而保持私有的图片 id。 */
export interface BatchImageResult {
  updated: number
  published?: number
  blocked?: string[]
}

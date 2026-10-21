import { request } from './client'
import type { ImageItem, ImagePermission } from './types'

/** X 解析结果中的作者。 */
export interface XAuthor {
  name: string
  handle: string
}

/** X 解析结果中的一张候选图片。 */
export interface XMedia {
  id?: string
  type: string
  url: string
  width?: number
  height?: number
  alt?: string
}

/** X 帖子解析结果。 */
export interface XPost {
  platform: string
  post_id: string
  source_url: string
  author: XAuthor
  text?: string
  sensitive?: boolean
  media: XMedia[]
}

/** 单张图片的导入结果。 */
export interface XImportItem {
  image?: ImageItem
  status: 'imported' | 'existing' | 'blocked' | 'failed'
  permission?: ImagePermission
  error?: string
}

/** 一次 X 导入的汇总。 */
export interface XImportResult {
  post: XPost
  items: XImportItem[]
  imported: number
  existing: number
  blocked: number
  failed: number
}

/** 解析 X 链接并返回作者与候选图片，不下载。 */
export function previewXImport(url: string): Promise<XPost> {
  return request<XPost>({
    method: 'POST',
    url: '/import/x/preview',
    data: { url },
    timeout: 30000,
  })
}

/** 下载并导入一条 X 帖子中的图片。 */
export function importX(params: {
  url: string
  mediaIds?: string[]
  permission?: ImagePermission
}): Promise<XImportResult> {
  return request<XImportResult>({
    method: 'POST',
    url: '/import/x',
    data: {
      url: params.url,
      media_ids: params.mediaIds,
      permission: params.permission,
    },
    timeout: 60000,
  })
}

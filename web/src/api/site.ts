import { request } from './client'
import type {
  Announcement,
  AnnouncementInput,
  Page,
  PageInput,
  Report,
  ReportInput,
  ReportStatus,
} from './types'

/** 已发布的公告（无需登录）。 */
export function listAnnouncements(): Promise<Announcement[]> {
  return request<Announcement[]>({ method: 'GET', url: '/announcements' })
}

/** 按 slug 获取已发布的独立页面（无需登录）。 */
export function getPage(slug: string): Promise<Page> {
  return request<Page>({ method: 'GET', url: `/pages/${slug}` })
}

/** 提交举报。 */
export function createReport(input: ReportInput): Promise<Report> {
  return request<Report>({ method: 'POST', url: '/reports', data: input })
}

/** 管理端：全部公告。 */
export function listAllAnnouncements(): Promise<Announcement[]> {
  return request<Announcement[]>({ method: 'GET', url: '/admin/announcements' })
}

export function createAnnouncement(input: AnnouncementInput): Promise<Announcement> {
  return request<Announcement>({ method: 'POST', url: '/admin/announcements', data: input })
}

export function updateAnnouncement(id: string, input: AnnouncementInput): Promise<Announcement> {
  return request<Announcement>({ method: 'PUT', url: `/admin/announcements/${id}`, data: input })
}

export function deleteAnnouncement(id: string): Promise<void> {
  return request<void>({ method: 'DELETE', url: `/admin/announcements/${id}` })
}

/** 管理端：举报。 */
export function listReports(status?: ReportStatus): Promise<Report[]> {
  return request<Report[]>({
    method: 'GET',
    url: '/admin/reports',
    params: status ? { status } : undefined,
  })
}

export function updateReport(id: string, status: ReportStatus, note: string): Promise<Report> {
  return request<Report>({ method: 'PATCH', url: `/admin/reports/${id}`, data: { status, note } })
}

/** 管理端：独立页面。 */
export function listPages(): Promise<Page[]> {
  return request<Page[]>({ method: 'GET', url: '/admin/pages' })
}

export function createPage(input: PageInput): Promise<Page> {
  return request<Page>({ method: 'POST', url: '/admin/pages', data: input })
}

export function updatePage(id: string, input: PageInput): Promise<Page> {
  return request<Page>({ method: 'PUT', url: `/admin/pages/${id}`, data: input })
}

export function deletePage(id: string): Promise<void> {
  return request<void>({ method: 'DELETE', url: `/admin/pages/${id}` })
}

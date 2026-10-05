import type {
  Album,
  Announcement,
  Coupon,
  CouponValidation,
  ImageItem,
  ImagingDrivers,
  Order,
  Page,
  PageData,
  Plan,
  Policy,
  PresignResult,
  PublicProfile,
  Report,
  RoleGroup,
  RuntimeInfo,
  Session,
  Share,
  SharePayload,
  Stats,
  StorageBackend,
  Ticket,
  Token,
  UploadPolicy,
  User,
} from './types'

export const VERSION = '0.1.0'

/** 统一的 API 错误。 */
export class AxmipicError extends Error {
  readonly status: number
  readonly code: number

  constructor(message: string, code: number, status: number) {
    super(message)
    this.name = 'AxmipicError'
    this.code = code
    this.status = status
  }

  isNotFound(): boolean {
    return this.status === 404
  }

  isUnauthorized(): boolean {
    return this.status === 401
  }

  isForbidden(): boolean {
    return this.status === 403
  }
}

interface Envelope<T> {
  code: number
  message: string
  data: T
}

export interface ClientOptions {
  /** 服务端基地址，例如 https://pic.example.com */
  baseUrl: string
  /** 初始会话或 API 令牌。 */
  token?: string
  /** 自定义 fetch 实现（默认为全局 fetch）。 */
  fetch?: typeof fetch
  /** 默认请求超时（毫秒），默认 30000。 */
  timeout?: number
}

export interface RequestOptions {
  method?: string
  body?: unknown
  contentType?: string
  query?: Record<string, string | number | boolean | undefined>
}

/** AXmiPic 的 TypeScript API 客户端。 */
export class AxmipicClient {
  private baseUrl: string
  private token: string
  private fetcher: typeof fetch
  private timeout: number

  constructor(options: ClientOptions) {
    const base = String(options.baseUrl ?? '').replace(/\/+$/, '')
    if (!base) {
      throw new Error('axmipic: baseUrl must not be empty')
    }
    if (!/^https?:\/\/[^/]+/i.test(base)) {
      throw new Error('axmipic: baseUrl must start with http:// or https://')
    }
    this.baseUrl = base
    this.token = options.token ?? ''
    this.fetcher = options.fetch ?? globalThis.fetch
    if (typeof this.fetcher !== 'function') {
      throw new Error('axmipic: no fetch implementation available')
    }
    this.timeout = options.timeout ?? 30000
  }

  /** 设置后续请求使用的令牌。 */
  setToken(token: string): void {
    this.token = token
  }

  /** 返回当前令牌。 */
  getToken(): string {
    return this.token
  }

  /** 返回服务端基地址。 */
  getBaseUrl(): string {
    return this.baseUrl
  }

  /** 发送一次请求并把 data 解包返回。 */
  async request<T>(path: string, options: RequestOptions = {}): Promise<T> {
    const url = this.buildUrl(path, options.query)
    const headers: Record<string, string> = { Accept: 'application/json' }
    if (this.token) {
      headers.Authorization = `Bearer ${this.token}`
    }
    let body: BodyInit | undefined
    if (options.body !== undefined) {
      if (options.contentType === 'multipart/form-data') {
        body = options.body as BodyInit
      } else {
        headers['Content-Type'] = options.contentType ?? 'application/json'
        body = JSON.stringify(options.body)
      }
    }
    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), this.timeout)
    let response: Response
    try {
      response = await this.fetcher(url, {
        method: options.method ?? 'GET',
        headers,
        body,
        signal: controller.signal,
      })
    } catch (error) {
      clearTimeout(timer)
      throw new AxmipicError(error instanceof Error ? error.message : 'network error', -1, 0)
    }
    clearTimeout(timer)

    const text = await response.text()
    let envelope: Envelope<T> | null = null
    if (text) {
      try {
        envelope = JSON.parse(text) as Envelope<T>
      } catch {
        if (response.status >= 400) {
          throw new AxmipicError(text.trim() || response.statusText, response.status, response.status)
        }
        throw new AxmipicError('invalid JSON response', -1, response.status)
      }
    }
    if (response.status >= 400 || (envelope && envelope.code !== 0)) {
      const message = envelope?.message || response.statusText || 'request failed'
      const code = envelope?.code ?? response.status
      throw new AxmipicError(message, code, response.status)
    }
    return (envelope?.data ?? (undefined as unknown)) as T
  }

  private buildUrl(
    path: string,
    query?: Record<string, string | number | boolean | undefined>,
  ): string {
    const url = this.baseUrl + path
    if (!query) return url
    const params = new URLSearchParams()
    for (const [key, value] of Object.entries(query)) {
      if (value === undefined || value === '') continue
      params.set(key, String(value))
    }
    const qs = params.toString()
    if (!qs) return url
    return url.includes('?') ? `${url}&${qs}` : `${url}?${qs}`
  }

  // ---- 认证 ----

  async register(username: string, password: string): Promise<User> {
    return this.request<User>('/api/v1/auth/register', {
      method: 'POST',
      body: { username, password },
    })
  }

  async login(username: string, password: string): Promise<Session> {
    const session = await this.request<Session>('/api/v1/auth/login', {
      method: 'POST',
      body: { username, password },
    })
    this.token = session.token
    return session
  }

  async loginAdmin(username: string, password: string): Promise<Session> {
    const session = await this.request<Session>('/api/v1/admin/auth/login', {
      method: 'POST',
      body: { username, password },
    })
    this.token = session.token
    return session
  }

  async me(): Promise<User> {
    return this.request<User>('/api/v1/auth/me')
  }

  async changePassword(currentPassword: string, newPassword: string): Promise<void> {
    await this.request<{ status: string }>('/api/v1/auth/password', {
      method: 'POST',
      body: { current_password: currentPassword, new_password: newPassword },
    })
  }

  async sendPasswordResetCode(email: string): Promise<void> {
    await this.request<{ status: string }>('/api/v1/auth/password/reset/code', {
      method: 'POST',
      body: { email },
    })
  }

  async resetPassword(email: string, code: string, newPassword: string): Promise<void> {
    await this.request<{ status: string }>('/api/v1/auth/password/reset', {
      method: 'POST',
      body: { email, code, new_password: newPassword },
    })
  }

  async policies(): Promise<UploadPolicy> {
    return this.request<UploadPolicy>('/api/v1/auth/policies')
  }

  async createToken(name: string): Promise<Token> {
    return this.request<Token>('/api/v1/tokens', { method: 'POST', body: { name } })
  }

  async listTokens(): Promise<Token[]> {
    return this.request<Token[]>('/api/v1/tokens')
  }

  async revokeToken(id: string): Promise<void> {
    await this.request<void>(`/api/v1/tokens/${id}`, { method: 'DELETE' })
  }

  // ---- 上传 ----

  async upload(filename: string, data: Blob | ArrayBuffer | Uint8Array): Promise<ImageItem> {
    const form = new FormData()
    let blob: Blob
    if (data instanceof Blob) {
      blob = data
    } else if (data instanceof Uint8Array) {
      blob = new Blob([data.slice()], { type: 'application/octet-stream' })
    } else {
      blob = new Blob([data], { type: 'application/octet-stream' })
    }
    form.append('file', blob, filename)
    return this.request<ImageItem>('/api/v1/upload', {
      method: 'POST',
      body: form,
      contentType: 'multipart/form-data',
    })
  }

  async presign(mimeType: string, size: number): Promise<PresignResult> {
    return this.request<PresignResult>('/api/v1/upload/presign', {
      method: 'POST',
      body: { mime_type: mimeType, size },
    })
  }

  async confirm(key: string): Promise<ImageItem> {
    return this.request<ImageItem>('/api/v1/upload/confirm', {
      method: 'POST',
      body: { key },
    })
  }

  // ---- 图片 ----

  async listImages(query: ImageQuery = {}): Promise<PageData<ImageItem>> {
    return this.request<PageData<ImageItem>>('/api/v1/images', { query: this.imageParams(query) })
  }

  async getImage(id: string): Promise<ImageItem> {
    return this.request<ImageItem>(`/api/v1/images/${id}`)
  }

  async renameImage(id: string, name: string): Promise<ImageItem> {
    return this.request<ImageItem>(`/api/v1/images/${id}`, { method: 'PATCH', body: { name } })
  }

  async deleteImage(id: string): Promise<void> {
    await this.request<void>(`/api/v1/images/${id}`, { method: 'DELETE' })
  }

  async batchImages(update: BatchUpdate): Promise<number> {
    const result = await this.request<{ updated: number }>('/api/v1/images/batch', {
      method: 'POST',
      body: update,
    })
    return result.updated
  }

  async listPlaza(query: ImageQuery = {}): Promise<PageData<ImageItem>> {
    return this.request<PageData<ImageItem>>('/api/v1/plaza', { query: this.imageParams(query) })
  }

  async listPublicAlbums(userId?: string): Promise<Album[]> {
    return this.request<Album[]>('/api/v1/plaza/albums', { query: { user_id: userId } })
  }

  async getPublicProfile(userId: string): Promise<PublicProfile> {
    return this.request<PublicProfile>(`/api/v1/users/${userId}`)
  }

  private imageParams(query: ImageQuery): Record<string, string | number | undefined> {
    return {
      page: query.page,
      page_size: query.pageSize,
      order: query.order,
      keyword: query.keyword,
      album_id: query.albumId,
      permission: query.permission,
      user_id: query.userId,
    }
  }

  // ---- 相册 ----

  async listAlbums(): Promise<Album[]> {
    return this.request<Album[]>('/api/v1/albums')
  }

  async createAlbum(input: AlbumInput): Promise<Album> {
    return this.request<Album>('/api/v1/albums', { method: 'POST', body: input })
  }

  async getAlbum(id: string): Promise<Album> {
    return this.request<Album>(`/api/v1/albums/${id}`)
  }

  async updateAlbum(id: string, input: AlbumInput): Promise<Album> {
    return this.request<Album>(`/api/v1/albums/${id}`, { method: 'PATCH', body: input })
  }

  async deleteAlbum(id: string): Promise<void> {
    await this.request<void>(`/api/v1/albums/${id}`, { method: 'DELETE' })
  }

  async listAlbumImages(id: string, page = 1, pageSize = 24): Promise<PageData<ImageItem>> {
    return this.request<PageData<ImageItem>>(`/api/v1/albums/${id}/images`, {
      query: { page, page_size: pageSize },
    })
  }

  // ---- 分享 ----

  async createShare(input: ShareInput): Promise<Share> {
    return this.request<Share>('/api/v1/shares', { method: 'POST', body: input })
  }

  async listShares(): Promise<Share[]> {
    return this.request<Share[]>('/api/v1/shares')
  }

  async revokeShare(id: string): Promise<void> {
    await this.request<void>(`/api/v1/shares/${id}`, { method: 'DELETE' })
  }

  async shareInfo(token: string): Promise<Share> {
    return this.request<Share>(`/api/v1/shares/${token}`)
  }

  async accessShare(token: string, password = ''): Promise<SharePayload> {
    return this.request<SharePayload>(`/api/v1/shares/${token}/access`, {
      method: 'POST',
      body: { password },
    })
  }

  // ---- 站点内容 ----

  async listAnnouncements(): Promise<Announcement[]> {
    return this.request<Announcement[]>('/api/v1/announcements')
  }

  async getPage(slug: string): Promise<Page> {
    return this.request<Page>(`/api/v1/pages/${slug}`)
  }

  async createReport(input: ReportInput): Promise<Report> {
    return this.request<Report>('/api/v1/reports', { method: 'POST', body: input })
  }

  // ---- 套餐 / 订单 / 工单 ----

  async listPlans(): Promise<Plan[]> {
    return this.request<Plan[]>('/api/v1/plans')
  }

  async listPaymentGateways(): Promise<string[]> {
    return this.request<string[]>('/api/v1/payment-gateways')
  }

  async validateCoupon(code: string, amountCents: number): Promise<CouponValidation> {
    return this.request<CouponValidation>('/api/v1/coupons/validate', {
      method: 'POST',
      body: { code, amount_cents: amountCents },
    })
  }

  async createOrder(input: OrderInput): Promise<Order> {
    return this.request<Order>('/api/v1/orders', { method: 'POST', body: input })
  }

  async listOrders(): Promise<Order[]> {
    return this.request<Order[]>('/api/v1/orders')
  }

  async payOrder(id: string): Promise<Order> {
    return this.request<Order>(`/api/v1/orders/${id}/pay`, { method: 'POST', body: {} })
  }

  async createTicket(input: TicketInput): Promise<Ticket> {
    return this.request<Ticket>('/api/v1/tickets', { method: 'POST', body: input })
  }

  async listTickets(status?: string): Promise<Ticket[]> {
    return this.request<Ticket[]>('/api/v1/tickets', { query: { status } })
  }

  async getTicket(id: string): Promise<Ticket> {
    return this.request<Ticket>(`/api/v1/tickets/${id}`)
  }

  async replyTicket(id: string, body: string): Promise<Ticket> {
    return this.request<Ticket>(`/api/v1/tickets/${id}/reply`, { method: 'POST', body: { body } })
  }

  // ---- 管理员 ----

  async adminStats(): Promise<Stats> {
    return this.request<Stats>('/api/v1/admin/stats')
  }

  async adminListCustomers(): Promise<User[]> {
    return this.request<User[]>('/api/v1/admin/customers')
  }

  async adminListAdmins(): Promise<User[]> {
    return this.request<User[]>('/api/v1/admin/admins')
  }

  async adminCreateAdmin(username: string, password: string): Promise<User> {
    return this.request<User>('/api/v1/admin/admins', { method: 'POST', body: { username, password } })
  }

  async adminUpdateCustomer(id: string, update: UserUpdate): Promise<User> {
    return this.request<User>(`/api/v1/admin/customers/${id}`, { method: 'PATCH', body: update })
  }

  async adminUpdateAdmin(id: string, update: UserUpdate): Promise<User> {
    return this.request<User>(`/api/v1/admin/admins/${id}`, { method: 'PATCH', body: update })
  }

  async adminDeleteCustomer(id: string): Promise<void> {
    await this.request<void>(`/api/v1/admin/customers/${id}`, { method: 'DELETE' })
  }

  async adminDeleteAdmin(id: string): Promise<void> {
    await this.request<void>(`/api/v1/admin/admins/${id}`, { method: 'DELETE' })
  }

  async adminListStorage(): Promise<StorageBackend[]> {
    return this.request<StorageBackend[]>('/api/v1/admin/storage')
  }

  async adminCreateStorage(input: StorageInput): Promise<StorageBackend> {
    return this.request<StorageBackend>('/api/v1/admin/storage', { method: 'POST', body: input })
  }

  async adminUpdateStorage(id: string, input: StorageInput): Promise<StorageBackend> {
    return this.request<StorageBackend>(`/api/v1/admin/storage/${id}`, { method: 'PUT', body: input })
  }

  async adminDeleteStorage(id: string): Promise<void> {
    await this.request<void>(`/api/v1/admin/storage/${id}`, { method: 'DELETE' })
  }

  async adminActivateStorage(id: string): Promise<void> {
    await this.request<void>(`/api/v1/admin/storage/${id}/activate`, { method: 'POST', body: {} })
  }

  async adminListRoleGroups(): Promise<RoleGroup[]> {
    return this.request<RoleGroup[]>('/api/v1/admin/role-groups')
  }

  async adminCreateRoleGroup(input: RoleGroupInput): Promise<RoleGroup> {
    return this.request<RoleGroup>('/api/v1/admin/role-groups', { method: 'POST', body: input })
  }

  async adminUpdateRoleGroup(id: string, input: RoleGroupInput): Promise<RoleGroup> {
    return this.request<RoleGroup>(`/api/v1/admin/role-groups/${id}`, { method: 'PUT', body: input })
  }

  async adminDeleteRoleGroup(id: string): Promise<void> {
    await this.request<void>(`/api/v1/admin/role-groups/${id}`, { method: 'DELETE' })
  }

  async adminAttachPolicy(roleGroupId: string, policyId: string): Promise<RoleGroup> {
    return this.request<RoleGroup>(`/api/v1/admin/role-groups/${roleGroupId}/policies`, {
      method: 'POST',
      body: { policy_id: policyId },
    })
  }

  async adminDetachPolicy(roleGroupId: string, policyId: string): Promise<RoleGroup> {
    return this.request<RoleGroup>(
      `/api/v1/admin/role-groups/${roleGroupId}/policies/${policyId}`,
      { method: 'DELETE' },
    )
  }

  async adminListPolicies(type?: string): Promise<Policy[]> {
    return this.request<Policy[]>('/api/v1/admin/policies', { query: { type } })
  }

  async adminCreatePolicy(input: PolicyInput): Promise<Policy> {
    return this.request<Policy>('/api/v1/admin/policies', { method: 'POST', body: input })
  }

  async adminUpdatePolicy(id: string, input: PolicyInput): Promise<Policy> {
    return this.request<Policy>(`/api/v1/admin/policies/${id}`, { method: 'PUT', body: input })
  }

  async adminDeletePolicy(id: string): Promise<void> {
    await this.request<void>(`/api/v1/admin/policies/${id}`, { method: 'DELETE' })
  }

  async adminListAnnouncements(): Promise<Announcement[]> {
    return this.request<Announcement[]>('/api/v1/admin/announcements')
  }

  async adminCreateAnnouncement(input: AnnouncementInput): Promise<Announcement> {
    return this.request<Announcement>('/api/v1/admin/announcements', { method: 'POST', body: input })
  }

  async adminUpdateAnnouncement(id: string, input: AnnouncementInput): Promise<Announcement> {
    return this.request<Announcement>(`/api/v1/admin/announcements/${id}`, { method: 'PUT', body: input })
  }

  async adminDeleteAnnouncement(id: string): Promise<void> {
    await this.request<void>(`/api/v1/admin/announcements/${id}`, { method: 'DELETE' })
  }

  async adminListReports(status?: string): Promise<Report[]> {
    return this.request<Report[]>('/api/v1/admin/reports', { query: { status } })
  }

  async adminUpdateReport(id: string, status: string, note: string): Promise<Report> {
    return this.request<Report>(`/api/v1/admin/reports/${id}`, {
      method: 'PATCH',
      body: { status, note },
    })
  }

  async adminListPages(): Promise<Page[]> {
    return this.request<Page[]>('/api/v1/admin/pages')
  }

  async adminCreatePage(input: PageInput): Promise<Page> {
    return this.request<Page>('/api/v1/admin/pages', { method: 'POST', body: input })
  }

  async adminUpdatePage(id: string, input: PageInput): Promise<Page> {
    return this.request<Page>(`/api/v1/admin/pages/${id}`, { method: 'PUT', body: input })
  }

  async adminDeletePage(id: string): Promise<void> {
    await this.request<void>(`/api/v1/admin/pages/${id}`, { method: 'DELETE' })
  }

  async adminListPlans(): Promise<Plan[]> {
    return this.request<Plan[]>('/api/v1/admin/plans')
  }

  async adminCreatePlan(input: PlanInput): Promise<Plan> {
    return this.request<Plan>('/api/v1/admin/plans', { method: 'POST', body: input })
  }

  async adminUpdatePlan(id: string, input: PlanInput): Promise<Plan> {
    return this.request<Plan>(`/api/v1/admin/plans/${id}`, { method: 'PUT', body: input })
  }

  async adminDeletePlan(id: string): Promise<void> {
    await this.request<void>(`/api/v1/admin/plans/${id}`, { method: 'DELETE' })
  }

  async adminListCoupons(): Promise<Coupon[]> {
    return this.request<Coupon[]>('/api/v1/admin/coupons')
  }

  async adminCreateCoupon(input: CouponInput): Promise<Coupon> {
    return this.request<Coupon>('/api/v1/admin/coupons', { method: 'POST', body: input })
  }

  async adminUpdateCoupon(id: string, input: CouponInput): Promise<Coupon> {
    return this.request<Coupon>(`/api/v1/admin/coupons/${id}`, { method: 'PUT', body: input })
  }

  async adminDeleteCoupon(id: string): Promise<void> {
    await this.request<void>(`/api/v1/admin/coupons/${id}`, { method: 'DELETE' })
  }

  async adminSetTicketStatus(id: string, status: string): Promise<Ticket> {
    return this.request<Ticket>(`/api/v1/admin/tickets/${id}`, {
      method: 'PATCH',
      body: { status },
    })
  }

  async adminNotifyChannels(): Promise<Record<string, string>> {
    return this.request<Record<string, string>>('/api/v1/admin/notify/channels')
  }

  async adminTestNotify(input: NotifyTest): Promise<void> {
    await this.request<void>('/api/v1/admin/notify/test', { method: 'POST', body: input })
  }

  async adminSecurity(): Promise<Record<string, string>> {
    return this.request<Record<string, string>>('/api/v1/admin/security')
  }

  async adminImagingDrivers(): Promise<ImagingDrivers> {
    return this.request<ImagingDrivers>('/api/v1/admin/imaging/drivers')
  }

  async adminRuntimeInfo(): Promise<RuntimeInfo> {
    return this.request<RuntimeInfo>('/api/v1/admin/runtime')
  }
}

// ---- 输入类型 ----

export interface ImageQuery {
  page?: number
  pageSize?: number
  order?: 'newest' | 'earliest' | 'largest' | 'smallest'
  keyword?: string
  albumId?: string
  permission?: 'public' | 'private'
  userId?: string
}

export interface BatchUpdate {
  ids: string[]
  permission?: 'public' | 'private'
  album_id?: string
  clear_album?: boolean
}

export interface AlbumInput {
  name: string
  intro: string
  permission?: 'public' | 'private'
}

export interface ShareInput {
  target_type: 'image' | 'album'
  target_id: string
  password?: string
  expires_in_hours?: number
  max_views?: number
}

export interface ReportInput {
  image_id?: string
  reason: string
  detail?: string
}

export interface OrderInput {
  plan_id: string
  coupon_code?: string
  provider?: string
}

export interface TicketInput {
  subject: string
  category?: string
  body: string
  priority?: 'low' | 'normal' | 'high'
}

export interface UserUpdate {
  disabled?: boolean
  role_group_id?: string
}

export interface StorageInput {
  name: string
  driver: 'local' | 's3' | 'qiniu'
  settings: Record<string, unknown>
  secrets: Record<string, string>
  activate?: boolean
}

export interface RoleGroupInput {
  name: string
  description: string
  is_default: boolean
}

export interface PolicyInput {
  name: string
  type: string
  description: string
  enabled: boolean
  settings: Record<string, unknown>
}

export interface AnnouncementInput {
  title: string
  content: string
  level: 'info' | 'success' | 'warning' | 'danger'
  pinned: boolean
  published: boolean
}

export interface PageInput {
  slug: string
  title: string
  content: string
  published: boolean
}

export interface PlanInput {
  name: string
  description: string
  price_cents: number
  duration_days: number
  quota_mb: number
  role_group_id?: string
  active: boolean
  sort_order: number
}

export interface CouponInput {
  code: string
  type: 'fixed' | 'percent'
  value: number
  min_amount_cents: number
  max_uses: number
  per_user_limit: number
  expires_at?: string | null
  active: boolean
}

export interface NotifyTest {
  channel: 'sms' | 'email'
  to: string
  subject?: string
  body: string
}

/** 即时图片处理参数。 */
export interface TransformParams {
  w?: number
  h?: number
  fit?: 'contain' | 'cover' | 'fill'
  q?: number
  f?: string
  r?: 0 | 90 | 180 | 270
  flip?: 'h' | 'v' | 'hv'
  gray?: boolean
  blur?: number
  sharpen?: number
  enlarge?: boolean
  wm?: string
  wm_pos?: string
  wm_opacity?: number
  wm_size?: number
  wm_color?: string
}

/** 在图片 URL 上附加处理参数。 */
export function transformUrl(baseUrl: string, params: TransformParams): string {
  const query = new URLSearchParams()
  const set = (key: string, value: unknown): void => {
    if (value === undefined || value === null || value === '' || value === false) return
    if (value === true) {
      query.set(key, '1')
      return
    }
    query.set(key, String(value))
  }
  set('w', params.w)
  set('h', params.h)
  set('fit', params.fit)
  set('q', params.q)
  set('f', params.f)
  set('r', params.r)
  set('flip', params.flip)
  set('gray', params.gray)
  set('blur', params.blur)
  set('sharpen', params.sharpen)
  set('enlarge', params.enlarge)
  set('wm', params.wm)
  set('wm_pos', params.wm_pos)
  set('wm_opacity', params.wm_opacity)
  set('wm_size', params.wm_size)
  set('wm_color', params.wm_color)
  const qs = query.toString()
  if (!qs) return baseUrl
  return baseUrl.includes('?') ? `${baseUrl}&${qs}` : `${baseUrl}?${qs}`
}

import { request } from './client'
import type {
  Coupon,
  CouponInput,
  Order,
  Plan,
  PlanInput,
  Ticket,
  TicketInput,
  TicketStatus,
} from './types'

/** 启用的套餐（无需登录）。 */
export function listPlans(): Promise<Plan[]> {
  return request<Plan[]>({ method: 'GET', url: '/plans' })
}

/** 已注册的支付渠道。 */
export function listPaymentGateways(): Promise<string[]> {
  return request<string[]>({ method: 'GET', url: '/payment-gateways' })
}

/** 校验优惠券并返回折扣（分）。 */
export function validateCoupon(code: string, amountCents: number): Promise<{ coupon: Coupon; discount_cents: number }> {
  return request({ method: 'POST', url: '/coupons/validate', data: { code, amount_cents: amountCents } })
}

/** 下单。 */
export function createOrder(planId: string, couponCode = '', provider = 'manual'): Promise<Order> {
  return request<Order>({
    method: 'POST',
    url: '/orders',
    data: { plan_id: planId, coupon_code: couponCode, provider },
  })
}

export function listOrders(): Promise<Order[]> {
  return request<Order[]>({ method: 'GET', url: '/orders' })
}

/** 完成手动/模拟订单的支付。 */
export function payOrder(id: string): Promise<Order> {
  return request<Order>({ method: 'POST', url: `/orders/${id}/pay` })
}

/** 工单。 */
export function listTickets(status?: TicketStatus): Promise<Ticket[]> {
  return request<Ticket[]>({ method: 'GET', url: '/tickets', params: status ? { status } : undefined })
}

export function createTicket(input: TicketInput): Promise<Ticket> {
  return request<Ticket>({ method: 'POST', url: '/tickets', data: input })
}

export function getTicket(id: string): Promise<Ticket> {
  return request<Ticket>({ method: 'GET', url: `/tickets/${id}` })
}

export function replyTicket(id: string, body: string): Promise<Ticket> {
  return request<Ticket>({ method: 'POST', url: `/tickets/${id}/reply`, data: { body } })
}

/** 管理端：套餐。 */
export function adminListPlans(): Promise<Plan[]> {
  return request<Plan[]>({ method: 'GET', url: '/admin/plans' })
}

export function createPlan(input: PlanInput): Promise<Plan> {
  return request<Plan>({ method: 'POST', url: '/admin/plans', data: input })
}

export function updatePlan(id: string, input: PlanInput): Promise<Plan> {
  return request<Plan>({ method: 'PUT', url: `/admin/plans/${id}`, data: input })
}

export function deletePlan(id: string): Promise<void> {
  return request<void>({ method: 'DELETE', url: `/admin/plans/${id}` })
}

/** 管理端：优惠券。 */
export function adminListCoupons(): Promise<Coupon[]> {
  return request<Coupon[]>({ method: 'GET', url: '/admin/coupons' })
}

export function createCoupon(input: CouponInput): Promise<Coupon> {
  return request<Coupon>({ method: 'POST', url: '/admin/coupons', data: input })
}

export function updateCoupon(id: string, input: CouponInput): Promise<Coupon> {
  return request<Coupon>({ method: 'PUT', url: `/admin/coupons/${id}`, data: input })
}

export function deleteCoupon(id: string): Promise<void> {
  return request<void>({ method: 'DELETE', url: `/admin/coupons/${id}` })
}

/** 管理端：工单状态。 */
export function setTicketStatus(id: string, status: TicketStatus): Promise<Ticket> {
  return request<Ticket>({ method: 'PATCH', url: `/admin/tickets/${id}`, data: { status } })
}

/** 管理端：通知渠道与测试。 */
export function getNotifyChannels(): Promise<{ sms: string; email: string }> {
  return request<{ sms: string; email: string }>({ method: 'GET', url: '/admin/notify/channels' })
}

export function sendTestNotify(payload: {
  channel: 'sms' | 'email'
  to: string
  subject?: string
  body: string
}): Promise<{ status: string }> {
  return request<{ status: string }>({ method: 'POST', url: '/admin/notify/test', data: payload })
}

/** 一条通知发送日志。 */
export interface NotifyLog {
  id: string
  channel: 'email' | 'sms'
  to: string
  subject: string
  body: string
  status: 'sent' | 'failed'
  error?: string
  provider: string
  created_at: string
}

export interface NotifyLogList {
  items: NotifyLog[]
  total: number
  page: number
  page_size: number
}

/** 管理端：通知发送日志（可按渠道过滤并分页）。 */
export function getNotifyLogs(params: {
  channel?: 'email' | 'sms'
  page?: number
  page_size?: number
}): Promise<NotifyLogList> {
  return request<NotifyLogList>({ method: 'GET', url: '/admin/notify/logs', params })
}

/** SMTP 邮件渠道设置。密码只返回是否已设置，不回传明文。 */
export interface SMTPConfig {
  enabled: boolean
  host: string
  port: number
  username: string
  from: string
  use_tls: boolean
  password_set: boolean
}

/** 更新 SMTP 设置的输入；password 为空表示保持原密码。 */
export interface SMTPInput {
  enabled: boolean
  host: string
  port: number
  username: string
  password: string
  from: string
  use_tls: boolean
}

/** 管理端：读取 SMTP 设置。 */
export function getSMTPConfig(): Promise<SMTPConfig> {
  return request<SMTPConfig>({ method: 'GET', url: '/admin/notify/smtp' })
}

/** 管理端：保存 SMTP 设置并即时生效。 */
export function updateSMTPConfig(input: SMTPInput): Promise<SMTPConfig> {
  return request<SMTPConfig>({ method: 'PUT', url: '/admin/notify/smtp', data: input })
}

/** 支付渠道设置（密钥仅返回是否已设置）。 */
export interface PaymentSettings {
  default_gateway: string
  alipay: {
    enabled: boolean
    gateway_url: string
    app_id: string
    private_key_set: boolean
    public_key_set: boolean
  }
  wechat: {
    enabled: boolean
    gateway_url: string
    app_id: string
    mch_id: string
    serial_no: string
    platform_serial_no: string
    private_key_set: boolean
    api_v3_key_set: boolean
    platform_public_key_set: boolean
  }
  epay: {
    enabled: boolean
    pid: string
    gateway_url: string
    api_url: string
    submit_url: string
    pay_type: string
    key_set: boolean
  }
}

/** 更新支付设置的输入；密钥为空表示保持不变。 */
export interface PaymentSettingsInput {
  default_gateway: string
  alipay: {
    enabled: boolean
    gateway_url: string
    app_id: string
    private_key: string
    public_key: string
  }
  wechat: {
    enabled: boolean
    gateway_url: string
    app_id: string
    mch_id: string
    serial_no: string
    private_key: string
    api_v3_key: string
    platform_public_key: string
    platform_serial_no: string
  }
  epay: {
    enabled: boolean
    pid: string
    key: string
    gateway_url: string
    api_url: string
    submit_url: string
    pay_type: string
  }
}

/** 管理端：读取支付设置。 */
export function getPaymentSettings(): Promise<PaymentSettings> {
  return request<PaymentSettings>({ method: 'GET', url: '/admin/payment' })
}

/** 管理端：保存支付设置并即时生效。 */
export function updatePaymentSettings(input: PaymentSettingsInput): Promise<PaymentSettings> {
  return request<PaymentSettings>({ method: 'PUT', url: '/admin/payment', data: input })
}

/** 管理端：安全扫描器信息。 */
export function getSecurityInfo(): Promise<{ scanner?: string }> {
  return request<{ scanner?: string }>({ method: 'GET', url: '/admin/security' })
}

/** 管理端：可用的图片处理驱动。 */
export function getImagingDrivers(): Promise<{ available: string[]; active: string }> {
  return request<{ available: string[]; active: string }>({
    method: 'GET',
    url: '/admin/imaging/drivers',
  })
}

/** 实例运行环境信息。 */
export interface RuntimeInfo {
  site_name: string
  base_url: string
  database_driver: string
  storage_driver: string
  processor: string
  formats: string[]
  allow_registration: boolean
  require_auth: boolean
  allow_guest_upload: boolean
  guest_quota_mb: number
  guest_upload_max_mb: number
  default_quota_mb: number
  upload_max_mb: number
  trust_proxy: boolean
  session_ttl_hours: number
  install_lock_file: string
  installed: boolean
  go_version: string
  platform: string
}

/** 管理端：运行环境信息。 */
export function getRuntimeInfo(): Promise<RuntimeInfo> {
  return request<RuntimeInfo>({ method: 'GET', url: '/admin/runtime' })
}

/** 进程实时运行时指标。 */
export interface ProcessInfo {
  goroutines: number
  heap_alloc_bytes: number
  heap_inuse_bytes: number
  heap_objects: number
  heap_sys_bytes: number
  sys_bytes: number
  stack_inuse_bytes: number
  gc_count: number
  num_cpu: number
  uptime_seconds: number
  go_version: string
  platform: string
}

/** 管理端：进程运行时指标。 */
export function getProcessInfo(): Promise<ProcessInfo> {
  return request<ProcessInfo>({ method: 'GET', url: '/admin/process' })
}

/** 可在线切换的权限开关。 */
export interface AuthConfig {
  allow_registration: boolean
  require_auth: boolean
  allow_guest_upload: boolean
}

/** 管理端：读取权限开关。 */
export function getAuthSettings(): Promise<AuthConfig> {
  return request<AuthConfig>({ method: 'GET', url: '/admin/auth' })
}

/** 管理端：保存权限开关并即时生效。 */
export function updateAuthSettings(payload: AuthConfig): Promise<AuthConfig> {
  return request<AuthConfig>({ method: 'PUT', url: '/admin/auth', data: payload })
}

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
  client_ip_source: string
  client_ip_header?: string
  client_ip_trusted_proxies?: string[]
  client_ip_xff_depth: number
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

/** 图片广场 AI 审查设置（密钥仅返回是否已设置）。 */
export interface ModerationSettings {
  enabled: boolean
  base_url: string
  model: string
  timeout_sec: number
  prompt: string
  max_image_mb: number
  api_key_set: boolean
}

/** 保存 AI 审查设置的输入；api_key 为空表示保持原密钥。 */
export interface ModerationSettingsInput {
  enabled: boolean
  base_url: string
  api_key: string
  model: string
  timeout_sec: number
  prompt: string
  max_image_mb: number
}

/** 管理端：读取 AI 审查设置。 */
export function getModerationSettings(): Promise<ModerationSettings> {
  return request<ModerationSettings>({ method: 'GET', url: '/admin/moderation' })
}

/** 管理端：保存 AI 审查设置并即时生效。 */
export function updateModerationSettings(payload: ModerationSettingsInput): Promise<ModerationSettings> {
  return request<ModerationSettings>({ method: 'PUT', url: '/admin/moderation', data: payload })
}

// ---- 通用系统设置域（upload/processing/security/sms/limits/maintenance/site）----

/** 可配置的设置域标识。 */
export type SettingDomain =
  | 'upload'
  | 'processing'
  | 'security'
  | 'sms'
  | 'limits'
  | 'maintenance'
  | 'site'
  | 'client_ip'

export interface UploadSettings {
  max_size_mb: number
  allowed_mime_types: string[]
}

export interface ImagingSettings {
  enabled: boolean
  max_width: number
  max_height: number
  default_quality: number
  allowed_formats: string[]
  allow_enlarge: boolean
  allow_effects: boolean
  allow_watermark: boolean
  watermark_text: string
}

export interface SecuritySettings {
  scanner: string
  cloud_processor: string
}

export interface SMSSettings {
  enabled: boolean
  /** 渠道类型：'log' 日志、'http' 通用网关，或已加载的短信插件名。 */
  channel: string
  provider: string
  endpoint: string
  method: string
}

/** 插件配置字段（与后端 plugin.Field 对齐）。 */
export interface PluginField {
  key: string
  label: string
  type?: string
  required?: boolean
  secret?: boolean
  default?: string
  options?: string[]
  help?: string
}

/** 插件自描述（与后端 plugin.Descriptor 对齐）。 */
export interface PluginDescriptor {
  category: string
  name: string
  title?: string
  version?: string
  fields?: PluginField[]
}

/** 插件配置（秘钥字段只返回是否已设置）。 */
export interface PluginConfig extends PluginDescriptor {
  values: Record<string, string>
  secrets: Record<string, boolean>
  configured: boolean
}

/** 列出指定类别的插件。 */
export function listPlugins(category?: string): Promise<{ items: PluginDescriptor[] }> {
  return request<{ items: PluginDescriptor[] }>({
    method: 'GET',
    url: '/admin/plugins',
    params: category ? { category } : undefined,
  })
}

/** 读取插件配置（秘钥仅返回是否已设置）。 */
export function getPluginConfig(name: string): Promise<PluginConfig> {
  return request<PluginConfig>({ method: 'GET', url: `/admin/plugins/${name}` })
}

/** 保存插件配置并即时应用。秘钥字段留空表示保持原值。 */
export function updatePluginConfig(
  name: string,
  values: Record<string, string>,
): Promise<PluginConfig> {
  return request<PluginConfig>({
    method: 'PUT',
    url: `/admin/plugins/${name}/config`,
    data: { values },
  })
}

/** 用当前配置向插件发送一条测试通知。 */
export function testPlugin(
  name: string,
  payload: { to: string; subject?: string; body?: string },
): Promise<{ status: string; result: unknown }> {
  return request<{ status: string; result: unknown }>({
    method: 'POST',
    url: `/admin/plugins/${name}/test`,
    data: payload,
  })
}

/** 插件市场索引条目。 */
export interface PluginMarketEntry {
  name: string
  version: string
  category: string
  runtime: string
  description?: string
  url: string
  sha256?: string
  signature?: string
}

/** 拉取插件市场索引。 */
export function listPluginRegistry(): Promise<{ items: PluginMarketEntry[]; configured: boolean }> {
  return request<{ items: PluginMarketEntry[]; configured: boolean }>({
    method: 'GET',
    url: '/admin/plugins/registry',
  })
}

/** 安装插件：按索引名，或按直链 URL + 校验值。 */
export function installPlugin(payload: {
  name?: string
  version?: string
  url?: string
  sha256?: string
  signature?: string
}): Promise<PluginDescriptor> {
  return request<PluginDescriptor>({ method: 'POST', url: '/admin/plugins/install', data: payload })
}

/** 卸载并删除插件。 */
export function removePlugin(name: string): Promise<{ status: string; name: string }> {
  return request<{ status: string; name: string }>({
    method: 'DELETE',
    url: `/admin/plugins/${name}`,
  })
}

/** 重新加载插件。 */
export function reloadPlugin(name: string): Promise<PluginConfig> {
  return request<PluginConfig>({ method: 'POST', url: `/admin/plugins/${name}/reload` })
}

/** 插件安装与启用状态。 */
export interface PluginStatus extends PluginDescriptor {
  runtime: string
  /** 三态：disabled（暂停）/ standby（已启用待激活）/ active（已加载）。 */
  state: 'disabled' | 'standby' | 'active'
  enabled: boolean
  loaded: boolean
  memory_bytes?: number
  last_used?: number
  last_error?: string
  configured: boolean
}

/** 列出全部已安装插件及其状态。 */
export function listInstalledPlugins(): Promise<{ items: PluginStatus[] }> {
  return request<{ items: PluginStatus[] }>({ method: 'GET', url: '/admin/plugins/installed' })
}

/** 启用或暂停插件（状态持久化）。 */
export function setPluginEnabled(
  name: string,
  enabled: boolean,
): Promise<{ name: string; enabled: boolean }> {
  return request<{ name: string; enabled: boolean }>({
    method: 'PUT',
    url: `/admin/plugins/${name}/enabled`,
    data: { enabled },
  })
}

/** 本地上传插件归档（zip）安装。 */
export function installPluginArchive(
  file: Blob,
  sha256 = '',
  signature = '',
): Promise<PluginDescriptor> {
  const headers: Record<string, string> = { 'Content-Type': 'application/octet-stream' }
  if (sha256) headers['X-Plugin-Sha256'] = sha256
  if (signature) headers['X-Plugin-Signature'] = signature
  return request<PluginDescriptor>({
    method: 'POST',
    url: '/admin/plugins/install',
    data: file,
    timeout: 120000,
    headers,
  })
}

export interface LimitsSettings {
  upload_per_minute: number
  upload_burst: number
  guest_per_minute: number
  guest_burst: number
  image_per_minute: number
  image_burst: number
  share_per_minute: number
  share_burst: number
}

export interface MaintenanceSettings {
  orphan_cleanup: boolean
  orphan_grace_hours: number
  orphan_interval_hours: number
}

export interface SiteSettings {
  name: string
  description: string
}

export interface ClientIPSettings {
  source: string
  header: string
  trusted_proxies: string[]
  xff_depth: number
}

/** 客户端 IP 解析详情（诊断/模拟）。 */
export interface ClientIPInfo {
  resolved: string
  peer: string
  source: string
  header: string
  trusted_peer: boolean
  trusted_proxies: string[]
  xff_depth: number
  x_forwarded_for?: string
  x_real_ip?: string
  cf_connecting_ip?: string
  true_client_ip?: string
  forwarded?: string
}

/** 当前请求的客户端 IP 解析详情。 */
export function getClientIPInfo(): Promise<ClientIPInfo> {
  return request<ClientIPInfo>({ method: 'GET', url: '/admin/client-ip' })
}

/** 用给定对端地址与请求头模拟解析，验证规则。 */
export function previewClientIP(payload: {
  remote_addr: string
  headers: Record<string, string>
}): Promise<ClientIPInfo> {
  return request<ClientIPInfo>({ method: 'POST', url: '/admin/client-ip/preview', data: payload })
}

/** 读取某个设置域的当前值。 */
export function getSettingDomain<T>(domain: SettingDomain): Promise<T> {
  return request<T>({ method: 'GET', url: `/admin/settings/${domain}` })
}

/** 保存某个设置域（可热应用的域即时生效）。 */
export function updateSettingDomain<T>(domain: SettingDomain, value: unknown): Promise<T> {
  return request<T>({ method: 'PUT', url: `/admin/settings/${domain}`, data: value })
}

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { DataLine, Monitor, Odometer, Refresh, SetUp } from '@element-plus/icons-vue'

import { fetchStats } from '@/api/admin'
import {
  getAuthSettings,
  getClientIPInfo,
  getImagingDrivers,
  getModerationSettings,
  getNotifyChannels,
  getPaymentSettings,
  getProcessInfo,
  getRuntimeInfo,
  getSecurityInfo,
  getSettingDomain,
  getSMTPConfig,
  listPaymentGateways,
  listInstalledPlugins,
  listPlugins,
  previewClientIP,
  sendTestNotify,
  updateAuthSettings,
  updateModerationSettings,
  updatePaymentSettings,
  updateSettingDomain,
  updateSMTPConfig,
  type AuthConfig,
  type ClientIPInfo,
  type ClientIPSettings,
  type ImagingSettings,
  type LimitsSettings,
  type MaintenanceSettings,
  type ModerationSettings,
  type PaymentSettings,
  type PaymentSettingsInput,
  type PluginDescriptor,
  type PluginStatus,
  type ProcessInfo,
  type RuntimeInfo,
  type SecuritySettings,
  type SiteSettings,
  type SMSSettings,
  type SMTPConfig,
  type SettingDomain,
  type UploadSettings,
} from '@/api/billing'
import { toApiError } from '@/api/client'
import type { AdminStats } from '@/api/types'
import ErrorState from '@/components/ErrorState.vue'
import PageHeader from '@/components/PageHeader.vue'
import { formatBytes, formatNumber } from '@/utils/format'

const loading = ref(false)
const errorMessage = ref('')
const activeTab = ref('overview')

const stats = ref<AdminStats | null>(null)
const runtime = ref<RuntimeInfo | null>(null)
const process = ref<ProcessInfo | null>(null)
const channels = ref<{ sms: string; email: string }>({ sms: '', email: '' })
const scannerName = ref('')
const drivers = ref<{ available: string[]; active: string }>({ available: [], active: '' })

// 可在线切换的权限开关（开放注册、上传鉴权、访客上传）。
const authSaving = ref(false)
const allowRegistration = ref(true)
const requireAuth = ref(true)
const allowGuestUpload = ref(false)

let processTimer: number | undefined

const processRows = computed(() => {
  const p = process.value
  if (!p) return []
  return [
    { label: 'Goroutine 数量', value: formatNumber(p.goroutines) },
    { label: '已分配堆内存', value: formatBytes(p.heap_alloc_bytes) },
    { label: '正在使用的堆内存', value: formatBytes(p.heap_inuse_bytes) },
    { label: '堆对象数', value: formatNumber(p.heap_objects) },
    { label: '运行时保留内存', value: formatBytes(p.heap_sys_bytes) },
    { label: '虚拟内存总量', value: formatBytes(p.sys_bytes) },
    { label: '栈内存使用', value: formatBytes(p.stack_inuse_bytes) },
    { label: 'GC 次数', value: formatNumber(p.gc_count) },
    { label: '逻辑 CPU', value: `${p.num_cpu} 核` },
    { label: '运行时长', value: formatDuration(p.uptime_seconds) },
  ]
})

const runtimeRows = computed(() => {
  const info = runtime.value
  if (!info) return []
  return [
    { label: '站点地址', value: info.base_url },
    { label: '数据库', value: info.database_driver },
    { label: '默认存储驱动', value: info.storage_driver },
    { label: '图片处理器', value: info.processor },
    { label: '运行环境', value: `${info.go_version} · ${info.platform}` },
    { label: '安装状态', value: info.installed ? '已安装' : '未安装' },
  ]
})

const policyRows = computed(() => {
  const info = runtime.value
  if (!info) return []
  return [
    { label: '默认配额', value: formatMiB(info.default_quota_mb) },
    { label: '单文件上限', value: formatMiB(info.upload_max_mb) },
    { label: '访客配额', value: formatMiB(info.guest_quota_mb) },
    { label: '访客单文件上限', value: formatMiB(info.guest_upload_max_mb) },
    { label: '会话有效期', value: `${info.session_ttl_hours} 小时` },
    { label: '客户端真实 IP', value: clientIPSummary(info) },
    { label: '可信代理', value: trustedProxySummary(info) },
  ]
})

/** 概览里展示当前生效的客户端 IP 解析方式。 */
function clientIPSummary(info: RuntimeInfo): string {
  const source = info.client_ip_source
  if (!source || source === 'remote') return '不信任转发头（remote）'
  const header = source === 'custom' && info.client_ip_header ? info.client_ip_header : source
  return `信任转发头（${header}）`
}

function trustedProxySummary(info: RuntimeInfo): string {
  const list = info.client_ip_trusted_proxies ?? []
  return list.length > 0 ? list.join('、') : '未配置'
}

/** 将秒数格式化为「Xd Xh Xm Xs」形式的运行时长。 */
function formatDuration(seconds: number): string {
  const total = Math.max(0, Math.floor(seconds))
  const d = Math.floor(total / 86400)
  const h = Math.floor((total % 86400) / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  const parts: string[] = []
  if (d > 0) parts.push(`${d} 天`)
  if (h > 0 || d > 0) parts.push(`${h} 小时`)
  if (m > 0 || h > 0 || d > 0) parts.push(`${m} 分`)
  parts.push(`${s} 秒`)
  return parts.join(' ')
}

function formatMiB(mb: number): string {
  if (mb <= 0) return '不限'
  return formatBytes(mb * 1024 * 1024)
}

// ---- SMTP 邮件渠道设置 ----
const smtpSaving = ref(false)
const smtpMeta = ref<SMTPConfig | null>(null)
const smtpForm = reactive({
  enabled: false,
  host: '',
  port: 587,
  username: '',
  password: '',
  from: '',
  use_tls: true,
})

const smtpRules = computed(() => ({
  host: smtpForm.enabled ? [{ required: true, message: '请输入 SMTP 服务器地址', trigger: 'blur' }] : [],
}))

/** 用服务端返回的配置填充表单，清空密码输入框。 */
function fillSMTP(cfg: SMTPConfig): void {
  smtpForm.enabled = cfg.enabled
  smtpForm.host = cfg.host
  smtpForm.port = cfg.port || 587
  smtpForm.username = cfg.username
  smtpForm.from = cfg.from
  smtpForm.use_tls = cfg.use_tls
  smtpForm.password = ''
}

function resetSMTP(): void {
  if (smtpMeta.value) fillSMTP(smtpMeta.value)
}

async function saveSMTP(): Promise<void> {
  if (smtpForm.enabled && !smtpForm.host.trim()) {
    ElMessage.warning('请填写 SMTP 服务器地址')
    return
  }
  smtpSaving.value = true
  try {
    const updated = await updateSMTPConfig({ ...smtpForm })
    smtpMeta.value = updated
    fillSMTP(updated)
    channels.value = await getNotifyChannels().catch(() => channels.value)
    ElMessage.success('SMTP 设置已保存并即时生效')
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    smtpSaving.value = false
  }
}

// ---- 图片广场 AI 审查设置 ----
const moderationSaving = ref(false)
const moderationMeta = ref<ModerationSettings | null>(null)
const moderationForm = reactive({
  enabled: false,
  base_url: '',
  api_key: '',
  model: '',
  timeout_sec: 30,
  prompt: '',
  max_image_mb: 10,
})

/** 用服务端返回的配置填充表单，清空密钥输入框。 */
function fillModeration(cfg: ModerationSettings): void {
  moderationForm.enabled = cfg.enabled
  moderationForm.base_url = cfg.base_url
  moderationForm.model = cfg.model
  moderationForm.timeout_sec = cfg.timeout_sec || 30
  moderationForm.prompt = cfg.prompt
  moderationForm.max_image_mb = cfg.max_image_mb || 10
  moderationForm.api_key = ''
}

async function saveModeration(): Promise<void> {
  if (moderationForm.enabled && !moderationForm.base_url.trim()) {
    ElMessage.warning('请填写接口地址')
    return
  }
  moderationSaving.value = true
  try {
    const updated = await updateModerationSettings({ ...moderationForm })
    moderationMeta.value = updated
    fillModeration(updated)
    ElMessage.success('AI 审查设置已保存并即时生效')
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    moderationSaving.value = false
  }
}

function resetModeration(): void {
  if (moderationMeta.value) fillModeration(moderationMeta.value)
}

// ---- 支付设置 ----
const providerLabels: Record<string, string> = {
  manual: '人工核销',
  mock: '模拟支付',
  alipay: '支付宝',
  wechat: '微信支付',
  epay: '易支付',
}

// knownGateways 是内置渠道，便于在同一次保存中把尚未启用的渠道设为默认。
const knownGateways = ['manual', 'mock', 'alipay', 'wechat', 'epay']

const paymentSaving = ref(false)
const paymentMeta = ref<PaymentSettings | null>(null)
const paymentGateways = ref<string[]>([])
const paymentForm = reactive<PaymentSettingsInput>({
  default_gateway: 'manual',
  alipay: { enabled: false, gateway_url: '', app_id: '', private_key: '', public_key: '' },
  wechat: {
    enabled: false,
    gateway_url: '',
    app_id: '',
    mch_id: '',
    serial_no: '',
    private_key: '',
    api_v3_key: '',
    platform_public_key: '',
    platform_serial_no: '',
  },
  epay: {
    enabled: false,
    pid: '',
    key: '',
    gateway_url: '',
    api_url: '',
    submit_url: '',
    pay_type: 'alipay',
  },
})

const providerLabel = (name: string): string => providerLabels[name] ?? name

/** 可选的默认渠道：内置渠道 ∪ 已注册渠道。 */
const selectableGateways = computed(() => {
  const set = new Set<string>(knownGateways)
  for (const name of paymentGateways.value) {
    set.add(name)
  }
  return Array.from(set)
})

/** 默认渠道选项标签；未启用时标注，便于理解保存后的回退行为。 */
const gatewayOptionLabel = (name: string): string =>
  paymentGateways.value.includes(name) ? providerLabel(name) : `${providerLabel(name)}（未启用）`

/** 用服务端返回的配置填充表单，清空密钥输入框（空表示保持不变）。 */
function fillPayment(cfg: PaymentSettings): void {
  paymentForm.default_gateway = cfg.default_gateway || 'manual'
  paymentForm.alipay.enabled = cfg.alipay.enabled
  paymentForm.alipay.gateway_url = cfg.alipay.gateway_url
  paymentForm.alipay.app_id = cfg.alipay.app_id
  paymentForm.alipay.private_key = ''
  paymentForm.alipay.public_key = ''
  paymentForm.wechat.enabled = cfg.wechat.enabled
  paymentForm.wechat.gateway_url = cfg.wechat.gateway_url
  paymentForm.wechat.app_id = cfg.wechat.app_id
  paymentForm.wechat.mch_id = cfg.wechat.mch_id
  paymentForm.wechat.serial_no = cfg.wechat.serial_no
  paymentForm.wechat.platform_serial_no = cfg.wechat.platform_serial_no
  paymentForm.wechat.private_key = ''
  paymentForm.wechat.api_v3_key = ''
  paymentForm.wechat.platform_public_key = ''
  paymentForm.epay.enabled = cfg.epay.enabled
  paymentForm.epay.pid = cfg.epay.pid
  paymentForm.epay.gateway_url = cfg.epay.gateway_url
  paymentForm.epay.api_url = cfg.epay.api_url
  paymentForm.epay.submit_url = cfg.epay.submit_url
  paymentForm.epay.pay_type = cfg.epay.pay_type || 'alipay'
  paymentForm.epay.key = ''
}

function resetPayment(): void {
  if (paymentMeta.value) fillPayment(paymentMeta.value)
}

const secretPlaceholder = (set: boolean | undefined, label: string): string =>
  set ? `${label}已设置，留空保持不变` : `请输入${label}`

async function savePayment(): Promise<void> {
  paymentSaving.value = true
  try {
    const updated = await updatePaymentSettings({
      default_gateway: paymentForm.default_gateway,
      alipay: { ...paymentForm.alipay },
      wechat: { ...paymentForm.wechat },
      epay: { ...paymentForm.epay },
    })
    paymentMeta.value = updated
    fillPayment(updated)
    paymentGateways.value = await listPaymentGateways().catch(() => paymentGateways.value)
    ElMessage.success('支付设置已保存并即时生效')
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    paymentSaving.value = false
  }
}

// ---- 测试通知 ----
const testChannel = ref<'sms' | 'email'>('email')
const testTo = ref('')
const testBody = ref('')
const testing = ref(false)

async function sendTest(): Promise<void> {
  if (!testTo.value.trim() || !testBody.value.trim()) {
    ElMessage.warning('请填写接收方与内容')
    return
  }
  testing.value = true
  try {
    await sendTestNotify({
      channel: testChannel.value,
      to: testTo.value.trim(),
      subject: testChannel.value === 'email' ? 'AXmiPic 测试邮件' : undefined,
      body: testBody.value.trim(),
    })
    ElMessage.success('测试通知已发送（未配置服务商时会记录到日志）')
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    testing.value = false
  }
}

/** 用服务端返回的权限配置刷新本地状态与运行环境展示。 */
function applyAuth(cfg: AuthConfig): void {
  allowRegistration.value = cfg.allow_registration
  requireAuth.value = cfg.require_auth
  allowGuestUpload.value = cfg.allow_guest_upload
  if (runtime.value) {
    runtime.value.allow_registration = cfg.allow_registration
    runtime.value.require_auth = cfg.require_auth
    runtime.value.allow_guest_upload = cfg.allow_guest_upload
  }
}

/** 在线保存权限开关；失败时回滚为最近一次服务端生效值。 */
async function saveAuth(): Promise<void> {
  authSaving.value = true
  try {
    const updated = await updateAuthSettings({
      allow_registration: allowRegistration.value,
      require_auth: requireAuth.value,
      allow_guest_upload: allowGuestUpload.value,
    })
    applyAuth(updated)
    ElMessage.success('权限设置已保存')
  } catch (error) {
    if (runtime.value) {
      applyAuth({
        allow_registration: runtime.value.allow_registration,
        require_auth: runtime.value.require_auth,
        allow_guest_upload: runtime.value.allow_guest_upload,
      })
    }
    ElMessage.error(toApiError(error).message)
  } finally {
    authSaving.value = false
  }
}

// ---- 运行设置（通用设置域）----
const savingDomain = ref<SettingDomain | ''>('')

const mimeOptions = ['image/jpeg', 'image/png', 'image/gif', 'image/webp']
const formatOptions = ['jpeg', 'png', 'gif', 'webp', 'avif']

const uploadForm = reactive<UploadSettings>({ max_size_mb: 20, allowed_mime_types: [] })
const imagingForm = reactive<ImagingSettings>({
  enabled: true,
  max_width: 4096,
  max_height: 4096,
  default_quality: 82,
  allowed_formats: [],
  allow_enlarge: false,
  allow_effects: true,
  allow_watermark: true,
  watermark_text: '',
})
const securityForm = reactive<SecuritySettings>({ scanner: 'builtin', cloud_processor: 'local' })
const smsForm = reactive<SMSSettings>({ enabled: false, channel: '', provider: '', endpoint: '', method: 'POST' })

// ---- 短信插件渠道 ----
const smsPlugins = ref<PluginDescriptor[]>([])

/** 判断渠道值是否为插件名。 */
function isPluginChannel(channel: string): boolean {
  return !!channel && channel !== 'http' && channel !== 'log'
}

/** 短信渠道下拉选项：日志、通用 HTTP 网关与全部已加载插件。 */
const smsChannelOptions = computed(() => {
  const opts = [
    { label: '日志（调试）', value: 'log' },
    { label: '通用 HTTP 网关', value: 'http' },
  ]
  for (const p of smsPlugins.value) {
    opts.push({ label: p.title || p.name, value: p.name })
  }
  return opts
})

const router = useRouter()
// 已安装插件的完整状态（启用/配置/三态），用于在短信渠道页展示就绪情况。
const installedPlugins = ref<PluginStatus[]>([])

/** 当前选中的插件渠道对应的安装状态。 */
const selectedPlugin = computed<PluginStatus | undefined>(() =>
  installedPlugins.value.find((p) => p.name === smsForm.channel),
)

/** 插件渠道就绪提示；非插件渠道返回 null。 */
const pluginChannelNotice = computed<{ type: 'success' | 'warning' | 'error'; text: string } | null>(() => {
  if (!isPluginChannel(smsForm.channel)) return null
  const p = selectedPlugin.value
  if (!p) return { type: 'error', text: `插件「${smsForm.channel}」未安装，请先到「插件市场」安装。` }
  if (!p.enabled) return { type: 'warning', text: `插件「${p.name}」已暂停，请先在「插件市场」启用。` }
  if (!p.configured) {
    return { type: 'warning', text: `插件「${p.name}」尚未配置，请到「插件市场」填写配置后再启用短信。` }
  }
  return { type: 'success', text: `插件「${p.name}」已就绪（${pluginStateLabel(p)}）。` }
})

/** 插件三态的可读标签。 */
function pluginStateLabel(p?: PluginStatus): string {
  if (!p) return '未安装'
  if (!p.enabled) return '已暂停'
  if (p.state === 'active') return '运行中'
  if (p.state === 'standby') return '待激活'
  return '已启用'
}

/** 跳转到插件市场并打开对应插件的配置弹窗。 */
function goPluginConfig(name: string): void {
  void router.push({ path: '/admin/plugins', query: { config: name } })
}
const limitsForm = reactive<LimitsSettings>({
  upload_per_minute: 30,
  upload_burst: 5,
  guest_per_minute: 6,
  guest_burst: 2,
  image_per_minute: 600,
  image_burst: 120,
  share_per_minute: 30,
  share_burst: 10,
})
const maintenanceForm = reactive<MaintenanceSettings>({
  orphan_cleanup: true,
  orphan_grace_hours: 72,
  orphan_interval_hours: 24,
})
const siteForm = reactive<SiteSettings>({ name: 'AXmiPic', description: '' })

// 客户端真实 IP（来源/可信代理）与校验。
const clientIPForm = reactive<ClientIPSettings>({
  source: 'remote',
  header: '',
  trusted_proxies: [],
  xff_depth: 0,
})
const clientIPChecking = ref(false)
const clientIPInfo = ref<ClientIPInfo | null>(null)
const simulateRemote = ref('')
const simulateHeaders = ref('')
const simulating = ref(false)
const simulateResult = ref<ClientIPInfo | null>(null)

function fillClientIP(v: ClientIPSettings): void {
  Object.assign(clientIPForm, v)
}

async function checkClientIP(): Promise<void> {
  clientIPChecking.value = true
  try {
    clientIPInfo.value = await getClientIPInfo()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    clientIPChecking.value = false
  }
}

async function simulateClientIP(): Promise<void> {
  const headers: Record<string, string> = {}
  for (const line of simulateHeaders.value.split('\n')) {
    const idx = line.indexOf(':')
    if (idx > 0) headers[line.slice(0, idx).trim()] = line.slice(idx + 1).trim()
  }
  simulating.value = true
  try {
    simulateResult.value = await previewClientIP({
      remote_addr: simulateRemote.value.trim(),
      headers,
    })
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    simulating.value = false
  }
}

function fillUpload(v: UploadSettings): void {
  Object.assign(uploadForm, v)
}
function fillImaging(v: ImagingSettings): void {
  Object.assign(imagingForm, v)
}
function fillSecurity(v: SecuritySettings): void {
  Object.assign(securityForm, v)
}
function fillSMS(v: SMSSettings): void {
  Object.assign(smsForm, v)
  // 规范化历史渠道值：未显式设置 channel 时按 endpoint 推断，便于下拉展示。
  if (!smsForm.channel) smsForm.channel = smsForm.endpoint ? 'http' : 'log'
}
function fillLimits(v: LimitsSettings): void {
  Object.assign(limitsForm, v)
}
function fillMaintenance(v: MaintenanceSettings): void {
  Object.assign(maintenanceForm, v)
}
function fillSite(v: SiteSettings): void {
  Object.assign(siteForm, v)
}

/** 保存单个设置域；可热应用的域即时生效，limits/maintenance 重启后生效。 */
async function saveDomain<T>(
  domain: SettingDomain,
  value: unknown,
  apply: (v: T) => void,
): Promise<boolean> {
  savingDomain.value = domain
  try {
    const updated = await updateSettingDomain<T>(domain, value)
    apply(updated)
    ElMessage.success('已保存')
    return true
  } catch (error) {
    ElMessage.error(toApiError(error).message)
    return false
  } finally {
    savingDomain.value = ''
  }
}

/** 保存短信设置，并刷新实际生效渠道；若插件未就绪而回退到日志则明确提示。 */
async function saveSMS(): Promise<void> {
  const ok = await saveDomain('sms', { ...smsForm }, fillSMS)
  if (!ok) return
  try {
    channels.value = await getNotifyChannels()
  } catch {
    // 忽略：仅影响提示，不影响保存结果。
  }
  const want = smsForm.channel
  if (smsForm.enabled && isPluginChannel(want) && channels.value.sms !== want) {
    ElMessage.warning(
      `插件「${want}」未就绪，短信渠道已回退为「${channels.value.sms || 'log'}」；请到「插件市场」启用并配置。`,
    )
  }
}

async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    const [
      statsData,
      runtimeData,
      processData,
      smtpData,
      ch,
      sec,
      drv,
      paymentData,
      gatewayList,
      authData,
      moderationData,
      uploadData,
      imagingData,
      securityData,
      smsData,
      limitsData,
      maintenanceData,
      siteData,
      clientIPData,
      pluginList,
      installedList,
    ] = await Promise.all([
      fetchStats(),
      getRuntimeInfo(),
      getProcessInfo(),
      getSMTPConfig(),
      getNotifyChannels().catch(() => ({ sms: '', email: '' })),
      getSecurityInfo().catch(() => ({ scanner: '' })),
      getImagingDrivers().catch(() => ({ available: [], active: '' })),
      getPaymentSettings().catch(() => null),
      listPaymentGateways().catch(() => []),
      getAuthSettings().catch(() => null),
      getModerationSettings().catch(() => null),
      getSettingDomain<UploadSettings>('upload').catch(() => null),
      getSettingDomain<ImagingSettings>('processing').catch(() => null),
      getSettingDomain<SecuritySettings>('security').catch(() => null),
      getSettingDomain<SMSSettings>('sms').catch(() => null),
      getSettingDomain<LimitsSettings>('limits').catch(() => null),
      getSettingDomain<MaintenanceSettings>('maintenance').catch(() => null),
      getSettingDomain<SiteSettings>('site').catch(() => null),
      getSettingDomain<ClientIPSettings>('client_ip').catch(() => null),
      listPlugins('notify.sms').catch(() => ({ items: [] })),
      listInstalledPlugins().catch(() => ({ items: [] })),
    ])
    stats.value = statsData
    runtime.value = runtimeData
    process.value = processData
    smtpMeta.value = smtpData
    fillSMTP(smtpData)
    channels.value = ch ?? { sms: '', email: '' }
    scannerName.value = sec?.scanner ?? ''
    drivers.value = drv ?? { available: [], active: '' }
    paymentGateways.value = gatewayList ?? []
    if (authData) {
      applyAuth(authData)
    } else {
      applyAuth({
        allow_registration: runtimeData.allow_registration,
        require_auth: runtimeData.require_auth,
        allow_guest_upload: runtimeData.allow_guest_upload,
      })
    }
    if (paymentData) {
      paymentMeta.value = paymentData
      fillPayment(paymentData)
    }
    if (moderationData) {
      moderationMeta.value = moderationData
      fillModeration(moderationData)
    }
    if (uploadData) fillUpload(uploadData)
    if (imagingData) fillImaging(imagingData)
    if (securityData) fillSecurity(securityData)
    smsPlugins.value = pluginList?.items ?? []
    installedPlugins.value = installedList?.items ?? []
    if (smsData) fillSMS(smsData)
    if (limitsData) fillLimits(limitsData)
    if (maintenanceData) fillMaintenance(maintenanceData)
    if (siteData) fillSite(siteData)
    if (clientIPData) fillClientIP(clientIPData)
  } catch (error) {
    errorMessage.value = toApiError(error).message
  } finally {
    loading.value = false
  }
}

/** 仅刷新进程指标，避免整页 loading 闪烁。 */
async function refreshProcess(): Promise<void> {
  try {
    process.value = await getProcessInfo()
  } catch {
    // 进程指标刷新失败时保留上一次的数值。
  }
}

onMounted(load)
onMounted(() => {
  // 进程指标每 5 秒自动刷新一次。
  processTimer = window.setInterval(refreshProcess, 5000)
})
onBeforeUnmount(() => {
  window.clearInterval(processTimer)
})
</script>

<template>
  <div class="ax-page">
    <PageHeader title="系统设置" description="查看实例运行环境，配置通知、支付渠道与系统集成。">
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="load" />

    <div v-else-if="loading && !runtime" class="ax-card" aria-busy="true">
      <div class="ax-card__body"><el-skeleton :rows="6" animated /></div>
    </div>

    <el-tabs v-else v-model="activeTab" class="settings-tabs">
      <!-- 概览 -->
      <el-tab-pane label="概览" name="overview">
        <section class="settings-grid overview-grid">
          <article class="ax-card">
            <header class="ax-card__head">
              <div class="card-head">
                <span class="card-head__icon"><el-icon><Monitor /></el-icon></span>
                <div class="card-head__text">
                  <h2 class="ax-card__title">运行环境</h2>
                  <p class="card-head__sub">站点地址、数据库与运行平台</p>
                </div>
              </div>
            </header>
            <div class="ax-card__body">
              <dl class="info-list">
                <div v-for="row in runtimeRows" :key="row.label" class="info-list__row">
                  <dt>{{ row.label }}</dt>
                  <dd>{{ row.value || '—' }}</dd>
                </div>
              </dl>
            </div>
          </article>

          <article class="ax-card">
            <header class="ax-card__head">
              <div class="card-head">
                <span class="card-head__icon"><el-icon><SetUp /></el-icon></span>
                <div class="card-head__text">
                  <h2 class="ax-card__title">策略概览</h2>
                  <p class="card-head__sub">注册、上传、配额与限流</p>
                </div>
              </div>
              <el-button size="small" @click="activeTab = 'runtime'">配置客户端真实 IP</el-button>
            </header>
            <div class="ax-card__body">
              <div class="policy-switch">
                <div class="policy-switch__text">
                  <span class="policy-switch__label">开放注册</span>
                  <span class="policy-switch__hint">关闭后新用户无法自助注册，已注册用户与管理员不受影响。</span>
                </div>
                <el-switch v-model="allowRegistration" :loading="authSaving" @change="saveAuth" />
              </div>
              <div class="policy-switch">
                <div class="policy-switch__text">
                  <span class="policy-switch__label">上传需要登录</span>
                  <span class="policy-switch__hint">开启后未登录访客不能调用上传接口。</span>
                </div>
                <el-switch v-model="requireAuth" :loading="authSaving" @change="saveAuth" />
              </div>
              <div class="policy-switch">
                <div class="policy-switch__text">
                  <span class="policy-switch__label">允许访客上传</span>
                  <span class="policy-switch__hint">开启后未登录访客以 Guest 角色上传，会覆盖「上传需要登录」。</span>
                </div>
                <el-switch v-model="allowGuestUpload" :loading="authSaving" @change="saveAuth" />
              </div>
              <dl class="info-list">
                <div v-for="row in policyRows" :key="row.label" class="info-list__row">
                  <dt>{{ row.label }}</dt>
                  <dd>{{ row.value }}</dd>
                </div>
              </dl>
            </div>
          </article>
        </section>

        <article v-if="process" class="ax-card settings-block">
          <header class="ax-card__head">
            <div class="card-head">
              <span class="card-head__icon"><el-icon><DataLine /></el-icon></span>
              <div class="card-head__text">
                <h2 class="ax-card__title">进程概览</h2>
                <p class="card-head__sub">Goroutine、内存、GC 与运行时长</p>
              </div>
            </div>
            <span class="process-hint">每 5 秒自动刷新</span>
          </header>
          <div class="ax-card__body process-grid">
            <div v-for="row in processRows" :key="row.label" class="process-item">
              <span class="process-item__label">{{ row.label }}</span>
              <span class="process-item__value">{{ row.value }}</span>
            </div>
          </div>
        </article>

        <article v-if="stats" class="ax-card settings-block">
          <header class="ax-card__head">
            <div class="card-head">
              <span class="card-head__icon"><el-icon><Odometer /></el-icon></span>
              <div class="card-head__text">
                <h2 class="ax-card__title">系统信息</h2>
                <p class="card-head__sub">规模、用量与能力</p>
              </div>
            </div>
          </header>
          <div class="ax-card__body">
            <div class="stat-grid">
              <div class="stat-tile">
                <span class="stat-tile__label">用户数</span>
                <span class="stat-tile__value">{{ formatNumber(stats.users) }}</span>
                <span class="stat-tile__meta">管理员 {{ formatNumber(stats.admins) }}</span>
              </div>
              <div class="stat-tile">
                <span class="stat-tile__label">图片数</span>
                <span class="stat-tile__value">{{ formatNumber(stats.images) }}</span>
                <span class="stat-tile__meta">已存储图片</span>
              </div>
              <div class="stat-tile">
                <span class="stat-tile__label">存储总用量</span>
                <span class="stat-tile__value">{{ formatBytes(stats.total_bytes) }}</span>
                <span class="stat-tile__meta">全部存储后端</span>
              </div>
            </div>
            <dl class="info-list">
              <div class="info-list__row">
                <dt>存储驱动</dt>
                <dd><el-tag size="small" effect="plain">{{ stats.storage_driver }}</el-tag></dd>
              </div>
              <div class="info-list__row">
                <dt>处理器</dt>
                <dd><el-tag size="small" effect="plain">{{ stats.processor }}</el-tag></dd>
              </div>
              <div class="info-list__row">
                <dt>支持格式</dt>
                <dd>
                  <span v-if="stats.formats && stats.formats.length" class="ax-cluster">
                    <el-tag
                      v-for="format in stats.formats"
                      :key="format"
                      size="small"
                      type="info"
                      effect="plain"
                    >
                      {{ format.toUpperCase() }}
                    </el-tag>
                  </span>
                  <span v-else class="ax-muted">—</span>
                </dd>
              </div>
            </dl>
          </div>
        </article>
      </el-tab-pane>

      <!-- 通知设置 -->
      <el-tab-pane label="通知设置" name="notify">
        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">SMTP 邮件设置</h2>
            <el-tag v-if="smtpMeta?.enabled" size="small" type="success" effect="plain">已启用</el-tag>
            <el-tag v-else size="small" type="info" effect="plain">未启用</el-tag>
          </header>
          <div class="ax-card__body">
            <el-form
              :model="smtpForm"
              :rules="smtpRules"
              label-position="top"
              class="smtp-form"
              @submit.prevent
            >
              <div class="smtp-grid">
                <el-form-item label="启用 SMTP" prop="enabled">
                  <el-switch v-model="smtpForm.enabled" />
                </el-form-item>
                <el-form-item label="服务器地址" prop="host" class="smtp-grid__wide">
                  <el-input v-model="smtpForm.host" placeholder="smtp.example.com" />
                </el-form-item>
                <el-form-item label="端口" prop="port">
                  <el-input-number v-model="smtpForm.port" :min="1" :max="65535" controls-position="right" />
                </el-form-item>
                <el-form-item label="用户名">
                  <el-input v-model="smtpForm.username" placeholder="account@example.com" />
                </el-form-item>
                <el-form-item label="密码 / 授权码">
                  <el-input
                    v-model="smtpForm.password"
                    type="password"
                    show-password
                    autocomplete="new-password"
                    :placeholder="smtpMeta?.password_set ? '已设置，留空保持不变' : '请输入密码或授权码'"
                  />
                </el-form-item>
                <el-form-item label="发件人">
                  <el-input v-model="smtpForm.from" placeholder="默认与用户名相同" />
                </el-form-item>
                <el-form-item label="隐式 TLS">
                  <el-switch v-model="smtpForm.use_tls" />
                  <span class="smtp-form__hint">465 端口开启；587 端口使用 STARTTLS</span>
                </el-form-item>
              </div>
              <div class="smtp-actions">
                <el-button type="primary" :loading="smtpSaving" @click="saveSMTP">保存并应用</el-button>
                <el-button :disabled="smtpSaving" @click="resetSMTP">重置</el-button>
              </div>
            </el-form>
          </div>
        </article>

        <article class="ax-card settings-block">
          <header class="ax-card__head">
            <h2 class="ax-card__title">通知渠道与测试</h2>
          </header>
          <div class="ax-card__body integration">
            <dl class="info-list">
              <div class="info-list__row">
                <dt>短信渠道</dt>
                <dd>{{ channels.sms || '未配置' }}</dd>
              </div>
              <div class="info-list__row">
                <dt>邮件渠道</dt>
                <dd>{{ channels.email || '未配置' }}</dd>
              </div>
            </dl>
            <el-divider content-position="left">发送测试通知</el-divider>
            <div class="integration__form">
              <el-radio-group v-model="testChannel">
                <el-radio-button value="sms">短信</el-radio-button>
                <el-radio-button value="email">邮件</el-radio-button>
              </el-radio-group>
              <el-input
                v-model="testTo"
                :placeholder="testChannel === 'sms' ? '手机号' : '邮箱地址'"
              />
              <el-input v-model="testBody" type="textarea" :rows="2" placeholder="通知内容" />
              <el-button type="primary" :loading="testing" @click="sendTest">发送测试</el-button>
            </div>
            <p class="integration__hint">
              保存 SMTP 设置后即时生效；未配置真实服务商时，通知会回退到日志渠道并记录到服务端日志。
            </p>
          </div>
        </article>

        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">短信渠道</h2>
            <el-tag size="small" type="success" effect="plain">即时生效</el-tag>
          </header>
          <div class="ax-card__body">
            <el-form label-position="top" class="smtp-form" @submit.prevent>
              <div class="smtp-grid">
                <el-form-item label="启用短信">
                  <el-switch v-model="smsForm.enabled" />
                </el-form-item>
                <el-form-item label="渠道">
                  <el-select v-model="smsForm.channel" style="width: 100%">
                    <el-option
                      v-for="opt in smsChannelOptions"
                      :key="opt.value"
                      :label="opt.label"
                      :value="opt.value"
                    />
                  </el-select>
                </el-form-item>

                <template v-if="smsForm.channel === 'http'">
                  <el-form-item label="服务商名称">
                    <el-input v-model="smsForm.provider" placeholder="例如 aliyun" />
                  </el-form-item>
                  <el-form-item label="HTTP 网关地址" class="smtp-grid__wide">
                    <el-input v-model="smsForm.endpoint" placeholder="https://sms.example.com/send" />
                  </el-form-item>
                  <el-form-item label="请求方法">
                    <el-select v-model="smsForm.method" style="width: 100%">
                      <el-option label="POST" value="POST" />
                      <el-option label="GET" value="GET" />
                    </el-select>
                  </el-form-item>
                </template>

                <el-form-item
                  v-else-if="!isPluginChannel(smsForm.channel)"
                  label="说明"
                  class="smtp-grid__wide"
                >
                  <span class="smtp-form__hint">日志渠道仅记录发送内容，便于本地调试。</span>
                </el-form-item>
              </div>

              <template v-if="isPluginChannel(smsForm.channel)">
                <el-form-item label="插件状态">
                  <el-tag size="small" :type="selectedPlugin?.enabled ? 'success' : 'info'" effect="plain">
                    {{ pluginStateLabel(selectedPlugin) }}
                  </el-tag>
                  <el-tag
                    size="small"
                    :type="selectedPlugin?.configured ? 'warning' : 'danger'"
                    effect="plain"
                    style="margin-left: 8px"
                  >
                    {{ selectedPlugin?.configured ? '已配置' : '未配置' }}
                  </el-tag>
                  <el-button link type="primary" style="margin-left: 8px" @click="goPluginConfig(smsForm.channel)">
                    去插件市场配置
                  </el-button>
                </el-form-item>
                <el-alert
                  v-if="pluginChannelNotice"
                  :title="pluginChannelNotice.text"
                  :type="pluginChannelNotice.type"
                  :closable="false"
                  show-icon
                  style="margin-bottom: 12px"
                />
              </template>

              <div class="smtp-actions">
                <el-button type="primary" :loading="savingDomain === 'sms'" @click="saveSMS">
                  保存并应用
                </el-button>
                <span v-if="isPluginChannel(smsForm.channel)" class="smtp-form__hint">
                  插件凭据在「插件市场」中配置
                </span>
              </div>
            </el-form>
          </div>
        </article>
      </el-tab-pane>

      <!-- 支付设置 -->
      <el-tab-pane label="支付设置" name="payment">
        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">支付设置</h2>
            <el-tag size="small" type="info" effect="plain">保存后即时生效</el-tag>
          </header>
          <div class="ax-card__body">
            <el-form label-position="top" class="smtp-form" @submit.prevent>
              <el-form-item label="默认支付渠道">
                <el-select v-model="paymentForm.default_gateway" style="max-width: 260px">
                  <el-option
                    v-for="gateway in selectableGateways"
                    :key="gateway"
                    :label="gatewayOptionLabel(gateway)"
                    :value="gateway"
                  />
                </el-select>
                <span class="smtp-form__hint">可选择任意内置渠道；保存后未启用的渠道会自动回退为人工核销</span>
              </el-form-item>
            </el-form>
            <div class="smtp-actions">
              <el-button type="primary" :loading="paymentSaving" @click="savePayment">保存并应用</el-button>
              <el-button :disabled="paymentSaving" @click="resetPayment">重置</el-button>
            </div>
            <p class="integration__hint">
              密钥以密文保存，接口仅返回是否已设置（留空保持不变）；保存后立即注册或切换支付渠道，无需重启。
            </p>
          </div>
        </article>

        <section class="settings-grid gateway-grid">
          <article class="ax-card gateway-card">
            <header class="ax-card__head">
              <h2 class="ax-card__title">支付宝当面付</h2>
              <el-tag size="small" :type="paymentForm.alipay.enabled ? 'success' : 'info'" effect="plain">
                {{ paymentForm.alipay.enabled ? '已启用' : '未启用' }}
              </el-tag>
            </header>
            <div class="ax-card__body">
              <p class="gateway-card__hint">扫码支付，需应用私钥与支付宝公钥，回调自动验签。</p>
              <el-form label-position="top" class="smtp-form" @submit.prevent>
                <div class="smtp-grid">
                  <el-form-item label="启用">
                    <el-switch v-model="paymentForm.alipay.enabled" />
                  </el-form-item>
                  <el-form-item label="网关地址" class="smtp-grid__wide">
                    <el-input v-model="paymentForm.alipay.gateway_url" placeholder="留空使用生产地址" />
                  </el-form-item>
                  <el-form-item label="App ID">
                    <el-input v-model="paymentForm.alipay.app_id" />
                  </el-form-item>
                  <el-form-item label="应用私钥">
                    <el-input
                      v-model="paymentForm.alipay.private_key"
                      type="password"
                      show-password
                      autocomplete="new-password"
                      :placeholder="secretPlaceholder(paymentMeta?.alipay.private_key_set, '私钥')"
                    />
                  </el-form-item>
                  <el-form-item label="支付宝公钥">
                    <el-input
                      v-model="paymentForm.alipay.public_key"
                      type="password"
                      show-password
                      autocomplete="new-password"
                      :placeholder="secretPlaceholder(paymentMeta?.alipay.public_key_set, '公钥')"
                    />
                  </el-form-item>
                </div>
              </el-form>
            </div>
          </article>

          <article class="ax-card gateway-card">
            <header class="ax-card__head">
              <h2 class="ax-card__title">微信支付 v3</h2>
              <el-tag size="small" :type="paymentForm.wechat.enabled ? 'success' : 'info'" effect="plain">
                {{ paymentForm.wechat.enabled ? '已启用' : '未启用' }}
              </el-tag>
            </header>
            <div class="ax-card__body">
              <p class="gateway-card__hint">Native 扫码，基于官方 SDK 下单并对回调验签/解密。</p>
              <el-form label-position="top" class="smtp-form" @submit.prevent>
                <div class="smtp-grid">
                  <el-form-item label="启用">
                    <el-switch v-model="paymentForm.wechat.enabled" />
                  </el-form-item>
                  <el-form-item label="网关地址" class="smtp-grid__wide">
                    <el-input v-model="paymentForm.wechat.gateway_url" placeholder="留空使用生产地址" />
                  </el-form-item>
                  <el-form-item label="App ID">
                    <el-input v-model="paymentForm.wechat.app_id" />
                  </el-form-item>
                  <el-form-item label="商户号">
                    <el-input v-model="paymentForm.wechat.mch_id" />
                  </el-form-item>
                  <el-form-item label="证书序列号">
                    <el-input v-model="paymentForm.wechat.serial_no" />
                  </el-form-item>
                  <el-form-item label="商户 API 私钥">
                    <el-input
                      v-model="paymentForm.wechat.private_key"
                      type="password"
                      show-password
                      autocomplete="new-password"
                      :placeholder="secretPlaceholder(paymentMeta?.wechat.private_key_set, '私钥')"
                    />
                  </el-form-item>
                  <el-form-item label="APIv3 密钥（32 字节）">
                    <el-input
                      v-model="paymentForm.wechat.api_v3_key"
                      type="password"
                      show-password
                      autocomplete="new-password"
                      :placeholder="secretPlaceholder(paymentMeta?.wechat.api_v3_key_set, '密钥')"
                    />
                  </el-form-item>
                  <el-form-item label="平台公钥">
                    <el-input
                      v-model="paymentForm.wechat.platform_public_key"
                      type="password"
                      show-password
                      autocomplete="new-password"
                      :placeholder="secretPlaceholder(paymentMeta?.wechat.platform_public_key_set, '公钥')"
                    />
                  </el-form-item>
                  <el-form-item label="平台证书序列号">
                    <el-input v-model="paymentForm.wechat.platform_serial_no" placeholder="可选，用于校验回调 serial" />
                  </el-form-item>
                </div>
              </el-form>
            </div>
          </article>

          <article class="ax-card gateway-card">
            <header class="ax-card__head">
              <h2 class="ax-card__title">易支付</h2>
              <el-tag size="small" :type="paymentForm.epay.enabled ? 'success' : 'info'" effect="plain">
                {{ paymentForm.epay.enabled ? '已启用' : '未启用' }}
              </el-tag>
            </header>
            <div class="ax-card__body">
              <p class="gateway-card__hint">彩虹易支付兼容聚合支付，MD5 签名下单与回调验签。</p>
              <el-form label-position="top" class="smtp-form" @submit.prevent>
                <div class="smtp-grid">
                  <el-form-item label="启用">
                    <el-switch v-model="paymentForm.epay.enabled" />
                  </el-form-item>
                  <el-form-item label="支付通道">
                    <el-select v-model="paymentForm.epay.pay_type" style="width: 100%">
                      <el-option label="支付宝" value="alipay" />
                      <el-option label="微信支付" value="wxpay" />
                      <el-option label="QQ 钱包" value="qqpay" />
                    </el-select>
                  </el-form-item>
                  <el-form-item label="商户号 PID">
                    <el-input v-model="paymentForm.epay.pid" />
                  </el-form-item>
                  <el-form-item label="商户密钥">
                    <el-input
                      v-model="paymentForm.epay.key"
                      type="password"
                      show-password
                      autocomplete="new-password"
                      :placeholder="secretPlaceholder(paymentMeta?.epay.key_set, '密钥')"
                    />
                  </el-form-item>
                  <el-form-item label="站点地址" class="smtp-grid__wide">
                    <el-input v-model="paymentForm.epay.gateway_url" placeholder="https://pay.example.com" />
                  </el-form-item>
                  <el-form-item label="下单接口">
                    <el-input v-model="paymentForm.epay.api_url" placeholder="默认 /mapi.php" />
                  </el-form-item>
                  <el-form-item label="收银台地址">
                    <el-input v-model="paymentForm.epay.submit_url" placeholder="默认 /submit.php" />
                  </el-form-item>
                </div>
              </el-form>
            </div>
          </article>
        </section>
      </el-tab-pane>

      <!-- AI 审查 -->
      <el-tab-pane label="AI 审查" name="moderation">
        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">图片广场 AI 审查</h2>
            <el-tag v-if="moderationMeta?.enabled" size="small" type="success" effect="plain">已启用</el-tag>
            <el-tag v-else size="small" type="info" effect="plain">未启用</el-tag>
          </header>
          <div class="ax-card__body">
            <el-form :model="moderationForm" label-position="top" class="smtp-form" @submit.prevent>
              <div class="smtp-grid">
                <el-form-item label="启用 AI 审查">
                  <el-switch v-model="moderationForm.enabled" />
                </el-form-item>
                <el-form-item label="接口地址" class="smtp-grid__wide">
                  <el-input v-model="moderationForm.base_url" placeholder="https://api.openai.com/v1" />
                </el-form-item>
                <el-form-item label="模型">
                  <el-input v-model="moderationForm.model" placeholder="gpt-4o-mini" />
                </el-form-item>
                <el-form-item label="API 密钥">
                  <el-input
                    v-model="moderationForm.api_key"
                    type="password"
                    show-password
                    autocomplete="new-password"
                    :placeholder="moderationMeta?.api_key_set ? '已设置，留空保持不变' : '请输入 API 密钥'"
                  />
                </el-form-item>
                <el-form-item label="超时（秒）">
                  <el-input-number v-model="moderationForm.timeout_sec" :min="1" :max="300" controls-position="right" />
                </el-form-item>
                <el-form-item label="送审体积上限 (MiB)">
                  <el-input-number v-model="moderationForm.max_image_mb" :min="1" :max="100" controls-position="right" />
                </el-form-item>
                <el-form-item label="审查提示词" class="smtp-grid__wide">
                  <el-input
                    v-model="moderationForm.prompt"
                    type="textarea"
                    :rows="4"
                    placeholder="留空将恢复内置默认提示词（要求模型只回答 SAFE 或 UNSAFE）"
                  />
                </el-form-item>
              </div>
              <p class="smtp-form__hint">
                仅作用于图片广场：图片「设为公开」时调用标准 OpenAI 兼容视觉接口审查，
                未通过（含模型无返回、调用失败）则保持私有。保存后即时生效，无需重启。
              </p>
              <div class="smtp-actions">
                <el-button type="primary" :loading="moderationSaving" @click="saveModeration">保存并应用</el-button>
                <el-button :disabled="moderationSaving || !moderationMeta" @click="resetModeration">重置</el-button>
              </div>
            </el-form>
          </div>
        </article>
      </el-tab-pane>

      <!-- 系统集成 -->
      <el-tab-pane label="系统集成" name="integration">
        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">系统集成</h2>
          </header>
          <div class="ax-card__body">
            <dl class="info-list">
              <div class="info-list__row">
                <dt>内容扫描器</dt>
                <dd>{{ scannerName || '未配置' }}</dd>
              </div>
              <div class="info-list__row">
                <dt>处理驱动</dt>
                <dd>
                  {{ drivers.active || '—' }}<span class="ax-muted">（可用：{{ drivers.available.join('、') || '—' }}）</span>
                </dd>
              </div>
            </dl>
          </div>
        </article>
      </el-tab-pane>
      <!-- 运行设置 -->
      <el-tab-pane label="运行设置" name="runtime">
        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">上传限制</h2>
            <el-tag size="small" type="success" effect="plain">即时生效</el-tag>
          </header>
          <div class="ax-card__body">
            <el-form label-position="top" class="smtp-form" @submit.prevent>
              <div class="smtp-grid">
                <el-form-item label="单文件上限 (MiB)">
                  <el-input-number v-model="uploadForm.max_size_mb" :min="1" :max="10240" controls-position="right" />
                </el-form-item>
                <el-form-item label="允许的媒体类型" class="smtp-grid__wide">
                  <el-select v-model="uploadForm.allowed_mime_types" multiple style="width: 100%">
                    <el-option v-for="m in mimeOptions" :key="m" :label="m" :value="m" />
                  </el-select>
                </el-form-item>
              </div>
              <div class="smtp-actions">
                <el-button
                  type="primary"
                  :loading="savingDomain === 'upload'"
                  @click="saveDomain('upload', { ...uploadForm }, fillUpload)"
                >
                  保存并应用
                </el-button>
              </div>
            </el-form>
          </div>
        </article>

        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">图像处理</h2>
            <el-tag size="small" type="success" effect="plain">即时生效</el-tag>
          </header>
          <div class="ax-card__body">
            <el-form label-position="top" class="smtp-form" @submit.prevent>
              <div class="smtp-grid">
                <el-form-item label="启用即时处理">
                  <el-switch v-model="imagingForm.enabled" />
                </el-form-item>
                <el-form-item label="最大宽度">
                  <el-input-number v-model="imagingForm.max_width" :min="1" :max="20000" controls-position="right" />
                </el-form-item>
                <el-form-item label="最大高度">
                  <el-input-number v-model="imagingForm.max_height" :min="1" :max="20000" controls-position="right" />
                </el-form-item>
                <el-form-item label="默认质量">
                  <el-input-number v-model="imagingForm.default_quality" :min="1" :max="100" controls-position="right" />
                </el-form-item>
                <el-form-item label="允许的输出格式" class="smtp-grid__wide">
                  <el-select v-model="imagingForm.allowed_formats" multiple style="width: 100%">
                    <el-option v-for="f in formatOptions" :key="f" :label="f" :value="f" />
                  </el-select>
                </el-form-item>
                <el-form-item label="允许放大">
                  <el-switch v-model="imagingForm.allow_enlarge" />
                </el-form-item>
                <el-form-item label="允许滤镜">
                  <el-switch v-model="imagingForm.allow_effects" />
                </el-form-item>
                <el-form-item label="允许水印">
                  <el-switch v-model="imagingForm.allow_watermark" />
                </el-form-item>
                <el-form-item label="强制水印文字" class="smtp-grid__wide">
                  <el-input v-model="imagingForm.watermark_text" placeholder="留空表示不强制" />
                </el-form-item>
              </div>
              <p class="smtp-form__hint">处理驱动（purego/libvips/magick）在启动时选定，修改驱动需重启。</p>
              <div class="smtp-actions">
                <el-button
                  type="primary"
                  :loading="savingDomain === 'processing'"
                  @click="saveDomain('processing', { ...imagingForm }, fillImaging)"
                >
                  保存并应用
                </el-button>
              </div>
            </el-form>
          </div>
        </article>

        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">安全扫描</h2>
            <el-tag size="small" type="success" effect="plain">即时生效</el-tag>
          </header>
          <div class="ax-card__body">
            <el-form label-position="top" class="smtp-form" @submit.prevent>
              <div class="smtp-grid">
                <el-form-item label="内容扫描器">
                  <el-select v-model="securityForm.scanner" style="width: 100%">
                    <el-option label="关闭（none）" value="none" />
                    <el-option label="内置（白名单+魔数）" value="builtin" />
                  </el-select>
                </el-form-item>
                <el-form-item label="云处理">
                  <el-select v-model="securityForm.cloud_processor" style="width: 100%">
                    <el-option label="本地（local）" value="local" />
                  </el-select>
                </el-form-item>
              </div>
              <div class="smtp-actions">
                <el-button
                  type="primary"
                  :loading="savingDomain === 'security'"
                  @click="saveDomain('security', { ...securityForm }, fillSecurity)"
                >
                  保存并应用
                </el-button>
              </div>
            </el-form>
          </div>
        </article>

        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">客户端真实 IP</h2>
            <el-tag size="small" type="success" effect="plain">即时生效</el-tag>
          </header>
          <div class="ax-card__body">
            <el-form label-position="top" class="smtp-form" @submit.prevent>
              <div class="smtp-grid">
                <el-form-item label="来源">
                  <el-select v-model="clientIPForm.source" style="width: 100%">
                    <el-option label="remote（忽略转发头）" value="remote" />
                    <el-option label="X-Forwarded-For" value="x-forwarded-for" />
                    <el-option label="X-Real-IP" value="x-real-ip" />
                    <el-option label="CF-Connecting-IP" value="cf-connecting-ip" />
                    <el-option label="True-Client-IP" value="true-client-ip" />
                    <el-option label="X-Client-IP" value="x-client-ip" />
                    <el-option label="Forwarded（RFC 7239）" value="forwarded" />
                    <el-option label="自定义头" value="custom" />
                  </el-select>
                </el-form-item>
                <el-form-item v-if="clientIPForm.source === 'custom'" label="自定义头名">
                  <el-input v-model="clientIPForm.header" placeholder="例如 X-Real-Client" />
                </el-form-item>
                <el-form-item label="XFF 右侧跳过数（0=右起第一个不可信）">
                  <el-input-number v-model="clientIPForm.xff_depth" :min="0" controls-position="right" />
                </el-form-item>
                <el-form-item label="可信代理 CIDR" class="smtp-grid__wide">
                  <el-select
                    v-model="clientIPForm.trusted_proxies"
                    multiple
                    filterable
                    allow-create
                    default-first-option
                    placeholder="输入 CIDR 后回车，如 10.0.0.0/8"
                    style="width: 100%"
                  >
                    <el-option
                      v-for="c in clientIPForm.trusted_proxies"
                      :key="c"
                      :label="c"
                      :value="c"
                    />
                  </el-select>
                </el-form-item>
              </div>
              <p class="smtp-form__hint">
                选择代理来源时必须配置可信代理 CIDR：仅当直接对端在可信网段内才解析转发头，否则回退对端地址（防伪造）。
              </p>
              <div class="smtp-actions">
                <el-button
                  type="primary"
                  :loading="savingDomain === 'client_ip'"
                  @click="saveDomain('client_ip', { ...clientIPForm }, fillClientIP)"
                >
                  保存并应用
                </el-button>
                <el-button :loading="clientIPChecking" @click="checkClientIP">校验当前请求</el-button>
              </div>
            </el-form>

            <dl v-if="clientIPInfo" class="info-list">
              <div class="info-list__row">
                <dt>解析到的 IP</dt>
                <dd>{{ clientIPInfo.resolved || '—' }}</dd>
              </div>
              <div class="info-list__row">
                <dt>直接对端</dt>
                <dd>{{ clientIPInfo.peer || '—' }}</dd>
              </div>
              <div class="info-list__row">
                <dt>来源 / 命中可信代理</dt>
                <dd>{{ clientIPInfo.source }} · {{ clientIPInfo.trusted_peer ? '是' : '否' }}</dd>
              </div>
              <div class="info-list__row">
                <dt>X-Forwarded-For</dt>
                <dd>{{ clientIPInfo.x_forwarded_for || '—' }}</dd>
              </div>
              <div class="info-list__row">
                <dt>X-Real-IP</dt>
                <dd>{{ clientIPInfo.x_real_ip || '—' }}</dd>
              </div>
            </dl>

            <el-divider content-position="left">模拟解析（验证规则）</el-divider>
            <el-form label-position="top" class="smtp-form" @submit.prevent>
              <div class="smtp-grid">
                <el-form-item label="对端地址（RemoteAddr）">
                  <el-input v-model="simulateRemote" placeholder="10.0.0.5:1234" />
                </el-form-item>
                <el-form-item label="请求头（每行 Key: Value）" class="smtp-grid__wide">
                  <el-input
                    v-model="simulateHeaders"
                    type="textarea"
                    :rows="2"
                    placeholder="X-Forwarded-For: 1.2.3.4, 10.0.0.7"
                  />
                </el-form-item>
              </div>
              <div class="smtp-actions">
                <el-button :loading="simulating" @click="simulateClientIP">模拟解析</el-button>
              </div>
            </el-form>
            <dl v-if="simulateResult" class="info-list">
              <div class="info-list__row">
                <dt>解析到的 IP</dt>
                <dd>{{ simulateResult.resolved || '—' }}</dd>
              </div>
              <div class="info-list__row">
                <dt>直接对端</dt>
                <dd>{{ simulateResult.peer || '—' }}</dd>
              </div>
              <div class="info-list__row">
                <dt>来源 / 命中可信代理</dt>
                <dd>{{ simulateResult.source }} · {{ simulateResult.trusted_peer ? '是' : '否' }}</dd>
              </div>
            </dl>
          </div>
        </article>

        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">限流</h2>
            <el-tag size="small" type="info" effect="plain">重启后生效</el-tag>
          </header>
          <div class="ax-card__body">
            <el-form label-position="top" class="smtp-form" @submit.prevent>
              <div class="smtp-grid">
                <el-form-item label="上传/分钟">
                  <el-input-number v-model="limitsForm.upload_per_minute" :min="0" controls-position="right" />
                </el-form-item>
                <el-form-item label="上传突发">
                  <el-input-number v-model="limitsForm.upload_burst" :min="0" controls-position="right" />
                </el-form-item>
                <el-form-item label="访客/分钟">
                  <el-input-number v-model="limitsForm.guest_per_minute" :min="0" controls-position="right" />
                </el-form-item>
                <el-form-item label="访客突发">
                  <el-input-number v-model="limitsForm.guest_burst" :min="0" controls-position="right" />
                </el-form-item>
                <el-form-item label="图片读取/分钟">
                  <el-input-number v-model="limitsForm.image_per_minute" :min="0" controls-position="right" />
                </el-form-item>
                <el-form-item label="图片读取突发">
                  <el-input-number v-model="limitsForm.image_burst" :min="0" controls-position="right" />
                </el-form-item>
                <el-form-item label="分享访问/分钟">
                  <el-input-number v-model="limitsForm.share_per_minute" :min="0" controls-position="right" />
                </el-form-item>
                <el-form-item label="分享访问突发">
                  <el-input-number v-model="limitsForm.share_burst" :min="0" controls-position="right" />
                </el-form-item>
              </div>
              <div class="smtp-actions">
                <el-button
                  type="primary"
                  :loading="savingDomain === 'limits'"
                  @click="saveDomain('limits', { ...limitsForm }, fillLimits)"
                >
                  保存
                </el-button>
              </div>
            </el-form>
          </div>
        </article>

        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">维护任务</h2>
            <el-tag size="small" type="info" effect="plain">重启后生效</el-tag>
          </header>
          <div class="ax-card__body">
            <el-form label-position="top" class="smtp-form" @submit.prevent>
              <div class="smtp-grid">
                <el-form-item label="启用孤儿对象对账">
                  <el-switch v-model="maintenanceForm.orphan_cleanup" />
                </el-form-item>
                <el-form-item label="保留时长（小时）">
                  <el-input-number v-model="maintenanceForm.orphan_grace_hours" :min="0" controls-position="right" />
                </el-form-item>
                <el-form-item label="执行间隔（小时）">
                  <el-input-number v-model="maintenanceForm.orphan_interval_hours" :min="1" controls-position="right" />
                </el-form-item>
              </div>
              <div class="smtp-actions">
                <el-button
                  type="primary"
                  :loading="savingDomain === 'maintenance'"
                  @click="saveDomain('maintenance', { ...maintenanceForm }, fillMaintenance)"
                >
                  保存
                </el-button>
              </div>
            </el-form>
          </div>
        </article>

        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">站点信息</h2>
          </header>
          <div class="ax-card__body">
            <el-form label-position="top" class="smtp-form" @submit.prevent>
              <div class="smtp-grid">
                <el-form-item label="站点名称">
                  <el-input v-model="siteForm.name" />
                </el-form-item>
                <el-form-item label="站点描述" class="smtp-grid__wide">
                  <el-input v-model="siteForm.description" type="textarea" :rows="2" />
                </el-form-item>
              </div>
              <div class="smtp-actions">
                <el-button
                  type="primary"
                  :loading="savingDomain === 'site'"
                  @click="saveDomain('site', { ...siteForm }, fillSite)"
                >
                  保存
                </el-button>
              </div>
            </el-form>
          </div>
        </article>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<style scoped>
.settings-tabs :deep(.el-tabs__header) {
  margin-bottom: var(--ax-space-4);
}

.settings-grid {
  display: grid;
  gap: var(--ax-space-4);
  grid-template-columns: repeat(auto-fit, minmax(min(320px, 100%), 1fr));
  align-items: start;
}

.settings-block {
  margin-top: var(--ax-space-4);
}

/* 概览双卡等高：运行环境与策略概览底边对齐，列表在卡内均分。 */
.overview-grid {
  align-items: stretch;
}

.overview-grid > .ax-card {
  display: flex;
  flex-direction: column;
}

.overview-grid > .ax-card > .ax-card__body {
  display: flex;
  flex-direction: column;
  flex: 1;
}

.overview-grid .info-list {
  flex: 1;
  justify-content: space-between;
}

/* 策略概览中的在线开关行。 */
.policy-switch {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--ax-space-3);
  margin-bottom: var(--ax-space-3);
  padding-bottom: var(--ax-space-3);
  border-bottom: 1px solid var(--ax-border-subtle);
}

.policy-switch__text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.policy-switch__label {
  color: var(--ax-text);
  font-size: var(--ax-text-sm);
  font-weight: var(--ax-weight-medium);
}

.policy-switch__hint {
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
  line-height: 1.5;
}

.gateway-grid {
  margin-top: var(--ax-space-4);
  grid-template-columns: repeat(auto-fit, minmax(min(320px, 100%), 1fr));
}

.gateway-card {
  height: 100%;
  min-width: 0;
}

/* 卡片内每个字段各占一行。 */
.gateway-card .smtp-grid {
  grid-template-columns: 1fr;
}

/* 单列时取消「宽字段跨列」，避免强制两列。 */
.gateway-card .smtp-grid__wide {
  grid-column: auto;
}

.gateway-card__hint {
  margin: 0 0 var(--ax-space-3);
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
  line-height: 1.5;
}

/* 概览卡片头：图标 + 标题 + 副标题。 */
.card-head {
  display: flex;
  align-items: center;
  gap: var(--ax-space-3);
  min-width: 0;
}

.card-head__icon {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  border-radius: var(--ax-radius-md);
  background: var(--ax-accent-soft);
  color: var(--ax-accent-bright);
  font-size: 18px;
}

.card-head__text {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.card-head__sub {
  margin: 2px 0 0;
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

/* 系统信息的统计方块。 */
.stat-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(160px, 100%), 1fr));
  gap: var(--ax-space-3);
  margin-bottom: var(--ax-space-4);
}

.stat-tile {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: var(--ax-space-3) var(--ax-space-4);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-md);
  background: linear-gradient(180deg, var(--ax-tint-weak), transparent);
}

.stat-tile__label {
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.stat-tile__value {
  color: var(--ax-text);
  font-size: var(--ax-text-xl);
  font-weight: var(--ax-weight-semibold);
  font-variant-numeric: tabular-nums;
  letter-spacing: var(--ax-tracking-tight);
}

.stat-tile__meta {
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.process-hint {
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.process-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(200px, 100%), 1fr));
  gap: var(--ax-space-3);
}

.process-item {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: var(--ax-space-3) var(--ax-space-4) var(--ax-space-3) calc(var(--ax-space-4) + 4px);
  background: var(--ax-tint-weak);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-md);
  overflow: hidden;
  transition: background var(--ax-duration-fast) var(--ax-ease),
    border-color var(--ax-duration-fast) var(--ax-ease);
}

.process-item::before {
  content: '';
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 3px;
  background: var(--ax-accent);
  opacity: 0.55;
}

.process-item:hover {
  background: var(--ax-tint);
  border-color: var(--ax-border);
}

.process-item__label {
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.process-item__value {
  color: var(--ax-text);
  font-size: var(--ax-text-lg);
  font-weight: var(--ax-weight-semibold);
  font-variant-numeric: tabular-nums;
}

.smtp-form {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-3);
}

.smtp-grid {
  display: grid;
  gap: var(--ax-space-2) var(--ax-space-4);
  grid-template-columns: repeat(auto-fit, minmax(min(220px, 100%), 1fr));
}

.smtp-grid__wide {
  grid-column: span 2;
}

.smtp-grid :deep(.el-form-item) {
  margin-bottom: 0;
}

.smtp-form__hint {
  margin-left: var(--ax-space-2);
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.smtp-actions {
  display: flex;
  gap: var(--ax-space-2);
}

.integration {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-3);
}

.integration__form {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-2);
}

.integration__hint {
  margin: 0;
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.info-list {
  display: flex;
  flex-direction: column;
  margin: 0;
  width: 100%;
}

.info-list__row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--ax-space-4);
  min-width: 0;
  padding: var(--ax-space-2) var(--ax-space-2);
  border-radius: var(--ax-radius-sm);
  transition: background var(--ax-duration-fast) var(--ax-ease);
}

.info-list__row + .info-list__row {
  border-top: 1px solid var(--ax-border-subtle);
}

.info-list__row:hover {
  background: var(--ax-tint-weak);
}

.info-list__row dt {
  flex: none;
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
}

.info-list__row dd {
  margin: 0;
  min-width: 0;
  color: var(--ax-text-2);
  font-size: var(--ax-text-sm);
  font-weight: var(--ax-weight-medium);
  text-align: right;
  overflow-wrap: anywhere;
}
</style>

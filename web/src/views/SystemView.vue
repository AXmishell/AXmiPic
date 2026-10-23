<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRouter, onBeforeRouteLeave } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { CopyDocument, DataLine, Monitor, Odometer, Refresh, SetUp } from '@element-plus/icons-vue'

import { fetchStats } from '@/api/admin'
import {
  getAdminEntry,
  rotateAdminEntry,
  updateAdminEntry,
  type AdminEntryInfo,
} from '@/api/admin_entry'
import { fetchSecurity, type SecurityInfo } from '@/api/auth'
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
  listPaymentGateways,
  previewClientIP,
  sendTestNotify,
  updateAuthSettings,
  updateModerationSettings,
  updatePaymentSettings,
  updateSettingDomain,
  type AuthConfig,
  type ClientIPInfo,
  type ClientIPSettings,
  type ImagingSettings,
  type LimitsSettings,
  type MaintenanceSettings,
  type ModerationSettings,
  type PaymentSettings,
  type PaymentSettingsInput,
  type ProcessInfo,
  type RuntimeInfo,
  type SecuritySettings,
  type SiteSettings,
  type SettingDomain,
  type UploadSettings,
} from '@/api/billing'
import { toApiError } from '@/api/client'
import type { AdminStats } from '@/api/types'
import ErrorState from '@/components/ErrorState.vue'
import PageHeader from '@/components/PageHeader.vue'
import { useAppStore } from '@/stores/app'
import { copyText } from '@/utils/clipboard'
import { formatBytes, formatDateTime, formatNumber } from '@/utils/format'
import { useTabQuery } from '@/utils/useTabQuery'

const { t } = useI18n()

type SettingsTab = 'overview' | 'notify' | 'payment' | 'moderation' | 'integration' | 'runtime'
const SETTINGS_TABS: readonly SettingsTab[] = [
  'overview',
  'notify',
  'payment',
  'moderation',
  'integration',
  'runtime',
]

const loading = ref(false)
const errorMessage = ref('')
const activeTab = useTabQuery<SettingsTab>('tab', 'overview', SETTINGS_TABS)
const router = useRouter()
const appState = useAppStore()

const stats = ref<AdminStats | null>(null)
const runtime = ref<RuntimeInfo | null>(null)
const process = ref<ProcessInfo | null>(null)
const channels = ref<{ sms: string; email: string }>({ sms: '', email: '' })
const scannerName = ref('')
const drivers = ref<{ available: string[]; active: string }>({ available: [], active: '' })

// 管理员安全入口
const adminEntry = ref<AdminEntryInfo | null>(null)
const adminEntrySecurity = ref<SecurityInfo | null>(null)
const adminEntryDialog = ref(false)
const adminEntryAction = ref<'enable' | 'disable' | 'rotate'>('enable')
const adminEntryBusy = ref(false)
const adminEntryForm = reactive({ password: '', totpCode: '' })

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
    { label: t('system.processRows.goroutines'), value: formatNumber(p.goroutines) },
    { label: t('system.processRows.heapAlloc'), value: formatBytes(p.heap_alloc_bytes) },
    { label: t('system.processRows.heapInuse'), value: formatBytes(p.heap_inuse_bytes) },
    { label: t('system.processRows.heapObjects'), value: formatNumber(p.heap_objects) },
    { label: t('system.processRows.heapSys'), value: formatBytes(p.heap_sys_bytes) },
    { label: t('system.processRows.sys'), value: formatBytes(p.sys_bytes) },
    { label: t('system.processRows.stackInuse'), value: formatBytes(p.stack_inuse_bytes) },
    { label: t('system.processRows.gcCount'), value: formatNumber(p.gc_count) },
    { label: t('system.processRows.logicalCpu'), value: t('system.processRows.cpuCores', { count: p.num_cpu }) },
    { label: t('system.processRows.uptime'), value: formatDuration(p.uptime_seconds) },
  ]
})

const runtimeRows = computed(() => {
  const info = runtime.value
  if (!info) return []
  return [
    { label: t('system.runtimeRows.siteUrl'), value: info.base_url },
    { label: t('system.runtimeRows.database'), value: info.database_driver },
    { label: t('system.runtimeRows.storageDriver'), value: info.storage_driver },
    { label: t('system.runtimeRows.processor'), value: info.processor },
    { label: t('system.runtimeRows.environment'), value: `${info.go_version} · ${info.platform}` },
    {
      label: t('system.runtimeRows.installStatus'),
      value: info.installed ? t('system.runtimeRows.installed') : t('system.runtimeRows.notInstalled'),
    },
  ]
})

const policyRows = computed(() => {
  const info = runtime.value
  if (!info) return []
  return [
    { label: t('system.policyRows.defaultQuota'), value: formatMiB(info.default_quota_mb) },
    { label: t('system.policyRows.fileLimit'), value: formatMiB(info.upload_max_mb) },
    { label: t('system.policyRows.guestQuota'), value: formatMiB(info.guest_quota_mb) },
    { label: t('system.policyRows.guestFileLimit'), value: formatMiB(info.guest_upload_max_mb) },
    { label: t('system.policyRows.sessionTtl'), value: t('system.duration.hours', { count: info.session_ttl_hours }) },
    { label: t('system.policyRows.clientIp'), value: clientIPSummary(info) },
    { label: t('system.policyRows.trustedProxies'), value: trustedProxySummary(info) },
  ]
})

/** 概览里展示当前生效的客户端 IP 解析方式。 */
function clientIPSummary(info: RuntimeInfo): string {
  const source = info.client_ip_source
  if (!source || source === 'remote') return t('system.clientIpSummary.untrusted')
  const header = source === 'custom' && info.client_ip_header ? info.client_ip_header : source
  return t('system.clientIpSummary.trusted', { header })
}

function trustedProxySummary(info: RuntimeInfo): string {
  const list = info.client_ip_trusted_proxies ?? []
  return list.length > 0 ? list.join(t('system.listSeparator')) : t('system.clientIpSummary.notConfigured')
}

/** 将秒数格式化为「Xd Xh Xm Xs」形式的运行时长。 */
function formatDuration(seconds: number): string {
  const total = Math.max(0, Math.floor(seconds))
  const d = Math.floor(total / 86400)
  const h = Math.floor((total % 86400) / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  const parts: string[] = []
  if (d > 0) parts.push(t('system.duration.days', { count: d }))
  if (h > 0 || d > 0) parts.push(t('system.duration.hours', { count: h }))
  if (m > 0 || h > 0 || d > 0) parts.push(t('system.duration.minutes', { count: m }))
  parts.push(t('system.duration.seconds', { count: s }))
  return parts.join(' ')
}

function formatMiB(mb: number): string {
  if (mb <= 0) return t('quotaMeter.unlimited')
  return formatBytes(mb * 1024 * 1024)
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
    ElMessage.warning(t('system.moderation.urlRequired'))
    return
  }
  moderationSaving.value = true
  try {
    const updated = await updateModerationSettings({ ...moderationForm })
    moderationMeta.value = updated
    fillModeration(updated)
    ElMessage.success(t('system.moderation.saved'))
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
const providerLabels = computed<Record<string, string>>(() => ({
  manual: t('system.payment.providers.manual'),
  mock: t('system.payment.providers.mock'),
  alipay: t('system.payment.providers.alipay'),
  wechat: t('system.payment.providers.wechat'),
  epay: t('system.payment.providers.epay'),
}))

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

const providerLabel = (name: string): string => providerLabels.value[name] ?? name

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
  paymentGateways.value.includes(name)
    ? providerLabel(name)
    : t('system.payment.disabledOption', { name: providerLabel(name) })

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
  set ? t('system.payment.secretSet', { label }) : t('system.payment.secretEnter', { label })

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
    ElMessage.success(t('system.payment.saved'))
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
    ElMessage.warning(t('system.notify.missingFields'))
    return
  }
  testing.value = true
  try {
    await sendTestNotify({
      channel: testChannel.value,
      to: testTo.value.trim(),
      subject: testChannel.value === 'email' ? t('system.notify.testSubject') : undefined,
      body: testBody.value.trim(),
    })
    ElMessage.success(t('system.notify.testSent'))
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
  const previous = runtime.value
  if (previous) {
    const risky: string[] = []
    if (previous.allow_registration && !allowRegistration.value) {
      risky.push(t('system.auth.disableRegistration'))
    }
    if (!previous.require_auth && requireAuth.value) risky.push(t('system.auth.requireAuth'))
    if (risky.length > 0) {
      try {
        await ElMessageBox.confirm(
          t('system.auth.confirmMessage', { actions: risky.join(t('system.listSeparator')) }),
          t('system.auth.confirmTitle'),
          {
            confirmButtonText: t('system.auth.continue'),
            cancelButtonText: t('common.cancel'),
            type: 'warning',
          },
        )
      } catch {
        applyAuth(previous)
        return
      }
    }
  }
  authSaving.value = true
  try {
    const updated = await updateAuthSettings({
      allow_registration: allowRegistration.value,
      require_auth: requireAuth.value,
      allow_guest_upload: allowGuestUpload.value,
    })
    applyAuth(updated)
    ElMessage.success(t('system.auth.saved'))
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

/** 最近一次服务端生效值的快照，用于脏状态与放弃修改。 */
const savedDomains = reactive<Record<string, unknown>>({})

function cloneValue<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

function markDomainClean(domain: SettingDomain, value: unknown): void {
  savedDomains[domain] = cloneValue(value)
}

function domainValue(domain: SettingDomain): unknown {
  switch (domain) {
    case 'upload':
      return uploadForm
    case 'processing':
      return imagingForm
    case 'security':
      return securityForm
    case 'client_ip':
      return clientIPForm
    case 'limits':
      return limitsForm
    case 'maintenance':
      return maintenanceForm
    case 'site':
      return siteForm
    default:
      return null
  }
}

function isDomainDirty(domain: SettingDomain): boolean {
  const saved = savedDomains[domain]
  if (saved === undefined) return false
  return JSON.stringify(domainValue(domain)) !== JSON.stringify(saved)
}

const domainLabels = computed<Record<string, string>>(() => ({
  upload: t('system.runtime.upload.title'),
  processing: t('system.runtime.processing.title'),
  security: t('system.runtime.security.title'),
  client_ip: t('system.runtime.clientIp.title'),
  limits: t('system.runtime.limits.title'),
  maintenance: t('system.runtime.maintenance.title'),
  site: t('system.runtime.site.title'),
}))

const DOMAIN_ORDER: SettingDomain[] = [
  'upload',
  'processing',
  'security',
  'client_ip',
  'limits',
  'maintenance',
  'site',
]

const dirtyDomains = computed(() => DOMAIN_ORDER.filter((domain) => isDomainDirty(domain)))

const savingAll = ref(false)

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
  markDomainClean('client_ip', clientIPForm)
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
  markDomainClean('upload', uploadForm)
}
function fillImaging(v: ImagingSettings): void {
  Object.assign(imagingForm, v)
  markDomainClean('processing', imagingForm)
}
function fillSecurity(v: SecuritySettings): void {
  Object.assign(securityForm, v)
  markDomainClean('security', securityForm)
}
function fillLimits(v: LimitsSettings): void {
  Object.assign(limitsForm, v)
  markDomainClean('limits', limitsForm)
}
function fillMaintenance(v: MaintenanceSettings): void {
  Object.assign(maintenanceForm, v)
  markDomainClean('maintenance', maintenanceForm)
}
function fillSite(v: SiteSettings): void {
  Object.assign(siteForm, v)
  markDomainClean('site', siteForm)
}

/** 把快照值写回对应表单（放弃修改时使用）。 */
function applyDomainValue(domain: SettingDomain, value: unknown): void {
  switch (domain) {
    case 'upload':
      fillUpload(value as UploadSettings)
      break
    case 'processing':
      fillImaging(value as ImagingSettings)
      break
    case 'security':
      fillSecurity(value as SecuritySettings)
      break
    case 'client_ip':
      fillClientIP(value as ClientIPSettings)
      break
    case 'limits':
      fillLimits(value as LimitsSettings)
      break
    case 'maintenance':
      fillMaintenance(value as MaintenanceSettings)
      break
    case 'site':
      fillSite(value as SiteSettings)
      break
  }
}

/** 跳转到插件市场。 */
function goPluginMarket(): void {
  void router.push('/admin/plugins')
}

/** 保存单个设置域；可热应用的域即时生效，limits/maintenance 重启后生效。 */
async function saveDomain<T>(
  domain: SettingDomain,
  value: unknown,
  apply: (v: T) => void,
  silent = false,
): Promise<boolean> {
  savingDomain.value = domain
  try {
    const updated = await updateSettingDomain<T>(domain, value)
    apply(updated)
    if (!silent) ElMessage.success(t('system.messages.saved'))
    return true
  } catch (error) {
    ElMessage.error(toApiError(error).message)
    return false
  } finally {
    savingDomain.value = ''
  }
}

/** 保存当前域的表单值，用于「保存全部」。 */
function persistDomain(domain: SettingDomain): Promise<boolean> {
  const value = cloneValue(domainValue(domain))
  return saveDomain<unknown>(domain, value, (updated) => applyDomainValue(domain, updated), true)
}

async function saveAllDomains(): Promise<void> {
  const pending = [...dirtyDomains.value]
  if (pending.length === 0 || savingAll.value) return
  savingAll.value = true
  const failed: string[] = []
  try {
    for (const domain of pending) {
      const ok = await persistDomain(domain)
      if (!ok) failed.push(domainLabels.value[domain] ?? domain)
    }
  } finally {
    savingAll.value = false
  }
  if (failed.length === 0) {
    ElMessage.success(t('system.messages.savedCount', { count: pending.length }))
  } else {
    ElMessage.error(t('system.messages.saveFailed', { items: failed.join(t('system.listSeparator')) }))
  }
}

function discardAllDomains(): void {
  for (const domain of DOMAIN_ORDER) {
    const saved = savedDomains[domain]
    if (saved !== undefined) applyDomainValue(domain, cloneValue(saved))
  }
  ElMessage.info(t('system.messages.discarded'))
}

function openAdminEntryDialog(action: 'enable' | 'disable' | 'rotate'): void {
  adminEntryAction.value = action
  adminEntryForm.password = ''
  adminEntryForm.totpCode = ''
  adminEntryDialog.value = true
}

const adminEntryActionLabel = computed(() => {
  switch (adminEntryAction.value) {
    case 'disable':
      return t('system.runtime.adminEntry.disable')
    case 'rotate':
      return t('system.runtime.adminEntry.rotate')
    default:
      return t('system.runtime.adminEntry.enable')
  }
})

async function confirmAdminEntryAction(): Promise<void> {
  if (!adminEntryForm.password) {
    ElMessage.warning(t('system.runtime.adminEntry.passwordRequired'))
    return
  }
  if (adminEntrySecurity.value?.totp_enabled && !/^\d{6}$/.test(adminEntryForm.totpCode.trim())) {
    ElMessage.warning(t('system.runtime.adminEntry.totpRequired'))
    return
  }
  const payload = {
    current_password: adminEntryForm.password,
    totp_code: adminEntrySecurity.value?.totp_enabled
      ? adminEntryForm.totpCode.trim()
      : undefined,
  }
  adminEntryBusy.value = true
  try {
    if (adminEntryAction.value === 'rotate') {
      adminEntry.value = await rotateAdminEntry(payload)
      ElMessage.success(t('system.runtime.adminEntry.rotated'))
    } else {
      adminEntry.value = await updateAdminEntry(adminEntryAction.value === 'enable', payload)
      ElMessage.success(
        adminEntryAction.value === 'enable'
          ? t('system.runtime.adminEntry.enabled')
          : t('system.runtime.adminEntry.disabled'),
      )
    }
    adminEntryDialog.value = false
    await appState.checkAdminEntry(true)
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    adminEntryBusy.value = false
  }
}

async function copyAdminEntryURL(): Promise<void> {
  const url = adminEntry.value?.url
  if (!url) return
  const copied = await copyText(url)
  if (copied) {
    ElMessage.success(t('system.runtime.adminEntry.copied'))
  } else {
    ElMessage.error(t('system.runtime.adminEntry.copyFailed'))
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
      limitsData,
      maintenanceData,
      siteData,
      clientIPData,
      adminEntryData,
      accountSecurityData,
    ] = await Promise.all([
      fetchStats(),
      getRuntimeInfo(),
      getProcessInfo(),
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
      getSettingDomain<LimitsSettings>('limits').catch(() => null),
      getSettingDomain<MaintenanceSettings>('maintenance').catch(() => null),
      getSettingDomain<SiteSettings>('site').catch(() => null),
      getSettingDomain<ClientIPSettings>('client_ip').catch(() => null),
      getAdminEntry().catch(() => null),
      fetchSecurity().catch(() => null),
    ])
    stats.value = statsData
    runtime.value = runtimeData
    process.value = processData
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
    if (limitsData) fillLimits(limitsData)
    if (maintenanceData) fillMaintenance(maintenanceData)
    if (siteData) fillSite(siteData)
    if (clientIPData) fillClientIP(clientIPData)
    adminEntry.value = adminEntryData
    adminEntrySecurity.value = accountSecurityData
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
  window.addEventListener('beforeunload', handleBeforeUnload)
})
onBeforeUnmount(() => {
  window.clearInterval(processTimer)
  window.removeEventListener('beforeunload', handleBeforeUnload)
})

/** 关闭标签页/刷新时提示未保存的运行设置。 */
function handleBeforeUnload(event: BeforeUnloadEvent): void {
  if (dirtyDomains.value.length === 0) return
  event.preventDefault()
  event.returnValue = ''
}

onBeforeRouteLeave(async () => {
  if (dirtyDomains.value.length === 0) return true
  try {
    await ElMessageBox.confirm(
      t('system.leave.message', {
        items: dirtyDomains.value
          .map((domain) => domainLabels.value[domain])
          .join(t('system.listSeparator')),
      }),
      t('system.leave.title'),
      {
        confirmButtonText: t('system.leave.confirm'),
        cancelButtonText: t('system.leave.cancel'),
        type: 'warning',
        confirmButtonClass: 'el-button--danger',
      },
    )
    return true
  } catch {
    return false
  }
})
</script>

<template>
  <div class="ax-page">
    <PageHeader :title="t('system.page.title')" :description="t('system.page.description')">
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="load" />

    <div v-else-if="loading && !runtime" class="ax-card" aria-busy="true">
      <div class="ax-card__body"><el-skeleton :rows="6" animated /></div>
    </div>

    <el-tabs v-else v-model="activeTab" class="settings-tabs">
      <!-- 概览 -->
      <el-tab-pane :label="t('system.tabs.overview')" name="overview">
        <section class="settings-grid overview-grid">
          <article class="ax-card">
            <header class="ax-card__head">
              <div class="card-head">
                <span class="card-head__icon"><el-icon><Monitor /></el-icon></span>
                <div class="card-head__text">
                  <h2 class="ax-card__title">{{ t('system.overview.runtime.title') }}</h2>
                  <p class="card-head__sub">{{ t('system.overview.runtime.description') }}</p>
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
                  <h2 class="ax-card__title">{{ t('system.overview.policy.title') }}</h2>
                  <p class="card-head__sub">{{ t('system.overview.policy.description') }}</p>
                </div>
              </div>
              <el-button size="small" @click="activeTab = 'runtime'">
                {{ t('system.overview.policy.configureClientIp') }}
              </el-button>
            </header>
            <div class="ax-card__body">
              <div class="policy-switch">
                <div class="policy-switch__text">
                  <span class="policy-switch__label">{{ t('system.overview.policy.registration') }}</span>
                  <span class="policy-switch__hint">{{ t('system.overview.policy.registrationHint') }}</span>
                </div>
                <el-switch v-model="allowRegistration" :loading="authSaving" @change="saveAuth" />
              </div>
              <div class="policy-switch">
                <div class="policy-switch__text">
                  <span class="policy-switch__label">{{ t('system.overview.policy.requireAuth') }}</span>
                  <span class="policy-switch__hint">{{ t('system.overview.policy.requireAuthHint') }}</span>
                </div>
                <el-switch v-model="requireAuth" :loading="authSaving" @change="saveAuth" />
              </div>
              <div class="policy-switch">
                <div class="policy-switch__text">
                  <span class="policy-switch__label">{{ t('system.overview.policy.guestUpload') }}</span>
                  <span class="policy-switch__hint">{{ t('system.overview.policy.guestUploadHint') }}</span>
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
                <h2 class="ax-card__title">{{ t('system.overview.process.title') }}</h2>
                <p class="card-head__sub">{{ t('system.overview.process.description') }}</p>
              </div>
            </div>
            <span class="process-hint">{{ t('system.overview.process.refreshHint') }}</span>
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
                <h2 class="ax-card__title">{{ t('system.overview.stats.title') }}</h2>
                <p class="card-head__sub">{{ t('system.overview.stats.description') }}</p>
              </div>
            </div>
          </header>
          <div class="ax-card__body">
            <div class="stat-grid">
              <div class="stat-tile">
                <span class="stat-tile__label">{{ t('system.overview.stats.users') }}</span>
                <span class="stat-tile__value">{{ formatNumber(stats.users) }}</span>
                <span class="stat-tile__meta">{{ t('system.overview.stats.admins', { count: formatNumber(stats.admins) }) }}</span>
              </div>
              <div class="stat-tile">
                <span class="stat-tile__label">{{ t('system.overview.stats.images') }}</span>
                <span class="stat-tile__value">{{ formatNumber(stats.images) }}</span>
                <span class="stat-tile__meta">{{ t('system.overview.stats.storedImages') }}</span>
              </div>
              <div class="stat-tile">
                <span class="stat-tile__label">{{ t('system.overview.stats.storage') }}</span>
                <span class="stat-tile__value">{{ formatBytes(stats.total_bytes) }}</span>
                <span class="stat-tile__meta">{{ t('system.overview.stats.allBackends') }}</span>
              </div>
            </div>
            <dl class="info-list">
              <div class="info-list__row">
                <dt>{{ t('system.overview.stats.storageDriver') }}</dt>
                <dd><el-tag size="small" effect="plain">{{ stats.storage_driver }}</el-tag></dd>
              </div>
              <div class="info-list__row">
                <dt>{{ t('system.overview.stats.processor') }}</dt>
                <dd><el-tag size="small" effect="plain">{{ stats.processor }}</el-tag></dd>
              </div>
              <div class="info-list__row">
                <dt>{{ t('system.overview.stats.formats') }}</dt>
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
      <el-tab-pane :label="t('system.tabs.notify')" name="notify">
        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">{{ t('system.notify.channelsTitle') }}</h2>
            <el-tag size="small" type="success" effect="plain">{{ t('system.notify.pluginTag') }}</el-tag>
          </header>
          <div class="ax-card__body">
            <el-alert
              :title="t('system.notify.migrated')"
              type="info"
              :closable="false"
              show-icon
            />
            <p class="integration__hint" style="margin-top: 12px">
              {{ t('system.notify.migratedHint') }}
            </p>
            <el-button type="primary" @click="goPluginMarket">{{ t('system.notify.openMarket') }}</el-button>
          </div>
        </article>

        <article class="ax-card settings-block">
          <header class="ax-card__head">
            <h2 class="ax-card__title">{{ t('system.notify.channelsTest') }}</h2>
          </header>
          <div class="ax-card__body integration">
            <dl class="info-list">
              <div class="info-list__row">
                <dt>{{ t('system.notify.smsChannel') }}</dt>
                <dd>{{ channels.sms || t('system.status.notConfigured') }}</dd>
              </div>
              <div class="info-list__row">
                <dt>{{ t('system.notify.emailChannel') }}</dt>
                <dd>{{ channels.email || t('system.status.notConfigured') }}</dd>
              </div>
            </dl>
            <el-divider content-position="left">{{ t('system.notify.testDivider') }}</el-divider>
            <div class="integration__form">
              <el-radio-group v-model="testChannel">
                <el-radio-button value="sms">{{ t('system.notify.sms') }}</el-radio-button>
                <el-radio-button value="email">{{ t('system.notify.email') }}</el-radio-button>
              </el-radio-group>
              <el-input
                v-model="testTo"
                :placeholder="testChannel === 'sms' ? t('system.notify.phonePlaceholder') : t('system.notify.emailPlaceholder')"
              />
              <el-input
                v-model="testBody"
                type="textarea"
                :rows="2"
                :placeholder="t('system.notify.bodyPlaceholder')"
              />
              <el-button type="primary" :loading="testing" @click="sendTest">{{ t('system.notify.sendTest') }}</el-button>
            </div>
            <p class="integration__hint">
              {{ t('system.notify.hint') }}
            </p>
          </div>
        </article>

      </el-tab-pane>

      <!-- 支付设置 -->
      <el-tab-pane :label="t('system.tabs.payment')" name="payment">
        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">{{ t('system.payment.title') }}</h2>
            <el-tag size="small" type="info" effect="plain">{{ t('system.payment.instant') }}</el-tag>
          </header>
          <div class="ax-card__body">
            <el-form label-position="top" class="smtp-form" @submit.prevent>
              <el-form-item :label="t('system.payment.defaultGateway')">
                <el-select v-model="paymentForm.default_gateway" style="max-width: 260px">
                  <el-option
                    v-for="gateway in selectableGateways"
                    :key="gateway"
                    :label="gatewayOptionLabel(gateway)"
                    :value="gateway"
                  />
                </el-select>
                <span class="smtp-form__hint">{{ t('system.payment.defaultGatewayHint') }}</span>
              </el-form-item>
            </el-form>
            <div class="smtp-actions">
              <el-button type="primary" :loading="paymentSaving" @click="savePayment">
                {{ t('system.actions.saveApply') }}
              </el-button>
              <el-button :disabled="paymentSaving" @click="resetPayment">{{ t('common.reset') }}</el-button>
            </div>
            <p class="integration__hint">
              {{ t('system.payment.securityHint') }}
            </p>
          </div>
        </article>

        <section class="settings-grid gateway-grid">
          <article class="ax-card gateway-card">
            <header class="ax-card__head">
              <h2 class="ax-card__title">{{ t('system.payment.alipay.title') }}</h2>
              <el-tag size="small" :type="paymentForm.alipay.enabled ? 'success' : 'info'" effect="plain">
                {{ paymentForm.alipay.enabled ? t('common.enabled') : t('system.status.notEnabled') }}
              </el-tag>
            </header>
            <div class="ax-card__body">
              <p class="gateway-card__hint">{{ t('system.payment.alipay.hint') }}</p>
              <el-form label-position="top" class="smtp-form" @submit.prevent>
                <div class="smtp-grid">
                  <el-form-item :label="t('system.payment.enable')">
                    <el-switch v-model="paymentForm.alipay.enabled" />
                  </el-form-item>
                  <el-form-item :label="t('system.payment.gatewayUrl')" class="smtp-grid__wide">
                    <el-input
                      v-model="paymentForm.alipay.gateway_url"
                      :placeholder="t('system.payment.gatewayUrlPlaceholder')"
                    />
                  </el-form-item>
                  <el-form-item :label="t('system.payment.appId')">
                    <el-input v-model="paymentForm.alipay.app_id" />
                  </el-form-item>
                  <el-form-item :label="t('system.payment.alipay.privateKey')">
                    <el-input
                      v-model="paymentForm.alipay.private_key"
                      type="password"
                      show-password
                      autocomplete="new-password"
                      :placeholder="secretPlaceholder(paymentMeta?.alipay.private_key_set, t('system.payment.secret.privateKey'))"
                    />
                  </el-form-item>
                  <el-form-item :label="t('system.payment.alipay.publicKey')">
                    <el-input
                      v-model="paymentForm.alipay.public_key"
                      type="password"
                      show-password
                      autocomplete="new-password"
                      :placeholder="secretPlaceholder(paymentMeta?.alipay.public_key_set, t('system.payment.secret.publicKey'))"
                    />
                  </el-form-item>
                </div>
              </el-form>
            </div>
          </article>

          <article class="ax-card gateway-card">
            <header class="ax-card__head">
              <h2 class="ax-card__title">{{ t('system.payment.wechat.title') }}</h2>
              <el-tag size="small" :type="paymentForm.wechat.enabled ? 'success' : 'info'" effect="plain">
                {{ paymentForm.wechat.enabled ? t('common.enabled') : t('system.status.notEnabled') }}
              </el-tag>
            </header>
            <div class="ax-card__body">
              <p class="gateway-card__hint">{{ t('system.payment.wechat.hint') }}</p>
              <el-form label-position="top" class="smtp-form" @submit.prevent>
                <div class="smtp-grid">
                  <el-form-item :label="t('system.payment.enable')">
                    <el-switch v-model="paymentForm.wechat.enabled" />
                  </el-form-item>
                  <el-form-item :label="t('system.payment.gatewayUrl')" class="smtp-grid__wide">
                    <el-input
                      v-model="paymentForm.wechat.gateway_url"
                      :placeholder="t('system.payment.gatewayUrlPlaceholder')"
                    />
                  </el-form-item>
                  <el-form-item :label="t('system.payment.appId')">
                    <el-input v-model="paymentForm.wechat.app_id" />
                  </el-form-item>
                  <el-form-item :label="t('system.payment.wechat.mchId')">
                    <el-input v-model="paymentForm.wechat.mch_id" />
                  </el-form-item>
                  <el-form-item :label="t('system.payment.wechat.serialNo')">
                    <el-input v-model="paymentForm.wechat.serial_no" />
                  </el-form-item>
                  <el-form-item :label="t('system.payment.wechat.privateKey')">
                    <el-input
                      v-model="paymentForm.wechat.private_key"
                      type="password"
                      show-password
                      autocomplete="new-password"
                      :placeholder="secretPlaceholder(paymentMeta?.wechat.private_key_set, t('system.payment.secret.privateKey'))"
                    />
                  </el-form-item>
                  <el-form-item :label="t('system.payment.wechat.apiV3Key')">
                    <el-input
                      v-model="paymentForm.wechat.api_v3_key"
                      type="password"
                      show-password
                      autocomplete="new-password"
                      :placeholder="secretPlaceholder(paymentMeta?.wechat.api_v3_key_set, t('system.payment.secret.key'))"
                    />
                  </el-form-item>
                  <el-form-item :label="t('system.payment.wechat.platformPublicKey')">
                    <el-input
                      v-model="paymentForm.wechat.platform_public_key"
                      type="password"
                      show-password
                      autocomplete="new-password"
                      :placeholder="secretPlaceholder(paymentMeta?.wechat.platform_public_key_set, t('system.payment.secret.publicKey'))"
                    />
                  </el-form-item>
                  <el-form-item :label="t('system.payment.wechat.platformSerialNo')">
                    <el-input
                      v-model="paymentForm.wechat.platform_serial_no"
                      :placeholder="t('system.payment.wechat.platformSerialNoPlaceholder')"
                    />
                  </el-form-item>
                </div>
              </el-form>
            </div>
          </article>

          <article class="ax-card gateway-card">
            <header class="ax-card__head">
              <h2 class="ax-card__title">{{ t('system.payment.epay.title') }}</h2>
              <el-tag size="small" :type="paymentForm.epay.enabled ? 'success' : 'info'" effect="plain">
                {{ paymentForm.epay.enabled ? t('common.enabled') : t('system.status.notEnabled') }}
              </el-tag>
            </header>
            <div class="ax-card__body">
              <p class="gateway-card__hint">{{ t('system.payment.epay.hint') }}</p>
              <el-form label-position="top" class="smtp-form" @submit.prevent>
                <div class="smtp-grid">
                  <el-form-item :label="t('system.payment.enable')">
                    <el-switch v-model="paymentForm.epay.enabled" />
                  </el-form-item>
                  <el-form-item :label="t('system.payment.epay.payType')">
                    <el-select v-model="paymentForm.epay.pay_type" style="width: 100%">
                      <el-option :label="t('system.payment.epay.payAlipay')" value="alipay" />
                      <el-option :label="t('system.payment.epay.payWechat')" value="wxpay" />
                      <el-option :label="t('system.payment.epay.payQq')" value="qqpay" />
                    </el-select>
                  </el-form-item>
                  <el-form-item :label="t('system.payment.epay.pid')">
                    <el-input v-model="paymentForm.epay.pid" />
                  </el-form-item>
                  <el-form-item :label="t('system.payment.epay.key')">
                    <el-input
                      v-model="paymentForm.epay.key"
                      type="password"
                      show-password
                      autocomplete="new-password"
                      :placeholder="secretPlaceholder(paymentMeta?.epay.key_set, t('system.payment.secret.key'))"
                    />
                  </el-form-item>
                  <el-form-item :label="t('system.payment.epay.siteUrl')" class="smtp-grid__wide">
                    <el-input v-model="paymentForm.epay.gateway_url" placeholder="https://pay.example.com" />
                  </el-form-item>
                  <el-form-item :label="t('system.payment.epay.apiUrl')">
                    <el-input
                      v-model="paymentForm.epay.api_url"
                      :placeholder="t('system.payment.epay.apiUrlPlaceholder')"
                    />
                  </el-form-item>
                  <el-form-item :label="t('system.payment.epay.submitUrl')">
                    <el-input
                      v-model="paymentForm.epay.submit_url"
                      :placeholder="t('system.payment.epay.submitUrlPlaceholder')"
                    />
                  </el-form-item>
                </div>
              </el-form>
            </div>
          </article>
        </section>
      </el-tab-pane>

      <!-- AI 审查 -->
      <el-tab-pane :label="t('system.tabs.moderation')" name="moderation">
        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">{{ t('system.moderation.title') }}</h2>
            <el-tag v-if="moderationMeta?.enabled" size="small" type="success" effect="plain">
              {{ t('common.enabled') }}
            </el-tag>
            <el-tag v-else size="small" type="info" effect="plain">{{ t('system.status.notEnabled') }}</el-tag>
          </header>
          <div class="ax-card__body">
            <el-form :model="moderationForm" label-position="top" class="smtp-form" @submit.prevent>
              <div class="smtp-grid">
                <el-form-item :label="t('system.moderation.enable')">
                  <el-switch v-model="moderationForm.enabled" />
                </el-form-item>
                <el-form-item :label="t('system.moderation.baseUrl')" class="smtp-grid__wide">
                  <el-input v-model="moderationForm.base_url" placeholder="https://api.openai.com/v1" />
                </el-form-item>
                <el-form-item :label="t('system.moderation.model')">
                  <el-input v-model="moderationForm.model" placeholder="gpt-4o-mini" />
                </el-form-item>
                <el-form-item :label="t('system.moderation.apiKey')">
                  <el-input
                    v-model="moderationForm.api_key"
                    type="password"
                    show-password
                    autocomplete="new-password"
                    :placeholder="moderationMeta?.api_key_set ? t('system.moderation.apiKeySet') : t('system.moderation.apiKeyPlaceholder')"
                  />
                </el-form-item>
                <el-form-item :label="t('system.moderation.timeout')">
                  <el-input-number v-model="moderationForm.timeout_sec" :min="1" :max="300" controls-position="right" />
                </el-form-item>
                <el-form-item :label="t('system.moderation.maxImage')">
                  <el-input-number v-model="moderationForm.max_image_mb" :min="1" :max="100" controls-position="right" />
                </el-form-item>
                <el-form-item :label="t('system.moderation.prompt')" class="smtp-grid__wide">
                  <el-input
                    v-model="moderationForm.prompt"
                    type="textarea"
                    :rows="4"
                    :placeholder="t('system.moderation.promptPlaceholder')"
                  />
                </el-form-item>
              </div>
              <p class="smtp-form__hint">
                {{ t('system.moderation.hint') }}
              </p>
              <div class="smtp-actions">
                <el-button type="primary" :loading="moderationSaving" @click="saveModeration">
                  {{ t('system.actions.saveApply') }}
                </el-button>
                <el-button :disabled="moderationSaving || !moderationMeta" @click="resetModeration">
                  {{ t('common.reset') }}
                </el-button>
              </div>
            </el-form>
          </div>
        </article>
      </el-tab-pane>

      <!-- 系统集成 -->
      <el-tab-pane :label="t('system.tabs.integration')" name="integration">
        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">{{ t('system.integration.title') }}</h2>
          </header>
          <div class="ax-card__body">
            <dl class="info-list">
              <div class="info-list__row">
                <dt>{{ t('system.integration.scanner') }}</dt>
                <dd>{{ scannerName || t('system.status.notConfigured') }}</dd>
              </div>
              <div class="info-list__row">
                <dt>{{ t('system.integration.driver') }}</dt>
                <dd>
                  {{ drivers.active || '—' }}<span class="ax-muted">{{ t('system.integration.available', { list: drivers.available.join(t('system.listSeparator')) || '—' }) }}</span>
                </dd>
              </div>
            </dl>
          </div>
        </article>
      </el-tab-pane>
      <!-- 运行设置 -->
      <el-tab-pane name="runtime">
        <template #label>
          <span class="settings-tab-label">
            {{ t('system.tabs.runtime') }}
            <span
              v-if="dirtyDomains.length > 0"
              class="settings-tab-dot"
              :aria-label="t('system.runtime.unsavedBadge')"
            />
          </span>
        </template>
        <div v-if="dirtyDomains.length > 0" class="settings-savebar" role="status">
          <div class="settings-savebar__text">
            <strong>{{ t('system.runtime.unsavedCount', { count: dirtyDomains.length }) }}</strong>
            <span>{{ dirtyDomains.map((domain) => domainLabels[domain]).join(t('system.listSeparator')) }}</span>
          </div>
          <div class="settings-savebar__actions">
            <el-button :disabled="savingAll || Boolean(savingDomain)" @click="discardAllDomains">
              {{ t('system.actions.discard') }}
            </el-button>
            <el-button
              type="primary"
              :loading="savingAll"
              :disabled="Boolean(savingDomain)"
              @click="saveAllDomains"
            >
              {{ t('system.actions.saveAll') }}
            </el-button>
          </div>
        </div>

        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">{{ t('system.runtime.upload.title') }}</h2>
            <el-tag size="small" type="success" effect="plain">{{ t('system.runtime.instant') }}</el-tag>
          </header>
          <div class="ax-card__body">
            <el-form label-position="top" class="smtp-form" @submit.prevent>
              <div class="smtp-grid">
                <el-form-item :label="t('system.runtime.upload.maxSize')">
                  <el-input-number v-model="uploadForm.max_size_mb" :min="1" :max="10240" controls-position="right" />
                </el-form-item>
                <el-form-item :label="t('system.runtime.upload.mimeTypes')" class="smtp-grid__wide">
                  <el-select v-model="uploadForm.allowed_mime_types" multiple style="width: 100%">
                    <el-option v-for="m in mimeOptions" :key="m" :label="m" :value="m" />
                  </el-select>
                </el-form-item>
              </div>
              <div class="smtp-actions">
                <el-button
                  type="primary"
                  :loading="savingDomain === 'upload'"
                  :disabled="!isDomainDirty('upload')"
                  :title="isDomainDirty('upload') ? t('system.actions.saveApply') : t('system.actions.noChanges')"
                  @click="saveDomain('upload', { ...uploadForm }, fillUpload)"
                >
                  {{ t('system.actions.saveApply') }}
                </el-button>
              </div>
            </el-form>
          </div>
        </article>

        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">{{ t('system.runtime.processing.title') }}</h2>
            <el-tag size="small" type="success" effect="plain">{{ t('system.runtime.instant') }}</el-tag>
          </header>
          <div class="ax-card__body">
            <el-form label-position="top" class="smtp-form" @submit.prevent>
              <div class="smtp-grid">
                <el-form-item :label="t('system.runtime.processing.enable')">
                  <el-switch v-model="imagingForm.enabled" />
                </el-form-item>
                <el-form-item :label="t('system.runtime.processing.maxWidth')">
                  <el-input-number v-model="imagingForm.max_width" :min="1" :max="20000" controls-position="right" />
                </el-form-item>
                <el-form-item :label="t('system.runtime.processing.maxHeight')">
                  <el-input-number v-model="imagingForm.max_height" :min="1" :max="20000" controls-position="right" />
                </el-form-item>
                <el-form-item :label="t('system.runtime.processing.quality')">
                  <el-input-number v-model="imagingForm.default_quality" :min="1" :max="100" controls-position="right" />
                </el-form-item>
                <el-form-item :label="t('system.runtime.processing.formats')" class="smtp-grid__wide">
                  <el-select v-model="imagingForm.allowed_formats" multiple style="width: 100%">
                    <el-option v-for="f in formatOptions" :key="f" :label="f" :value="f" />
                  </el-select>
                </el-form-item>
                <el-form-item :label="t('system.runtime.processing.allowEnlarge')">
                  <el-switch v-model="imagingForm.allow_enlarge" />
                </el-form-item>
                <el-form-item :label="t('system.runtime.processing.allowEffects')">
                  <el-switch v-model="imagingForm.allow_effects" />
                </el-form-item>
                <el-form-item :label="t('system.runtime.processing.allowWatermark')">
                  <el-switch v-model="imagingForm.allow_watermark" />
                </el-form-item>
                <el-form-item :label="t('system.runtime.processing.watermarkText')" class="smtp-grid__wide">
                  <el-input
                    v-model="imagingForm.watermark_text"
                    :placeholder="t('system.runtime.processing.watermarkPlaceholder')"
                  />
                </el-form-item>
              </div>
              <p class="smtp-form__hint">{{ t('system.runtime.processing.hint') }}</p>
              <div class="smtp-actions">
                <el-button
                  type="primary"
                  :loading="savingDomain === 'processing'"
                  :disabled="!isDomainDirty('processing')"
                  :title="isDomainDirty('processing') ? t('system.actions.saveApply') : t('system.actions.noChanges')"
                  @click="saveDomain('processing', { ...imagingForm }, fillImaging)"
                >
                  {{ t('system.actions.saveApply') }}
                </el-button>
              </div>
            </el-form>
          </div>
        </article>

        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">{{ t('system.runtime.security.title') }}</h2>
            <el-tag size="small" type="success" effect="plain">{{ t('system.runtime.instant') }}</el-tag>
          </header>
          <div class="ax-card__body">
            <el-form label-position="top" class="smtp-form" @submit.prevent>
              <div class="smtp-grid">
                <el-form-item :label="t('system.runtime.security.scanner')">
                  <el-select v-model="securityForm.scanner" style="width: 100%">
                    <el-option :label="t('system.runtime.security.scannerOff')" value="none" />
                    <el-option :label="t('system.runtime.security.scannerBuiltin')" value="builtin" />
                  </el-select>
                </el-form-item>
                <el-form-item :label="t('system.runtime.security.cloud')">
                  <el-select v-model="securityForm.cloud_processor" style="width: 100%">
                    <el-option :label="t('system.runtime.security.cloudLocal')" value="local" />
                  </el-select>
                </el-form-item>
              </div>
              <div class="smtp-actions">
                <el-button
                  type="primary"
                  :loading="savingDomain === 'security'"
                  :disabled="!isDomainDirty('security')"
                  :title="isDomainDirty('security') ? t('system.actions.saveApply') : t('system.actions.noChanges')"
                  @click="saveDomain('security', { ...securityForm }, fillSecurity)"
                >
                  {{ t('system.actions.saveApply') }}
                </el-button>
              </div>
            </el-form>
          </div>
        </article>

        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">{{ t('system.runtime.adminEntry.title') }}</h2>
            <el-tag
              size="small"
              :type="adminEntry?.enabled ? 'success' : 'info'"
              effect="plain"
            >
              {{ adminEntry?.enabled ? t('system.status.on') : t('system.status.off') }}
            </el-tag>
          </header>
          <div class="ax-card__body">
            <p class="security-card__desc">
              {{ t('system.runtime.adminEntry.description') }}
            </p>
            <dl class="info-list">
              <div class="info-list__row">
                <dt>{{ t('system.runtime.adminEntry.status') }}</dt>
                <dd>{{ adminEntry?.enabled ? t('system.status.on') : t('system.status.off') }}</dd>
              </div>
              <div v-if="adminEntry?.enabled" class="info-list__row">
                <dt>{{ t('system.runtime.adminEntry.url') }}</dt>
                <dd class="admin-entry-url">
                  <span class="ax-mono">{{ adminEntry.url }}</span>
                  <el-button
                    size="small"
                    :icon="CopyDocument"
                    :title="t('system.runtime.adminEntry.copyTitle')"
                    @click="copyAdminEntryURL"
                  >
                    {{ t('common.copy') }}
                  </el-button>
                </dd>
              </div>
              <div v-if="adminEntry?.updated_at" class="info-list__row">
                <dt>{{ t('system.runtime.adminEntry.updatedAt') }}</dt>
                <dd>{{ formatDateTime(adminEntry.updated_at) }}</dd>
              </div>
            </dl>
            <div class="security-card__actions">
              <el-button
                v-if="!adminEntry?.enabled"
                type="primary"
                @click="openAdminEntryDialog('enable')"
              >
                {{ t('system.runtime.adminEntry.enable') }}
              </el-button>
              <template v-else>
                <el-button @click="openAdminEntryDialog('rotate')">
                  {{ t('system.runtime.adminEntry.rotate') }}
                </el-button>
                <el-button type="danger" plain @click="openAdminEntryDialog('disable')">
                  {{ t('system.runtime.adminEntry.disable') }}
                </el-button>
              </template>
            </div>
          </div>
        </article>

        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">{{ t('system.runtime.clientIp.title') }}</h2>
            <el-tag size="small" type="success" effect="plain">{{ t('system.runtime.instant') }}</el-tag>
          </header>
          <div class="ax-card__body">
            <el-form label-position="top" class="smtp-form" @submit.prevent>
              <div class="smtp-grid">
                <el-form-item :label="t('system.runtime.clientIp.source')">
                  <el-select v-model="clientIPForm.source" style="width: 100%">
                    <el-option :label="t('system.runtime.clientIp.sourceRemote')" value="remote" />
                    <el-option label="X-Forwarded-For" value="x-forwarded-for" />
                    <el-option label="X-Real-IP" value="x-real-ip" />
                    <el-option label="CF-Connecting-IP" value="cf-connecting-ip" />
                    <el-option label="True-Client-IP" value="true-client-ip" />
                    <el-option label="X-Client-IP" value="x-client-ip" />
                    <el-option label="Forwarded (RFC 7239)" value="forwarded" />
                    <el-option :label="t('system.runtime.clientIp.sourceCustom')" value="custom" />
                  </el-select>
                </el-form-item>
                <el-form-item v-if="clientIPForm.source === 'custom'" :label="t('system.runtime.clientIp.headerName')">
                  <el-input
                    v-model="clientIPForm.header"
                    :placeholder="t('system.runtime.clientIp.headerPlaceholder')"
                  />
                </el-form-item>
                <el-form-item :label="t('system.runtime.clientIp.xffDepth')">
                  <el-input-number v-model="clientIPForm.xff_depth" :min="0" controls-position="right" />
                </el-form-item>
                <el-form-item :label="t('system.runtime.clientIp.trustedProxies')" class="smtp-grid__wide">
                  <el-select
                    v-model="clientIPForm.trusted_proxies"
                    multiple
                    filterable
                    allow-create
                    default-first-option
                    :placeholder="t('system.runtime.clientIp.trustedProxiesPlaceholder')"
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
                {{ t('system.runtime.clientIp.hint') }}
              </p>
              <div class="smtp-actions">
                <el-button
                  type="primary"
                  :loading="savingDomain === 'client_ip'"
                  :disabled="!isDomainDirty('client_ip')"
                  :title="isDomainDirty('client_ip') ? t('system.actions.saveApply') : t('system.actions.noChanges')"
                  @click="saveDomain('client_ip', { ...clientIPForm }, fillClientIP)"
                >
                  {{ t('system.actions.saveApply') }}
                </el-button>
                <el-button :loading="clientIPChecking" @click="checkClientIP">
                  {{ t('system.runtime.clientIp.check') }}
                </el-button>
              </div>
            </el-form>

            <dl v-if="clientIPInfo" class="info-list">
              <div class="info-list__row">
                <dt>{{ t('system.runtime.clientIp.resolvedIp') }}</dt>
                <dd>{{ clientIPInfo.resolved || '—' }}</dd>
              </div>
              <div class="info-list__row">
                <dt>{{ t('system.runtime.clientIp.peer') }}</dt>
                <dd>{{ clientIPInfo.peer || '—' }}</dd>
              </div>
              <div class="info-list__row">
                <dt>{{ t('system.runtime.clientIp.sourceTrusted') }}</dt>
                <dd>{{ clientIPInfo.source }} · {{ clientIPInfo.trusted_peer ? t('common.yes') : t('common.no') }}</dd>
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

            <el-divider content-position="left">{{ t('system.runtime.clientIp.simulateDivider') }}</el-divider>
            <el-form label-position="top" class="smtp-form" @submit.prevent>
              <div class="smtp-grid">
                <el-form-item :label="t('system.runtime.clientIp.remoteAddr')">
                  <el-input v-model="simulateRemote" :placeholder="t('system.runtime.clientIp.remotePlaceholder')" />
                </el-form-item>
                <el-form-item :label="t('system.runtime.clientIp.headers')" class="smtp-grid__wide">
                  <el-input
                    v-model="simulateHeaders"
                    type="textarea"
                    :rows="2"
                    :placeholder="t('system.runtime.clientIp.headersPlaceholder')"
                  />
                </el-form-item>
              </div>
              <div class="smtp-actions">
                <el-button :loading="simulating" @click="simulateClientIP">
                  {{ t('system.runtime.clientIp.simulate') }}
                </el-button>
              </div>
            </el-form>
            <dl v-if="simulateResult" class="info-list">
              <div class="info-list__row">
                <dt>{{ t('system.runtime.clientIp.resolvedIp') }}</dt>
                <dd>{{ simulateResult.resolved || '—' }}</dd>
              </div>
              <div class="info-list__row">
                <dt>{{ t('system.runtime.clientIp.peer') }}</dt>
                <dd>{{ simulateResult.peer || '—' }}</dd>
              </div>
              <div class="info-list__row">
                <dt>{{ t('system.runtime.clientIp.sourceTrusted') }}</dt>
                <dd>{{ simulateResult.source }} · {{ simulateResult.trusted_peer ? t('common.yes') : t('common.no') }}</dd>
              </div>
            </dl>
          </div>
        </article>

        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">{{ t('system.runtime.limits.title') }}</h2>
            <el-tag size="small" type="info" effect="plain">{{ t('system.runtime.restart') }}</el-tag>
          </header>
          <div class="ax-card__body">
            <el-form label-position="top" class="smtp-form" @submit.prevent>
              <div class="smtp-grid">
                <el-form-item :label="t('system.runtime.limits.uploadPerMinute')">
                  <el-input-number v-model="limitsForm.upload_per_minute" :min="0" controls-position="right" />
                </el-form-item>
                <el-form-item :label="t('system.runtime.limits.uploadBurst')">
                  <el-input-number v-model="limitsForm.upload_burst" :min="0" controls-position="right" />
                </el-form-item>
                <el-form-item :label="t('system.runtime.limits.guestPerMinute')">
                  <el-input-number v-model="limitsForm.guest_per_minute" :min="0" controls-position="right" />
                </el-form-item>
                <el-form-item :label="t('system.runtime.limits.guestBurst')">
                  <el-input-number v-model="limitsForm.guest_burst" :min="0" controls-position="right" />
                </el-form-item>
                <el-form-item :label="t('system.runtime.limits.imagePerMinute')">
                  <el-input-number v-model="limitsForm.image_per_minute" :min="0" controls-position="right" />
                </el-form-item>
                <el-form-item :label="t('system.runtime.limits.imageBurst')">
                  <el-input-number v-model="limitsForm.image_burst" :min="0" controls-position="right" />
                </el-form-item>
                <el-form-item :label="t('system.runtime.limits.sharePerMinute')">
                  <el-input-number v-model="limitsForm.share_per_minute" :min="0" controls-position="right" />
                </el-form-item>
                <el-form-item :label="t('system.runtime.limits.shareBurst')">
                  <el-input-number v-model="limitsForm.share_burst" :min="0" controls-position="right" />
                </el-form-item>
              </div>
              <div class="smtp-actions">
                <el-button
                  type="primary"
                  :loading="savingDomain === 'limits'"
                  :disabled="!isDomainDirty('limits')"
                  :title="isDomainDirty('limits') ? t('system.actions.saveApply') : t('system.actions.noChanges')"
                  @click="saveDomain('limits', { ...limitsForm }, fillLimits)"
                >
                  {{ t('common.save') }}
                </el-button>
              </div>
            </el-form>
          </div>
        </article>

        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">{{ t('system.runtime.maintenance.title') }}</h2>
            <el-tag size="small" type="info" effect="plain">{{ t('system.runtime.restart') }}</el-tag>
          </header>
          <div class="ax-card__body">
            <el-form label-position="top" class="smtp-form" @submit.prevent>
              <div class="smtp-grid">
                <el-form-item :label="t('system.runtime.maintenance.orphan')">
                  <el-switch v-model="maintenanceForm.orphan_cleanup" />
                </el-form-item>
                <el-form-item :label="t('system.runtime.maintenance.grace')">
                  <el-input-number v-model="maintenanceForm.orphan_grace_hours" :min="0" controls-position="right" />
                </el-form-item>
                <el-form-item :label="t('system.runtime.maintenance.interval')">
                  <el-input-number v-model="maintenanceForm.orphan_interval_hours" :min="1" controls-position="right" />
                </el-form-item>
              </div>
              <div class="smtp-actions">
                <el-button
                  type="primary"
                  :loading="savingDomain === 'maintenance'"
                  :disabled="!isDomainDirty('maintenance')"
                  :title="isDomainDirty('maintenance') ? t('system.actions.saveApply') : t('system.actions.noChanges')"
                  @click="saveDomain('maintenance', { ...maintenanceForm }, fillMaintenance)"
                >
                  {{ t('common.save') }}
                </el-button>
              </div>
            </el-form>
          </div>
        </article>

        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">{{ t('system.runtime.site.title') }}</h2>
          </header>
          <div class="ax-card__body">
            <el-form label-position="top" class="smtp-form" @submit.prevent>
              <div class="smtp-grid">
                <el-form-item :label="t('system.runtime.site.name')">
                  <el-input v-model="siteForm.name" />
                </el-form-item>
                <el-form-item :label="t('system.runtime.site.description')" class="smtp-grid__wide">
                  <el-input v-model="siteForm.description" type="textarea" :rows="2" />
                </el-form-item>
              </div>
              <div class="smtp-actions">
                <el-button
                  type="primary"
                  :loading="savingDomain === 'site'"
                  :disabled="!isDomainDirty('site')"
                  :title="isDomainDirty('site') ? t('system.actions.saveApply') : t('system.actions.noChanges')"
                  @click="saveDomain('site', { ...siteForm }, fillSite)"
                >
                  {{ t('common.save') }}
                </el-button>
              </div>
            </el-form>
          </div>
        </article>
      </el-tab-pane>
    </el-tabs>

    <!-- 管理员安全入口敏感操作确认 -->
    <el-dialog
      v-model="adminEntryDialog"
      :title="adminEntryActionLabel"
      width="min(420px, 92vw)"
      append-to-body
      :close-on-click-modal="false"
    >
      <p class="security-card__desc">
        {{ adminEntryAction === 'rotate'
          ? t('system.runtime.adminEntry.dialog.rotate')
          : adminEntryAction === 'disable'
            ? t('system.runtime.adminEntry.dialog.disable')
            : t('system.runtime.adminEntry.dialog.enable') }}
      </p>
      <div class="email-form">
        <el-input
          v-model="adminEntryForm.password"
          type="password"
          show-password
          :placeholder="t('system.runtime.adminEntry.dialog.passwordPlaceholder')"
          autocomplete="current-password"
        />
        <el-input
          v-if="adminEntrySecurity?.totp_enabled"
          v-model="adminEntryForm.totpCode"
          maxlength="6"
          inputmode="numeric"
          :placeholder="t('system.runtime.adminEntry.dialog.totpPlaceholder')"
          autocomplete="one-time-code"
        />
      </div>
      <template #footer>
        <el-button @click="adminEntryDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button
          type="primary"
          :loading="adminEntryBusy"
          @click="confirmAdminEntryAction"
        >
          {{ adminEntryAction === 'rotate'
            ? t('system.runtime.adminEntry.dialog.confirmRotate')
            : adminEntryAction === 'disable'
              ? t('system.runtime.adminEntry.dialog.confirmDisable')
              : t('system.runtime.adminEntry.dialog.confirmEnable') }}
        </el-button>
      </template>
    </el-dialog>
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

.settings-savebar {
  position: sticky;
  top: 0;
  z-index: 6;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--ax-space-3);
  margin-bottom: var(--ax-space-4);
  padding: var(--ax-space-2) var(--ax-space-4);
  background: var(--ax-elevated);
  border: 1px solid var(--ax-accent-ring);
  border-radius: var(--ax-radius-md);
  box-shadow: var(--ax-shadow-md);
}

.settings-savebar__text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.settings-savebar__text strong {
  color: var(--ax-text);
  font-size: var(--ax-text-sm);
  font-weight: var(--ax-weight-medium);
}

.settings-savebar__text span {
  color: var(--ax-text-3);
  font-size: var(--ax-text-xs);
  overflow-wrap: anywhere;
}

.settings-savebar__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--ax-space-2);
}

.settings-tab-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.settings-tab-dot {
  width: 6px;
  height: 6px;
  background: var(--ax-accent-bright);
  border-radius: var(--ax-radius-full);
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

.admin-entry-url {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--ax-space-2);
  flex-wrap: wrap;
}
</style>

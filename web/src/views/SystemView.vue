<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { DataLine, Monitor, Odometer, Refresh, SetUp } from '@element-plus/icons-vue'

import { fetchStats } from '@/api/admin'
import {
  getAuthSettings,
  getImagingDrivers,
  getNotifyChannels,
  getPaymentSettings,
  getProcessInfo,
  getRuntimeInfo,
  getSecurityInfo,
  getSMTPConfig,
  listPaymentGateways,
  sendTestNotify,
  updateAuthSettings,
  updatePaymentSettings,
  updateSMTPConfig,
  type AuthConfig,
  type PaymentSettings,
  type PaymentSettingsInput,
  type ProcessInfo,
  type RuntimeInfo,
  type SMTPConfig,
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
    { label: '信任代理头', value: info.trust_proxy ? '是' : '否' },
  ]
})

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

async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    const [statsData, runtimeData, processData, smtpData, ch, sec, drv, paymentData, gatewayList, authData] = await Promise.all([
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

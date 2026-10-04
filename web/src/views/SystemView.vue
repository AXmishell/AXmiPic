<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'

import { fetchStats } from '@/api/admin'
import {
  getImagingDrivers,
  getNotifyChannels,
  getProcessInfo,
  getRuntimeInfo,
  getSecurityInfo,
  getSMTPConfig,
  sendTestNotify,
  updateSMTPConfig,
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
    { label: '开放注册', value: info.allow_registration ? '允许' : '禁止' },
    { label: '上传需要登录', value: info.require_auth ? '是' : '否' },
    { label: '允许访客上传', value: info.allow_guest_upload ? '允许' : '禁止' },
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

async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    const [statsData, runtimeData, processData, smtpData, ch, sec, drv] = await Promise.all([
      fetchStats(),
      getRuntimeInfo(),
      getProcessInfo(),
      getSMTPConfig(),
      getNotifyChannels().catch(() => ({ sms: '', email: '' })),
      getSecurityInfo().catch(() => ({ scanner: '' })),
      getImagingDrivers().catch(() => ({ available: [], active: '' })),
    ])
    stats.value = statsData
    runtime.value = runtimeData
    process.value = processData
    smtpMeta.value = smtpData
    fillSMTP(smtpData)
    channels.value = ch ?? { sms: '', email: '' }
    scannerName.value = sec?.scanner ?? ''
    drivers.value = drv ?? { available: [], active: '' }
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
    <PageHeader title="系统设置" description="查看实例运行环境、配置通知渠道与系统集成。">
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
        <section class="settings-grid">
          <article class="ax-card">
            <header class="ax-card__head">
              <h2 class="ax-card__title">运行环境</h2>
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
              <h2 class="ax-card__title">策略概览</h2>
            </header>
            <div class="ax-card__body">
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
            <h2 class="ax-card__title">进程概览</h2>
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
            <h2 class="ax-card__title">系统信息</h2>
          </header>
          <div class="ax-card__body">
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
                <dt>用户数</dt>
                <dd>{{ formatNumber(stats.users) }}（管理员 {{ formatNumber(stats.admins) }}）</dd>
              </div>
              <div class="info-list__row">
                <dt>图片数</dt>
                <dd>{{ formatNumber(stats.images) }}</dd>
              </div>
              <div class="info-list__row">
                <dt>存储总用量</dt>
                <dd>{{ formatBytes(stats.total_bytes) }}</dd>
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
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: var(--ax-space-3) var(--ax-space-4);
  background: var(--ax-tint-weak);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-md);
}

.process-item__label {
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.process-item__value {
  color: var(--ax-text);
  font-size: var(--ax-text-md);
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
  gap: var(--ax-space-3);
  margin: 0;
  width: 100%;
}

.info-list__row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--ax-space-4);
  min-width: 0;
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
  text-align: right;
  overflow-wrap: anywhere;
}
</style>

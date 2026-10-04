<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'

import { fetchStats } from '@/api/admin'
import {
  getImagingDrivers,
  getNotifyChannels,
  getRuntimeInfo,
  getSecurityInfo,
  sendTestNotify,
  type RuntimeInfo,
} from '@/api/billing'
import { toApiError } from '@/api/client'
import type { AdminStats } from '@/api/types'
import ErrorState from '@/components/ErrorState.vue'
import PageHeader from '@/components/PageHeader.vue'
import { formatBytes, formatNumber } from '@/utils/format'

const loading = ref(false)
const errorMessage = ref('')

const stats = ref<AdminStats | null>(null)
const runtime = ref<RuntimeInfo | null>(null)
const channels = ref<{ sms: string; email: string }>({ sms: '', email: '' })
const scannerName = ref('')
const drivers = ref<{ available: string[]; active: string }>({ available: [], active: '' })

const testChannel = ref<'sms' | 'email'>('sms')
const testTo = ref('')
const testBody = ref('')
const testing = ref(false)

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

function formatMiB(mb: number): string {
  if (mb <= 0) return '不限'
  return formatBytes(mb * 1024 * 1024)
}

async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    const [statsData, runtimeData, ch, sec, drv] = await Promise.all([
      fetchStats(),
      getRuntimeInfo(),
      getNotifyChannels().catch(() => ({ sms: '', email: '' })),
      getSecurityInfo().catch(() => ({ scanner: '' })),
      getImagingDrivers().catch(() => ({ available: [], active: '' })),
    ])
    stats.value = statsData
    runtime.value = runtimeData
    channels.value = ch ?? { sms: '', email: '' }
    scannerName.value = sec?.scanner ?? ''
    drivers.value = drv ?? { available: [], active: '' }
  } catch (error) {
    errorMessage.value = toApiError(error).message
  } finally {
    loading.value = false
  }
}

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

onMounted(load)
</script>

<template>
  <div class="ax-page">
    <PageHeader title="系统设置" description="查看实例运行环境、系统集成与运行状态。">
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="load" />

    <div v-else-if="loading && !runtime" class="ax-card" aria-busy="true">
      <div class="ax-card__body"><el-skeleton :rows="6" animated /></div>
    </div>

    <template v-else>
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

      <article class="ax-card settings-block">
        <header class="ax-card__head">
          <h2 class="ax-card__title">系统集成</h2>
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
            <div class="info-list__row">
              <dt>内容扫描器</dt>
              <dd>{{ scannerName || '未配置' }}</dd>
            </div>
            <div class="info-list__row">
              <dt>处理驱动</dt>
              <dd>{{ drivers.active || '—' }}<span class="ax-muted">（可用：{{ drivers.available.join('、') || '—' }}）</span></dd>
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
            未配置真实服务商时，通知会回退到日志渠道并记录到服务端日志。
          </p>
        </div>
      </article>
    </template>
  </div>
</template>

<style scoped>
.settings-grid {
  display: grid;
  gap: var(--ax-space-4);
  grid-template-columns: repeat(auto-fit, minmax(min(320px, 100%), 1fr));
  align-items: start;
}

.settings-block {
  margin-top: var(--ax-space-4);
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

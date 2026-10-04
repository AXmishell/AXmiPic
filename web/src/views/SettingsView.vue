<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Key, Refresh, SwitchButton } from '@element-plus/icons-vue'

import { fetchStats } from '@/api/admin'
import { toApiError } from '@/api/client'
import { getImagingDrivers, getNotifyChannels, getSecurityInfo, sendTestNotify } from '@/api/billing'
import type { AdminStats } from '@/api/types'
import ErrorState from '@/components/ErrorState.vue'
import PageHeader from '@/components/PageHeader.vue'
import QuotaMeter from '@/components/QuotaMeter.vue'
import UserAvatar from '@/components/UserAvatar.vue'
import { useAuthStore } from '@/stores/auth'
import { useThemeStore, type ThemeMode } from '@/stores/theme'
import { formatBytes, formatDateTime, formatNumber } from '@/utils/format'

const auth = useAuthStore()
const theme = useThemeStore()
const router = useRouter()

const loading = ref(false)
const errorMessage = ref('')
const stats = ref<AdminStats | null>(null)

const me = computed(() => auth.user)
const isAdmin = computed(() => auth.isAdmin)

const themeOptions: { value: ThemeMode; label: string }[] = [
  { value: 'dark', label: '深色' },
  { value: 'light', label: '浅色' },
]

function setTheme(mode: ThemeMode): void {
  theme.setMode(mode)
}

async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    await auth.refreshUser()
    if (auth.isAdmin) {
      stats.value = await fetchStats()
      await loadIntegrations()
    }
  } catch (error) {
    errorMessage.value = toApiError(error).message
  } finally {
    loading.value = false
  }
}

// 系统集成：通知渠道与安全扫描器。
const channels = ref<{ sms: string; email: string }>({ sms: '', email: '' })
const scannerName = ref('')
const drivers = ref<{ available: string[]; active: string }>({ available: [], active: '' })
const testChannel = ref<'sms' | 'email'>('sms')
const testTo = ref('')
const testBody = ref('')
const testing = ref(false)

async function loadIntegrations(): Promise<void> {
  try {
    const [ch, sec, drv] = await Promise.all([
      getNotifyChannels(),
      getSecurityInfo(),
      getImagingDrivers(),
    ])
    channels.value = ch ?? { sms: '', email: '' }
    scannerName.value = sec?.scanner ?? ''
    drivers.value = drv ?? { available: [], active: '' }
  } catch {
    // 集成信息加载失败不影响其他设置项。
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

async function handleLogout(): Promise<void> {
  try {
    await ElMessageBox.confirm('退出后需要重新登录才能继续使用控制台。', '确定退出登录吗？', {
      confirmButtonText: '退出登录',
      cancelButtonText: '取消',
      type: 'warning',
    })
  } catch {
    return
  }
  auth.logout()
  ElMessage.success('已退出登录')
  await router.replace('/login')
}

onMounted(load)
</script>

<template>
  <div class="ax-page">
    <PageHeader title="账号设置" description="查看账户信息、配额与登录状态。">
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="load" />

    <div v-else-if="loading && !me" class="ax-card" aria-busy="true">
      <div class="ax-card__body"><el-skeleton :rows="6" animated /></div>
    </div>

    <template v-else-if="me">
      <section class="settings-grid">
        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">账户信息</h2>
          </header>
          <div class="ax-card__body settings-profile">
            <div class="settings-identity">
              <UserAvatar :name="me.username" size="lg" />
              <div class="settings-identity__meta">
                <p class="settings-identity__name">{{ me.username }}</p>
                <el-tag size="small" :type="isAdmin ? 'primary' : 'info'" effect="plain">
                  {{ isAdmin ? '管理员' : '用户' }}
                </el-tag>
              </div>
            </div>
            <dl class="info-list">
              <div class="info-list__row">
                <dt>用户 ID</dt>
                <dd class="ax-mono">{{ me.id }}</dd>
              </div>
              <div class="info-list__row">
                <dt>角色</dt>
                <dd>{{ isAdmin ? '管理员' : '用户' }}</dd>
              </div>
              <div class="info-list__row">
                <dt>注册时间</dt>
                <dd>{{ formatDateTime(me.created_at) }}</dd>
              </div>
              <div class="info-list__row">
                <dt>状态</dt>
                <dd>{{ me.disabled ? '已禁用' : '正常' }}</dd>
              </div>
            </dl>
          </div>
        </article>

        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">存储配额</h2>
          </header>
          <div class="ax-card__body settings-quota">
            <QuotaMeter :used="me.used_bytes" :quota="me.quota_bytes" />
            <el-button size="small" :icon="Key" @click="router.push(auth.isAdmin ? '/admin/settings' : '/user/tokens')">
              管理访问令牌
            </el-button>
          </div>
        </article>
      </section>

      <article v-if="isAdmin && stats" class="ax-card settings-block">
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
          <h2 class="ax-card__title">外观主题</h2>
        </header>
        <div class="ax-card__body settings-theme">
          <div class="settings-theme__text">
            <strong>界面主题</strong>
            <span>选择深色或浅色外观，设置会保存在本机浏览器。</span>
          </div>
          <el-radio-group :model-value="theme.mode" @change="setTheme($event as ThemeMode)">
            <el-radio-button v-for="option in themeOptions" :key="option.value" :value="option.value">
              {{ option.label }}
            </el-radio-button>
          </el-radio-group>
        </div>
      </article>

      <article class="ax-card settings-block">
        <header class="ax-card__head">
          <h2 class="ax-card__title">登录状态</h2>
        </header>
        <div class="ax-card__body settings-session">
          <div class="settings-session__text">
            <strong>退出登录</strong>
            <span>退出当前账号并清除本地会话，需要重新登录。</span>
          </div>
          <el-button type="danger" plain :icon="SwitchButton" @click="handleLogout">
            退出登录
          </el-button>
        </div>
      </article>

      <article v-if="isAdmin" class="ax-card settings-block">
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

.settings-profile {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-5);
}

.settings-identity {
  display: flex;
  align-items: center;
  gap: var(--ax-space-3);
}

.settings-identity__meta {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--ax-space-1);
  min-width: 0;
}

.settings-identity__name {
  overflow: hidden;
  max-width: 100%;
  color: var(--ax-text);
  font-size: var(--ax-text-md);
  font-weight: var(--ax-weight-semibold);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.settings-quota {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-5);
  align-items: flex-start;
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

.settings-session {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--ax-space-4);
}

.settings-theme {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--ax-space-4);
}

.settings-theme__text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 200px;
}

.settings-theme__text strong {
  color: var(--ax-text-2);
  font-size: var(--ax-text-sm);
  font-weight: var(--ax-weight-medium);
}

.settings-theme__text span {
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.settings-session__text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 200px;
}

.settings-session__text strong {
  color: var(--ax-text-2);
  font-size: var(--ax-text-sm);
  font-weight: var(--ax-weight-medium);
}

.settings-session__text span {
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}
</style>

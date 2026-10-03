<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Key, Refresh, SwitchButton } from '@element-plus/icons-vue'

import { fetchStats } from '@/api/admin'
import { toApiError } from '@/api/client'
import type { AdminStats } from '@/api/types'
import ErrorState from '@/components/ErrorState.vue'
import PageHeader from '@/components/PageHeader.vue'
import QuotaMeter from '@/components/QuotaMeter.vue'
import UserAvatar from '@/components/UserAvatar.vue'
import { useAuthStore } from '@/stores/auth'
import { formatBytes, formatDateTime, formatNumber } from '@/utils/format'

const auth = useAuthStore()
const router = useRouter()

const loading = ref(false)
const errorMessage = ref('')
const stats = ref<AdminStats | null>(null)

const me = computed(() => auth.user)
const isAdmin = computed(() => auth.isAdmin)

async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    await auth.refreshUser()
    if (auth.isAdmin) {
      stats.value = await fetchStats()
    }
  } catch (error) {
    errorMessage.value = toApiError(error).message
  } finally {
    loading.value = false
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
            <el-button size="small" :icon="Key" @click="router.push('/tokens')">
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
              <dd>{{ formatNumber(stats.users) }}</dd>
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

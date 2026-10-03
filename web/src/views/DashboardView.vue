<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Coin, DataLine, Picture, Refresh, Timer, User } from '@element-plus/icons-vue'

import { fetchStats } from '@/api/admin'
import { ApiError } from '@/api/client'
import type { AdminStats } from '@/api/types'
import ErrorState from '@/components/ErrorState.vue'
import PageHeader from '@/components/PageHeader.vue'
import QuotaMeter from '@/components/QuotaMeter.vue'
import StatCard from '@/components/StatCard.vue'
import UserAvatar from '@/components/UserAvatar.vue'
import { useAuthStore } from '@/stores/auth'
import { formatBytes, formatDateTime, formatNumber } from '@/utils/format'

const auth = useAuthStore()
const router = useRouter()

const loading = ref(true)
const errorMessage = ref('')
const stats = ref<AdminStats | null>(null)

const me = computed(() => auth.user)
const isAdmin = computed(() => auth.isAdmin)
const formatCount = computed(() => stats.value?.formats?.length ?? 0)
const formatsLabel = computed(() => (formatCount.value > 0 ? `${formatCount.value} 种` : '—'))
const remainingLabel = computed(() => {
  if (!me.value || !me.value.quota_bytes || me.value.quota_bytes <= 0) return '不限'
  return formatBytes(Math.max(0, me.value.quota_bytes - me.value.used_bytes))
})

async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    if (!auth.user) {
      await auth.refreshUser()
    }
    if (auth.isAdmin) {
      stats.value = await fetchStats()
    }
  } catch (error) {
    errorMessage.value = error instanceof ApiError ? error.message : '加载失败，请稍后重试'
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="ax-page">
    <PageHeader
      title="仪表盘"
      :description="isAdmin ? '系统运行概览与账户状态' : '你的账户信息与存储用量'"
    >
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="load" />

    <div v-else-if="loading" class="dash-skeleton">
      <div class="ax-grid">
        <div v-for="index in 3" :key="index" class="ax-card">
          <div class="ax-card__body"><el-skeleton :rows="2" animated /></div>
        </div>
      </div>
      <div class="dash-cols">
        <div v-for="index in 2" :key="index" class="ax-card">
          <div class="ax-card__body"><el-skeleton :rows="4" animated /></div>
        </div>
      </div>
    </div>

    <template v-else-if="me">
      <section class="ax-grid dash-stats" aria-label="概览统计">
        <template v-if="isAdmin && stats">
          <StatCard label="用户数" :value="formatNumber(stats.users)" :icon="User" hint="已注册账户" />
          <StatCard
            label="图片数"
            :value="formatNumber(stats.images)"
            :icon="Picture"
            tone="success"
            hint="全部账号累计"
          />
          <StatCard
            label="存储用量"
            :value="formatBytes(stats.total_bytes)"
            :icon="Coin"
            tone="warning"
            hint="全站已占用空间"
          />
          <StatCard label="存储驱动" :value="stats.storage_driver" hint="当前存储后端" />
          <StatCard label="处理器" :value="stats.processor" hint="图片处理引擎" />
          <StatCard label="支持格式" :value="formatsLabel" hint="可输出的图片格式">
            <div v-if="stats.formats && stats.formats.length" class="ax-cluster">
              <el-tag
                v-for="format in stats.formats"
                :key="format"
                size="small"
                type="info"
                effect="plain"
              >
                {{ format.toUpperCase() }}
              </el-tag>
            </div>
          </StatCard>
        </template>
        <template v-else>
          <StatCard
            label="已用空间"
            :value="formatBytes(me.used_bytes)"
            :icon="Coin"
            hint="当前账号占用"
          />
          <StatCard
            label="剩余配额"
            :value="remainingLabel"
            :icon="DataLine"
            tone="success"
            hint="配额为 0 表示不限"
          />
          <StatCard
            label="注册时间"
            :value="formatDateTime(me.created_at)"
            :icon="Timer"
            tone="warning"
          />
        </template>
      </section>

      <section class="dash-cols">
        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">我的账户</h2>
          </header>
          <div class="ax-card__body account">
            <div class="account__identity">
              <UserAvatar :name="me.username" size="lg" />
              <div class="account__meta">
                <p class="account__name">{{ me.username }}</p>
                <el-tag size="small" :type="isAdmin ? 'primary' : 'info'" effect="plain">
                  {{ isAdmin ? '管理员' : '用户' }}
                </el-tag>
              </div>
            </div>
            <QuotaMeter :used="me.used_bytes" :quota="me.quota_bytes" />
            <dl class="info-list">
              <div class="info-list__row">
                <dt>用户 ID</dt>
                <dd class="ax-mono">{{ me.id }}</dd>
              </div>
              <div class="info-list__row">
                <dt>注册时间</dt>
                <dd>{{ formatDateTime(me.created_at) }}</dd>
              </div>
            </dl>
          </div>
        </article>

        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">快捷操作</h2>
          </header>
          <div class="ax-card__body stack-actions">
            <div class="quick-action">
              <div class="quick-action__text">
                <strong>管理图片</strong>
                <span>查看、复制链接或删除已上传的图片</span>
              </div>
              <el-button size="small" @click="router.push('/images')">前往</el-button>
            </div>
            <div class="quick-action">
              <div class="quick-action__text">
                <strong>访问令牌</strong>
                <span>创建或吊销用于 API 上传的访问令牌</span>
              </div>
              <el-button size="small" @click="router.push('/tokens')">前往</el-button>
            </div>
            <div v-if="isAdmin" class="quick-action">
              <div class="quick-action__text">
                <strong>用户管理</strong>
                <span>调整角色、启用或禁用账号</span>
              </div>
              <el-button size="small" @click="router.push('/users')">前往</el-button>
            </div>
          </div>
        </article>
      </section>
    </template>
  </div>
</template>

<style scoped>
.dash-skeleton {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-4);
}

.dash-stats {
  margin-bottom: var(--ax-space-4);
}

.dash-cols {
  display: grid;
  gap: var(--ax-space-4);
  grid-template-columns: repeat(auto-fit, minmax(min(320px, 100%), 1fr));
  align-items: start;
}

.account {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-5);
}

.account__identity {
  display: flex;
  align-items: center;
  gap: var(--ax-space-3);
}

.account__meta {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--ax-space-1);
  min-width: 0;
}

.account__name {
  overflow: hidden;
  max-width: 100%;
  color: var(--ax-text);
  font-size: var(--ax-text-md);
  font-weight: var(--ax-weight-semibold);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.info-list {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-3);
  margin: 0;
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

.stack-actions {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-2);
}

.quick-action {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--ax-space-4);
  padding: var(--ax-space-3) var(--ax-space-4);
  background: rgba(255, 255, 255, 0.02);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-md);
}

.quick-action__text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.quick-action__text strong {
  color: var(--ax-text-2);
  font-size: var(--ax-text-sm);
  font-weight: var(--ax-weight-medium);
}

.quick-action__text span {
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
  overflow-wrap: anywhere;
}

@media (max-width: 768px) {
  .quick-action {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>

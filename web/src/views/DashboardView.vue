<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  Coin,
  DataLine,
  Picture,
  Refresh,
  Timer,
  User,
  UserFilled,
} from '@element-plus/icons-vue'

import { fetchStats } from '@/api/admin'
import { ApiError } from '@/api/client'
import { listAnnouncements } from '@/api/site'
import type { AdminStats, Announcement } from '@/api/types'
import ErrorState from '@/components/ErrorState.vue'
import PageHeader from '@/components/PageHeader.vue'
import QuotaMeter from '@/components/QuotaMeter.vue'
import StatCard from '@/components/StatCard.vue'
import TrendChart from '@/components/TrendChart.vue'
import UserAvatar from '@/components/UserAvatar.vue'
import { useAuthStore } from '@/stores/auth'
import { formatBytes, formatDateTime, formatNumber } from '@/utils/format'

const auth = useAuthStore()

const loading = ref(true)
const errorMessage = ref('')
const stats = ref<AdminStats | null>(null)
const announcements = ref<Announcement[]>([])

const me = computed(() => auth.user)
const isAdmin = computed(() => auth.isAdmin)
const trend = computed(() => stats.value?.trend ?? [])
const formatCount = computed(() => stats.value?.formats?.length ?? 0)
const remainingLabel = computed(() => {
  if (!me.value || !me.value.quota_bytes || me.value.quota_bytes <= 0) return '不限'
  return formatBytes(Math.max(0, me.value.quota_bytes - me.value.used_bytes))
})

interface QuickAction {
  title: string
  description: string
  to: string
}

const quickActions = computed<QuickAction[]>(() =>
  isAdmin.value
    ? [
        { title: '用户管理', description: '查看用户、调整角色组或禁用账号', to: '/admin/users' },
        { title: '站点内容', description: '发布公告、处理举报与独立页面', to: '/admin/site' },
        { title: '计费管理', description: '维护套餐、优惠券与工单', to: '/admin/billing' },
      ]
    : [
        { title: '我的图片', description: '上传、整理与复制图片链接', to: '/user/images' },
        { title: '图片处理', description: '实时预览缩放、滤镜与水印', to: '/user/processing' },
        { title: '访问令牌', description: '创建用于 API 上传的令牌', to: '/user/tokens' },
      ],
)

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
    try {
      announcements.value = await listAnnouncements()
    } catch {
      // 公告加载失败不影响仪表盘其余内容。
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

    <div v-else-if="loading" class="dash-skeleton" aria-busy="true">
      <div class="dash-kpis">
        <div v-for="index in isAdmin ? 4 : 3" :key="index" class="ax-card dash-skeleton__stat">
          <div class="ax-card__body"><el-skeleton :rows="2" animated /></div>
        </div>
      </div>
      <div class="dash-overview">
        <div class="ax-card"><div class="ax-card__body"><el-skeleton :rows="5" animated /></div></div>
        <div class="ax-card"><div class="ax-card__body"><el-skeleton :rows="5" animated /></div></div>
      </div>
      <div class="dash-cols">
        <div class="ax-card"><div class="ax-card__body"><el-skeleton :rows="4" animated /></div></div>
        <div class="ax-card"><div class="ax-card__body"><el-skeleton :rows="4" animated /></div></div>
      </div>
    </div>

    <template v-else-if="me">
      <section v-if="announcements.length > 0" class="dash-announcements" aria-label="站内公告">
        <el-alert
          v-for="item in announcements"
          :key="item.id"
          :type="item.level === 'info' ? 'info' : item.level"
          :closable="false"
          show-icon
          class="dash-announcement"
        >
          <template #title>
            <strong>{{ item.title }}</strong>
            <el-tag
              v-if="item.pinned"
              size="small"
              type="danger"
              effect="plain"
              class="dash-announcement__pin"
            >
              置顶
            </el-tag>
          </template>
          <p v-if="item.content" class="dash-announcement__content">{{ item.content }}</p>
        </el-alert>
      </section>

      <section class="dash-kpis" aria-label="概览统计">
        <template v-if="isAdmin && stats">
          <StatCard
            label="管理员"
            :value="formatNumber(stats.admins)"
            :icon="User"
            hint="特权账号"
          />
          <StatCard
            label="普通用户"
            :value="formatNumber(stats.customers)"
            :icon="UserFilled"
            hint="已注册账号"
          />
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
            hint="账号创建时间"
          />
        </template>
      </section>

      <section v-if="isAdmin && stats" class="dash-overview" aria-label="运行概览">
        <article class="ax-card dash-trend">
          <header class="ax-card__head">
            <div class="dash-card-title">
              <h2 class="ax-card__title">上传趋势</h2>
              <p class="dash-card-title__hint">近 7 日每日新增图片与占用空间</p>
            </div>
            <el-tag size="small" type="info" effect="plain">7 天</el-tag>
          </header>
          <div class="ax-card__body">
            <TrendChart :points="trend" />
          </div>
        </article>

        <article class="ax-card dash-runtime">
          <header class="ax-card__head">
            <div class="dash-card-title">
              <h2 class="ax-card__title">运行环境</h2>
              <p class="dash-card-title__hint">当前实例的存储与处理能力</p>
            </div>
          </header>
          <div class="ax-card__body">
            <dl class="runtime-list">
              <div class="runtime-list__row">
                <dt>存储驱动</dt>
                <dd class="ax-mono">{{ stats.storage_driver }}</dd>
              </div>
              <div class="runtime-list__row">
                <dt>图片处理器</dt>
                <dd class="ax-mono">{{ stats.processor || '—' }}</dd>
              </div>
              <div class="runtime-list__row">
                <dt>输出格式</dt>
                <dd>
                  <span v-if="formatCount === 0" class="ax-muted">—</span>
                  <span v-else class="ax-cluster runtime-list__formats">
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
                </dd>
              </div>
              <div class="runtime-list__row">
                <dt>图片总数</dt>
                <dd>{{ formatNumber(stats.images) }} 张</dd>
              </div>
            </dl>
            <p class="runtime-list__hint">
              存储后端与处理器可在
              <router-link to="/admin/system">系统设置</router-link>
              中调整。
            </p>
          </div>
        </article>
      </section>

      <section class="dash-cols">
        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">我的账户</h2>
            <el-tag size="small" :type="isAdmin ? 'primary' : 'info'" effect="plain">
              {{ isAdmin ? '管理员' : '用户' }}
            </el-tag>
          </header>
          <div class="ax-card__body account">
            <div class="account__identity">
              <UserAvatar :name="me.username" size="lg" />
              <div class="account__meta">
                <p class="account__name">{{ me.username }}</p>
                <span class="account__role">
                  {{ isAdmin ? '拥有实例管理权限' : '普通账户' }}
                </span>
              </div>
            </div>
            <QuotaMeter :used="me.used_bytes" :quota="me.quota_bytes" />
            <dl class="info-list">
              <div class="info-list__row">
                <dt>用户 ID</dt>
                <dd class="ax-mono account__id" :title="me.id">{{ me.id }}</dd>
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
            <router-link
              v-for="action in quickActions"
              :key="action.to"
              :to="action.to"
              class="quick-action"
            >
              <span class="quick-action__text">
                <strong>{{ action.title }}</strong>
                <span>{{ action.description }}</span>
              </span>
              <span class="quick-action__arrow" aria-hidden="true">→</span>
            </router-link>
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

.dash-skeleton__stat {
  min-height: 120px;
}

.dash-announcements {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-2);
  margin-bottom: var(--ax-space-4);
}

.dash-announcement__pin {
  margin-left: var(--ax-space-2);
}

.dash-announcement__content {
  margin: var(--ax-space-1) 0 0;
  white-space: pre-wrap;
}

.dash-kpis {
  display: grid;
  gap: var(--ax-space-4);
  margin-bottom: var(--ax-space-4);
  grid-template-columns: repeat(auto-fit, minmax(min(210px, 100%), 1fr));
}

.dash-overview {
  display: grid;
  gap: var(--ax-space-4);
  align-items: stretch;
  margin-bottom: var(--ax-space-4);
  grid-template-columns: minmax(0, 1.55fr) minmax(0, 1fr);
}

.dash-trend,
.dash-runtime {
  min-width: 0;
}

.dash-card-title {
  min-width: 0;
}

.dash-card-title__hint {
  margin-top: 2px;
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.runtime-list {
  display: flex;
  flex-direction: column;
  gap: 0;
  margin: 0;
}

.runtime-list__row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--ax-space-4);
  min-height: 40px;
  padding: var(--ax-space-2) 0;
  border-bottom: 1px solid var(--ax-border-subtle);
}

.runtime-list__row:first-child {
  padding-top: 0;
}

.runtime-list__row:last-child {
  border-bottom: none;
}

.runtime-list__row dt {
  flex: none;
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
}

.runtime-list__row dd {
  margin: 0;
  min-width: 0;
  color: var(--ax-text-2);
  font-size: var(--ax-text-sm);
  text-align: right;
}

.runtime-list__formats {
  justify-content: flex-end;
  gap: var(--ax-space-1);
}

.runtime-list__hint {
  margin-top: var(--ax-space-3);
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.runtime-list__hint a {
  color: var(--ax-accent-hover);
}

.dash-cols {
  display: grid;
  gap: var(--ax-space-4);
  align-items: start;
  grid-template-columns: repeat(auto-fit, minmax(min(340px, 100%), 1fr));
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
  gap: 2px;
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

.account__role {
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
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

.account__id {
  overflow: hidden;
  max-width: 60%;
  text-overflow: ellipsis;
  white-space: nowrap;
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
  color: inherit;
  background: var(--ax-tint-weak);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-md);
  transition:
    background-color var(--ax-duration-fast) var(--ax-ease),
    border-color var(--ax-duration-fast) var(--ax-ease);
}

.quick-action:hover {
  background: var(--ax-tint);
  border-color: var(--ax-border);
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

.quick-action__arrow {
  flex: none;
  color: var(--ax-text-4);
  font-size: var(--ax-text-md);
}

.quick-action:hover .quick-action__arrow {
  color: var(--ax-accent-hover);
}

@media (max-width: 960px) {
  .dash-overview {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 640px) {
  .info-list__row {
    align-items: flex-start;
    flex-direction: column;
    gap: var(--ax-space-1);
  }

  .info-list__row dd {
    max-width: 100%;
    text-align: left;
  }
}
</style>

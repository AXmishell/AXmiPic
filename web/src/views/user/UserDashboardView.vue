<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Coin, DataLine, Picture, Refresh, Timer } from '@element-plus/icons-vue'

import { listAnnouncements } from '@/api/site'
import type { Announcement } from '@/api/types'
import ErrorState from '@/components/ErrorState.vue'
import PageHeader from '@/components/PageHeader.vue'
import QuotaMeter from '@/components/QuotaMeter.vue'
import StatCard from '@/components/StatCard.vue'
import UserAvatar from '@/components/UserAvatar.vue'
import { useAuthStore } from '@/stores/auth'
import { formatBytes, formatDateTime } from '@/utils/format'

const auth = useAuthStore()
const router = useRouter()

const loading = ref(false)
const errorMessage = ref('')
const announcements = ref<Announcement[]>([])

const me = computed(() => auth.user)
const remainingLabel = computed(() => {
  if (!me.value || !me.value.quota_bytes || me.value.quota_bytes <= 0) return '不限'
  return formatBytes(Math.max(0, me.value.quota_bytes - me.value.used_bytes))
})

async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    if (!auth.user) await auth.refreshUser()
    try {
      announcements.value = await listAnnouncements()
    } catch {
      // 公告加载失败不影响用户中心其余内容。
    }
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '加载失败，请稍后重试'
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="ax-page">
    <PageHeader title="用户中心" description="你的账户信息、存储用量与快捷入口。">
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
        <el-button type="primary" :icon="Picture" @click="router.push('/user/images')">上传图片</el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="load" />

    <div v-else-if="loading && !me" class="ax-grid">
      <div v-for="i in 3" :key="i" class="ax-card"><div class="ax-card__body"><el-skeleton :rows="2" animated /></div></div>
    </div>

    <template v-else-if="me">
      <section v-if="announcements.length > 0" class="dash-announcements" aria-label="站内公告">
        <el-alert
          v-for="item in announcements"
          :key="item.id"
          :type="item.level === 'info' ? 'info' : item.level"
          :closable="false"
          show-icon
        >
          <template #title>
            <strong>{{ item.title }}</strong>
            <el-tag v-if="item.pinned" size="small" type="danger" effect="plain" class="dash-announcements__pin">置顶</el-tag>
          </template>
          <p v-if="item.content" class="dash-announcements__content">{{ item.content }}</p>
        </el-alert>
      </section>

      <section class="ax-grid dash-stats" aria-label="概览统计">
        <StatCard label="已用空间" :value="formatBytes(me.used_bytes)" :icon="Coin" hint="当前账号占用" />
        <StatCard label="剩余配额" :value="remainingLabel" :icon="DataLine" tone="success" hint="配额为 0 表示不限" />
        <StatCard label="注册时间" :value="formatDateTime(me.created_at)" :icon="Timer" tone="warning" />
      </section>

      <section class="dash-cols">
        <article class="ax-card">
          <header class="ax-card__head"><h2 class="ax-card__title">我的账户</h2></header>
          <div class="ax-card__body account">
            <div class="account__identity">
              <UserAvatar :name="me.username" size="lg" />
              <div class="account__meta">
                <p class="account__name">{{ me.username }}</p>
                <el-tag size="small" type="info" effect="plain">用户</el-tag>
              </div>
            </div>
            <QuotaMeter :used="me.used_bytes" :quota="me.quota_bytes" />
            <dl class="info-list">
              <div class="info-list__row"><dt>用户 ID</dt><dd class="ax-mono">{{ me.id }}</dd></div>
              <div class="info-list__row"><dt>注册时间</dt><dd>{{ formatDateTime(me.created_at) }}</dd></div>
            </dl>
          </div>
        </article>

        <article class="ax-card">
          <header class="ax-card__head"><h2 class="ax-card__title">快捷操作</h2></header>
          <div class="ax-card__body stack-actions">
            <div class="quick-action">
              <div class="quick-action__text"><strong>我的图片</strong><span>上传、整理与复制图片链接</span></div>
              <el-button size="small" @click="router.push('/user/images')">前往</el-button>
            </div>
            <div class="quick-action">
              <div class="quick-action__text"><strong>图片处理</strong><span>实时缩放、滤镜与水印预览</span></div>
              <el-button size="small" @click="router.push('/user/processing')">前往</el-button>
            </div>
            <div class="quick-action">
              <div class="quick-action__text"><strong>访问令牌</strong><span>创建用于 API 上传的令牌</span></div>
              <el-button size="small" @click="router.push('/user/tokens')">前往</el-button>
            </div>
            <div class="quick-action">
              <div class="quick-action__text"><strong>我的分享</strong><span>管理图片与相册的分享链接</span></div>
              <el-button size="small" @click="router.push('/user/shares')">前往</el-button>
            </div>
          </div>
        </article>
      </section>
    </template>
  </div>
</template>

<style scoped>
.dash-announcements {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-2);
  margin-bottom: var(--ax-space-4);
}

.dash-announcements__pin {
  margin-left: var(--ax-space-2);
}

.dash-announcements__content {
  margin: var(--ax-space-1) 0 0;
  white-space: pre-wrap;
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
  background: var(--ax-tint-weak);
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
}

@media (max-width: 768px) {
  .quick-action {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>

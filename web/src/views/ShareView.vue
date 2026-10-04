<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Lock, Picture } from '@element-plus/icons-vue'

import { toApiError } from '@/api/client'
import { shareAccess, shareInfo } from '@/api/shares'
import type { Share, SharePayload } from '@/api/types'
import { copyText } from '@/utils/clipboard'
import { formatDateTime } from '@/utils/format'

const route = useRoute()
const token = String(route.params.token ?? '')

const info = ref<Share | null>(null)
const payload = ref<SharePayload | null>(null)
const password = ref('')
const loading = ref(false)
const errorMessage = ref('')
const needsPassword = ref(false)

async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    const share = await shareInfo(token)
    info.value = share
    if (share.has_password) {
      needsPassword.value = true
    } else {
      await access()
    }
  } catch (error) {
    errorMessage.value = toApiError(error).message
  } finally {
    loading.value = false
  }
}

async function access(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    payload.value = await shareAccess(token, password.value)
    needsPassword.value = false
  } catch (error) {
    errorMessage.value = toApiError(error).message
  } finally {
    loading.value = false
  }
}

async function copy(url: string): Promise<void> {
  const ok = await copyText(url)
  if (ok) ElMessage.success('已复制链接')
  else ElMessage.error('复制失败')
}

onMounted(load)
</script>

<template>
  <main class="share-page">
    <section class="share-card">
      <header class="share-card__head">
        <span class="share-card__badge">
          <el-icon :size="16"><Picture /></el-icon>
          AXmiPic 分享
        </span>
      </header>

      <div v-if="loading && !payload && !needsPassword" class="share-state">正在加载…</div>

      <div v-else-if="errorMessage" class="share-state share-state--error">
        <p>{{ errorMessage }}</p>
      </div>

      <form v-else-if="needsPassword" class="share-password" @submit.prevent="access">
        <el-icon :size="28" class="share-password__icon"><Lock /></el-icon>
        <p class="share-password__title">该分享需要密码</p>
        <el-input
          v-model="password"
          type="password"
          show-password
          placeholder="请输入访问密码"
          class="share-password__input"
        />
        <el-button type="primary" native-type="submit" :loading="loading">访问</el-button>
      </form>

      <template v-else-if="payload">
        <!-- 单图分享 -->
        <figure v-if="payload.image" class="share-figure">
          <img :src="payload.image.url" :alt="payload.image.original_name || payload.image.key" />
          <figcaption>
            <strong>{{ payload.image.original_name || payload.image.filename || payload.image.key }}</strong>
            <span>{{ formatDateTime(payload.image.created_at) }}</span>
            <el-button size="small" @click="copy(payload.image?.url ?? '')">复制图片链接</el-button>
          </figcaption>
        </figure>

        <!-- 相册分享 -->
        <template v-else-if="payload.album">
          <div class="share-album__head">
            <h1>{{ payload.album.name }}</h1>
            <p v-if="payload.album.intro" class="share-album__intro">{{ payload.album.intro }}</p>
            <p class="share-album__meta">
              作者：{{ payload.album.owner_username || '匿名' }} ·
              共 {{ payload.images?.length ?? 0 }} 张图片
            </p>
          </div>
          <div class="share-grid">
            <figure v-for="image in payload.images" :key="image.id" class="share-grid__item">
              <img :src="image.url" loading="lazy" :alt="image.original_name || image.key" />
            </figure>
          </div>
        </template>
      </template>
    </section>
  </main>
</template>

<style scoped>
.share-page {
  min-height: 100dvh;
  padding: clamp(var(--ax-space-4), 5vw, var(--ax-space-7));
  background: var(--ax-bg);
}

.share-card {
  max-width: 1100px;
  margin: 0 auto;
  padding: clamp(var(--ax-space-4), 4vw, var(--ax-space-6));
  background: var(--ax-panel);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-lg);
  box-shadow: var(--ax-shadow-md);
}

.share-card__head {
  margin-bottom: var(--ax-space-4);
}

.share-card__badge {
  display: inline-flex;
  align-items: center;
  gap: var(--ax-space-2);
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
}

.share-state {
  padding: var(--ax-space-7) 0;
  color: var(--ax-text-3);
  text-align: center;
}

.share-state--error {
  color: var(--ax-danger);
}

.share-password {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--ax-space-3);
  max-width: 320px;
  margin: var(--ax-space-6) auto;
}

.share-password__icon {
  color: var(--ax-text-4);
}

.share-password__title {
  margin: 0;
  color: var(--ax-text);
  font-weight: var(--ax-weight-medium);
}

.share-password__input {
  width: 100%;
}

.share-figure {
  margin: 0;
  text-align: center;
}

.share-figure img {
  max-width: 100%;
  max-height: 78vh;
  border-radius: var(--ax-radius-md);
}

.share-figure figcaption {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: center;
  gap: var(--ax-space-3);
  margin-top: var(--ax-space-3);
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
}

.share-album__head {
  margin-bottom: var(--ax-space-4);
}

.share-album__head h1 {
  margin: 0 0 var(--ax-space-2);
  color: var(--ax-text);
  font-size: var(--ax-text-xl);
}

.share-album__intro,
.share-album__meta {
  margin: 0;
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
}

.share-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(200px, 100%), 1fr));
  gap: var(--ax-space-3);
}

.share-grid__item {
  margin: 0;
  overflow: hidden;
  background: var(--ax-tint-weak);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-md);
}

.share-grid__item img {
  display: block;
  width: 100%;
  height: 100%;
  aspect-ratio: 1;
  object-fit: cover;
}
</style>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  ArrowRight,
  CopyDocument,
  Grid,
  Moon,
  Picture,
  Sunny,
  Upload,
  UploadFilled,
  User,
} from '@element-plus/icons-vue'

import { listAnnouncements } from '@/api/site'
import { listPlaza, uploadImage } from '@/api/images'
import { toApiError } from '@/api/client'
import type { Announcement, ImageItem } from '@/api/types'
import CopyField from '@/components/CopyField.vue'
import { useAuthStore } from '@/stores/auth'
import { useThemeStore } from '@/stores/theme'
import { formatDateTime } from '@/utils/format'

const router = useRouter()
const auth = useAuthStore()
const theme = useThemeStore()

const uploadRef = ref<{ clearFiles: () => void } | null>(null)
const uploading = ref(false)
const dragActive = ref(false)
const uploaded = ref<ImageItem | null>(null)
const announcements = ref<Announcement[]>([])
const plaza = ref<ImageItem[]>([])
const ACCEPT = 'image/jpeg,image/png,image/gif,image/webp'

// ---- 首页图片广场：瀑布流 + 无感自动加载 ----
const PLAZA_PAGE_SIZE = 20
const plazaPage = ref(1)
const plazaTotal = ref(0)
const plazaLoading = ref(false)
const plazaHasMore = ref(true)
const plazaSentinel = ref<HTMLElement | null>(null)
let plazaObserver: IntersectionObserver | null = null

async function loadPlaza(): Promise<void> {
  if (plazaLoading.value || !plazaHasMore.value) return
  plazaLoading.value = true
  try {
    const data = await listPlaza({ page: plazaPage.value, pageSize: PLAZA_PAGE_SIZE, order: 'newest' })
    const items = data.items ?? []
    plaza.value = plazaPage.value === 1 ? items : [...plaza.value, ...items]
    plazaTotal.value = data.total ?? plaza.value.length
    // 返回不足一页或已达总数即认为没有更多。
    plazaHasMore.value = items.length === PLAZA_PAGE_SIZE && plaza.value.length < plazaTotal.value
    plazaPage.value += 1
  } catch {
    // 广场为可选内容，失败时停止加载，不影响首页其他部分。
    plazaHasMore.value = false
  } finally {
    plazaLoading.value = false
  }
}

// 监听哨兵元素进入视口，自动加载下一页。
watch(plazaSentinel, (el) => {
  plazaObserver?.disconnect()
  if (!el) return
  plazaObserver = new IntersectionObserver(
    (entries) => {
      if (entries.some((entry) => entry.isIntersecting)) void loadPlaza()
    },
    { rootMargin: '300px 0px' },
  )
  plazaObserver.observe(el)
})

const displayName = computed(() => (auth.isAuthenticated ? auth.username : '访客'))

function validateFile(file: { size: number; type: string; name: string }): string | null {
  if (!ACCEPT.split(',').includes(file.type)) return `不支持的格式：${file.name}`
  return null
}

async function uploadFiles(files: File[]): Promise<void> {
  if (files.length === 0) return
  const file = files[0]!
  const reason = validateFile(file)
  if (reason) {
    ElMessage.error(reason)
    return
  }
  uploading.value = true
  try {
    uploaded.value = await uploadImage(file)
    ElMessage.success('上传成功')
    if (auth.isAuthenticated) {
      // 已登录用户可在用户中心继续管理。
    }
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    uploading.value = false
    uploadRef.value?.clearFiles()
  }
}

function beforeUpload(file: { size: number; type: string; name: string }): boolean {
  const reason = validateFile(file)
  if (reason) {
    ElMessage.error(reason)
    return false
  }
  return true
}

function handleUpload(options: { file: File }): void {
  void uploadFiles([options.file])
}

function onDrop(event: DragEvent): void {
  dragActive.value = false
  const dropped = event.dataTransfer?.files
  if (!dropped || dropped.length === 0) return
  event.preventDefault()
  void uploadFiles([...dropped])
}

function onDragOver(event: DragEvent): void {
  if (event.dataTransfer?.types.includes('Files')) dragActive.value = true
}

function onDragLeave(event: DragEvent): void {
  if (event.relatedTarget === null) dragActive.value = false
}

function goUserCenter(): void {
  if (auth.isAuthenticated) {
    void router.push(auth.isAdmin ? '/admin' : '/user')
  } else {
    void router.push('/login')
  }
}

onMounted(async () => {
  try {
    announcements.value = await listAnnouncements()
  } catch {
    // 首页公告为可选内容。
  }
  void loadPlaza()
})

onBeforeUnmount(() => {
  plazaObserver?.disconnect()
})
</script>

<template>
  <div
    class="home"
    :class="{ 'is-dragging': dragActive }"
    @dragover="onDragOver"
    @dragleave="onDragLeave"
    @drop="onDrop"
  >
    <header class="home__topbar">
      <router-link to="/" class="brand" aria-label="AXmiPic 首页">
        <span class="brand__mark" aria-hidden="true">
          <svg viewBox="0 0 32 32" width="18" height="18">
            <defs>
              <linearGradient id="home-mark" x1="0" y1="0" x2="1" y2="1">
                <stop offset="0" stop-color="#828fff" />
                <stop offset="1" stop-color="#5e6ad2" />
              </linearGradient>
            </defs>
            <path d="M16 6.5 23.8 25.5h-4.05l-1.55-4.05h-4.4L12.25 25.5H8.2Z" fill="url(#home-mark)" />
          </svg>
        </span>
        <span class="brand__name">AXmiPic</span>
      </router-link>

      <div class="home__topbar-actions">
        <button
          class="icon-btn"
          type="button"
          :aria-label="theme.isDark ? '切换到浅色主题' : '切换到深色主题'"
          @click="theme.toggle()"
        >
          <el-icon :size="16"><component :is="theme.isDark ? Sunny : Moon" /></el-icon>
        </button>
        <el-button v-if="auth.isAuthenticated" type="primary" :icon="User" @click="goUserCenter">
          {{ auth.isAdmin ? '管理控制台' : '用户中心' }}
        </el-button>
        <template v-else>
          <el-button @click="router.push('/login')">登录</el-button>
          <el-button type="primary" @click="router.push('/login')">注册 / 登录</el-button>
        </template>
      </div>
    </header>

    <main class="home__main">
      <section class="hero">
        <div class="hero__glow" aria-hidden="true" />
        <h1 class="hero__title">轻量、可靠的自托管图床</h1>
        <p class="hero__desc">
          拖拽、粘贴或点击即可上传图片，实时获得可分享的直链。
        </p>
      </section>

      <section v-if="announcements.length > 0" class="home__announcements" aria-label="站内公告">
        <el-alert
          v-for="item in announcements.slice(0, 3)"
          :key="item.id"
          :type="item.level === 'info' ? 'info' : item.level"
          :closable="false"
          show-icon
        >
          <template #title>{{ item.title }}</template>
          <p v-if="item.content" class="home__announcement-body">{{ item.content }}</p>
        </el-alert>
      </section>

      <section class="upload-card ax-card">
        <div class="upload-card__head">
          <h2 class="upload-card__title">
            <el-icon :size="16"><Upload /></el-icon>
            上传图片
          </h2>
          <span class="upload-card__hint">
            支持 JPG / PNG / GIF / WebP · 当前身份：{{ displayName }}
          </span>
        </div>

        <el-upload
          ref="uploadRef"
          class="upload-card__drop"
          drag
          :show-file-list="false"
          :accept="ACCEPT"
          :multiple="false"
          :before-upload="beforeUpload"
          :http-request="handleUpload"
        >
          <el-icon class="upload-card__icon" :size="40"><UploadFilled /></el-icon>
          <div class="upload-card__text">
            将图片拖到此处，或<em>点击上传</em>
          </div>
          <template #tip>
            <p class="upload-card__tip">未登录访客也可上传，登录后可获得更大的配额与完整管理能力。</p>
          </template>
        </el-upload>

        <div v-if="uploading" class="upload-card__progress"><el-progress :indeterminate="true" :show-text="false" /></div>

        <div v-if="uploaded" class="upload-result">
          <div class="upload-result__preview">
            <img :src="uploaded.url" :alt="uploaded.original_name || uploaded.key" />
          </div>
          <div class="upload-result__meta">
            <p class="upload-result__name">{{ uploaded.original_name || uploaded.filename || uploaded.key }}</p>
            <CopyField label="图片直链" :value="uploaded.url" />
            <div class="upload-result__actions">
              <el-button
                size="small"
                :icon="CopyDocument"
                @click="router.push(auth.isAuthenticated ? (auth.isAdmin ? '/admin' : '/user/images') : '/login')"
              >
                {{ auth.isAuthenticated ? '去用户中心管理' : '登录以管理' }}
              </el-button>
            </div>
          </div>
        </div>
      </section>

      <section class="home__plaza" aria-label="图片广场">
        <div class="home__section-head">
          <h2 class="home__section-title"><el-icon :size="16"><Grid /></el-icon>图片广场</h2>
          <router-link v-if="auth.isAuthenticated" to="/user/plaza" class="home__section-link">
            查看全部<el-icon :size="12"><ArrowRight /></el-icon>
          </router-link>
        </div>

        <div v-if="plaza.length === 0 && plazaLoading" class="home__plaza-skeleton">
          <el-skeleton :rows="3" animated />
        </div>

        <div v-else-if="plaza.length > 0" class="home__plaza-masonry">
          <router-link
            v-for="item in plaza"
            :key="item.id"
            :to="auth.isAuthenticated ? '/user/plaza' : '/login'"
            class="home__plaza-card"
          >
            <img :src="item.url" :alt="item.original_name || item.key" loading="lazy" />
            <span class="home__plaza-meta">
              <el-icon :size="12"><Picture /></el-icon>
              {{ item.owner_username || '匿名' }}
              <span class="home__plaza-time">{{ formatDateTime(item.created_at) }}</span>
            </span>
          </router-link>
        </div>

        <p v-else class="home__plaza-empty">暂无公开图片</p>

        <div ref="plazaSentinel" class="home__plaza-sentinel" aria-hidden="true">
          <span v-if="plazaLoading && plaza.length > 0">正在加载更多…</span>
          <span v-else-if="!plazaHasMore && plaza.length > 0">已经到底啦</span>
        </div>
      </section>
    </main>

    <footer class="home__footer">
      <span>AXmiPic · 自托管图床</span>
      <router-link to="/login" class="home__footer-link">登录 / 注册</router-link>
    </footer>

    <div v-if="dragActive" class="home__dropzone" aria-hidden="true">
      <el-icon :size="40"><Upload /></el-icon>
      <span>松开以上传图片</span>
    </div>
  </div>
</template>

<style scoped>
.home {
  min-height: 100dvh;
  background: var(--ax-bg);
}

.home__topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  max-width: 1080px;
  margin: 0 auto;
  padding: var(--ax-space-4) var(--ax-space-5);
}

.brand {
  display: flex;
  align-items: center;
  gap: var(--ax-space-3);
  color: inherit;
}

.brand__mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  background: var(--ax-accent-soft);
  border: 1px solid var(--ax-accent-ring);
  border-radius: var(--ax-radius-md);
}

.brand__name {
  color: var(--ax-text);
  font-size: var(--ax-text-lg);
  font-weight: var(--ax-weight-semibold);
  letter-spacing: var(--ax-tracking-display);
}

.home__topbar-actions {
  display: flex;
  align-items: center;
  gap: var(--ax-space-2);
}

.icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  padding: 0;
  color: var(--ax-text-3);
  background: var(--ax-tint);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-sm);
  cursor: pointer;
}

.icon-btn:hover {
  color: var(--ax-text);
  background: var(--ax-tint-strong);
}

.home__main {
  max-width: 780px;
  margin: 0 auto;
  padding: var(--ax-space-6) var(--ax-space-5) var(--ax-space-12);
}

.hero {
  position: relative;
  padding: var(--ax-space-8) 0 var(--ax-space-6);
  text-align: center;
}

.hero__glow {
  position: absolute;
  inset: -20% 10% auto;
  height: 220px;
  background: var(--ax-aurora);
  filter: blur(60px);
  opacity: 0.5;
  pointer-events: none;
}

.hero__title {
  position: relative;
  margin: 0;
  color: var(--ax-text);
  font-size: clamp(1.8rem, 5vw, 2.6rem);
  font-weight: var(--ax-weight-semibold);
  letter-spacing: var(--ax-tracking-display);
}

.hero__desc {
  position: relative;
  max-width: 46ch;
  margin: var(--ax-space-3) auto 0;
  color: var(--ax-text-3);
  font-size: var(--ax-text-md);
}

.home__announcements {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-2);
  margin-bottom: var(--ax-space-4);
}

.home__announcement-body {
  margin: var(--ax-space-1) 0 0;
  white-space: pre-wrap;
}

.upload-card {
  margin-bottom: var(--ax-space-6);
}

.upload-card__head {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--ax-space-2);
  margin-bottom: var(--ax-space-3);
}

.upload-card__title {
  display: inline-flex;
  align-items: center;
  gap: var(--ax-space-2);
  margin: 0;
  color: var(--ax-text);
  font-size: var(--ax-text-lg);
}

.upload-card__hint {
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.upload-card__drop {
  width: 100%;
}

.upload-card__drop :deep(.el-upload-dragger) {
  width: 100%;
  padding: var(--ax-space-8) var(--ax-space-4);
  background: var(--ax-tint-weak);
  border: 1px dashed var(--ax-border-strong);
  border-radius: var(--ax-radius-md);
}

.upload-card__icon {
  color: var(--ax-accent-bright);
}

.upload-card__text {
  margin-top: var(--ax-space-3);
  color: var(--ax-text-2);
  font-size: var(--ax-text-sm);
}

.upload-card__text em {
  color: var(--ax-accent-hover);
  font-style: normal;
}

.upload-card__tip {
  margin: var(--ax-space-2) 0 0;
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
  text-align: center;
}

.upload-card__progress {
  margin-top: var(--ax-space-3);
}

.upload-result {
  display: grid;
  grid-template-columns: minmax(0, 160px) minmax(0, 1fr);
  gap: var(--ax-space-4);
  margin-top: var(--ax-space-4);
  padding-top: var(--ax-space-4);
  border-top: 1px solid var(--ax-border-subtle);
}

.upload-result__preview {
  overflow: hidden;
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-md);
}

.upload-result__preview img {
  display: block;
  width: 100%;
  height: 100%;
  aspect-ratio: 1;
  object-fit: cover;
}

.upload-result__meta {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-3);
  min-width: 0;
}

.upload-result__name {
  margin: 0;
  overflow: hidden;
  color: var(--ax-text);
  font-weight: var(--ax-weight-medium);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.home__plaza {
  margin-top: var(--ax-space-6);
}

.home__section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--ax-space-3);
}

.home__section-title {
  display: inline-flex;
  align-items: center;
  gap: var(--ax-space-2);
  margin: 0;
  color: var(--ax-text);
  font-size: var(--ax-text-lg);
}

.home__section-link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--ax-accent-hover);
  font-size: var(--ax-text-sm);
}

.home__plaza-masonry {
  column-count: 3;
  column-gap: var(--ax-space-3);
}

.home__plaza-card {
  display: block;
  break-inside: avoid;
  margin-bottom: var(--ax-space-3);
  overflow: hidden;
  background: var(--ax-tint-weak);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-md);
}

.home__plaza-card img {
  display: block;
  width: 100%;
  height: auto;
}

.home__plaza-skeleton {
  padding: var(--ax-space-2) 0;
}

.home__plaza-empty {
  margin: 0;
  padding: var(--ax-space-6) 0;
  color: var(--ax-text-4);
  font-size: var(--ax-text-sm);
  text-align: center;
}

.home__plaza-sentinel {
  display: flex;
  justify-content: center;
  min-height: 24px;
  padding: var(--ax-space-3) 0;
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.home__plaza-meta {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 6px var(--ax-space-2);
  overflow: hidden;
  color: var(--ax-text-4);
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.home__plaza-time {
  margin-left: auto;
}

.home__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  max-width: 1080px;
  margin: 0 auto;
  padding: var(--ax-space-5);
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
  border-top: 1px solid var(--ax-border-subtle);
}

.home__footer-link {
  color: var(--ax-accent-hover);
}

.home__dropzone {
  position: fixed;
  inset: 12px;
  z-index: var(--ax-z-drawer);
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--ax-space-3);
  color: var(--ax-accent-hover);
  font-size: var(--ax-text-lg);
  background: var(--ax-accent-soft);
  border: 2px dashed var(--ax-accent-bright);
  border-radius: var(--ax-radius-lg);
  pointer-events: none;
}

@media (max-width: 640px) {
  .upload-result {
    grid-template-columns: minmax(0, 1fr);
  }

  .home__plaza-masonry {
    column-count: 2;
  }
}
</style>

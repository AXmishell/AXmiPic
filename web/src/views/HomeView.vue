<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import {
  CopyDocument,
  Grid,
  Moon,
  Sunny,
  Upload,
  UploadFilled,
  User,
} from '@element-plus/icons-vue'

import { listAnnouncements } from '@/api/site'
import { uploadImage } from '@/api/images'
import { fetchHealthStatus, isDatabaseHealthy } from '@/api/health'
import { toApiError } from '@/api/client'
import type { Announcement, ImageItem } from '@/api/types'
import BrandMark from '@/components/BrandMark.vue'
import CopyField from '@/components/CopyField.vue'
import PlazaMasonry from '@/components/PlazaMasonry.vue'
import { useAuthStore } from '@/stores/auth'
import { useThemeStore } from '@/stores/theme'

const { t } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const theme = useThemeStore()

const uploadRef = ref<{ clearFiles: () => void } | null>(null)
const uploading = ref(false)
const dragActive = ref(false)
const uploaded = ref<ImageItem | null>(null)
const announcements = ref<Announcement[]>([])
const ACCEPT = 'image/jpeg,image/png,image/gif,image/webp'

/** 数据库异常提示文案；为 null 表示连接正常，不展示提示。 */
const dbIssue = ref<string | null>(null)
const dbChecking = ref(false)
const DB_CHECK_INTERVAL = 30_000
let dbTimer: number | undefined

const displayName = computed(() => (auth.isAuthenticated ? auth.username : t('home.guest')))

function validateFile(file: { size: number; type: string; name: string }): string | null {
  if (!ACCEPT.split(',').includes(file.type)) return t('home.unsupportedFormat', { name: file.name })
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
    ElMessage.success(t('home.uploadSuccess'))
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
  void checkDatabase()
  dbTimer = window.setInterval(() => {
    void checkDatabase()
  }, DB_CHECK_INTERVAL)
  try {
    announcements.value = await listAnnouncements()
  } catch {
    // 首页公告为可选内容。
  }
})

onBeforeUnmount(() => {
  if (dbTimer !== undefined) {
    window.clearInterval(dbTimer)
    dbTimer = undefined
  }
})

/**
 * 探测数据库连接状态：仅在异常时设置提示文案，连接正常时清除提示。
 * 后端不可达同样视为异常，因为此时页面的大部分功能都不可用。
 */
async function checkDatabase(): Promise<void> {
  if (dbChecking.value) return
  dbChecking.value = true
  try {
    const health = await fetchHealthStatus()
    dbIssue.value = isDatabaseHealthy(health)
      ? null
      : health.database?.message || t('home.dbUnavailable')
  } catch {
    dbIssue.value = t('home.dbConnectFailed')
  } finally {
    dbChecking.value = false
  }
}
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
      <router-link to="/" class="brand" :aria-label="t('home.aria.brandHome')">
        <span class="brand__mark" aria-hidden="true">
          <BrandMark :size="18" />
        </span>
        <span class="brand__name">AXmiPic</span>
      </router-link>

      <div class="home__topbar-actions">
        <a
          class="icon-btn home__github"
          href="https://github.com/AXmishell/AXmiPic"
          target="_blank"
          rel="noopener noreferrer"
          :aria-label="t('home.aria.githubRepo')"
          :title="t('home.aria.githubRepo')"
        >
          <svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true" fill="currentColor">
            <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27s1.36.09 2 .27c1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0 0 16 8c0-4.42-3.58-8-8-8z" />
          </svg>
        </a>
        <button
          class="icon-btn"
          type="button"
          :aria-label="theme.isDark ? t('topbar.switchToLight') : t('topbar.switchToDark')"
          @click="theme.toggle()"
        >
          <el-icon :size="16"><component :is="theme.isDark ? Sunny : Moon" /></el-icon>
        </button>
        <el-button v-if="auth.isAuthenticated" type="primary" :icon="User" @click="goUserCenter">
          {{ auth.isAdmin ? t('topbar.adminConsole') : t('topbar.userCenter') }}
        </el-button>
        <el-button v-else type="primary" @click="router.push('/login')">{{ t('home.loginRegister') }}</el-button>
      </div>
    </header>

    <main class="home__main">
      <section v-if="dbIssue" class="home__db-alert" aria-live="polite">
        <el-alert type="error" :closable="false" show-icon>
          <template #title>{{ t('home.dbAlertTitle') }}</template>
          <div class="home__db-alert-body">
            <span>{{ dbIssue }}</span>
            <el-button size="small" :loading="dbChecking" @click="checkDatabase">{{ t('common.retry') }}</el-button>
            <el-button size="small" link type="primary" @click="router.push('/install')">
              {{ t('home.goInstall') }}
            </el-button>
          </div>
        </el-alert>
      </section>

      <section class="hero">
        <div class="hero__glow" aria-hidden="true" />
        <span class="hero__eyebrow">{{ t('home.hero.eyebrow') }}</span>
        <h1 class="hero__title">{{ t('home.hero.title') }}</h1>
        <p class="hero__desc">
          {{ t('home.hero.desc') }}
        </p>
      </section>

      <section v-if="announcements.length > 0" class="home__announcements" :aria-label="t('home.aria.announcements')">
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
            {{ t('home.upload.title') }}
          </h2>
          <span class="upload-card__hint">
            {{ t('home.upload.hint', { name: displayName }) }}
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
            {{ t('home.upload.dropPrefix') }}<em>{{ t('home.upload.dropAction') }}</em>
          </div>
          <template #tip>
            <p class="upload-card__tip">{{ t('home.upload.tip') }}</p>
          </template>
        </el-upload>

        <div v-if="uploading" class="upload-card__progress">
          <el-progress :indeterminate="true" :show-text="false" />
        </div>

        <div v-if="uploaded" class="upload-result">
          <div class="upload-result__preview">
            <img :src="uploaded.url" :alt="uploaded.original_name || uploaded.key" />
          </div>
          <div class="upload-result__meta">
            <p class="upload-result__name">
              {{ uploaded.original_name || uploaded.filename || uploaded.key }}
            </p>
            <CopyField :label="t('home.upload.imageLink')" :value="uploaded.url" />
            <div class="upload-result__actions">
              <el-button
                size="small"
                :icon="CopyDocument"
                @click="router.push(auth.isAuthenticated ? (auth.isAdmin ? '/admin' : '/user/images') : '/login')"
              >
                {{ auth.isAuthenticated ? t('home.upload.goManage') : t('home.upload.loginToManage') }}
              </el-button>
            </div>
          </div>
        </div>
      </section>

      <section class="home__steps" :aria-label="t('home.aria.steps')">
        <div class="home__step">
          <span class="home__step-index">01</span>
          <h3 class="home__step-title">{{ t('home.steps.upload.title') }}</h3>
          <p class="home__step-desc">{{ t('home.steps.upload.desc') }}</p>
        </div>
        <div class="home__step">
          <span class="home__step-index">02</span>
          <h3 class="home__step-title">{{ t('home.steps.process.title') }}</h3>
          <p class="home__step-desc">{{ t('home.steps.process.desc') }}</p>
        </div>
        <div class="home__step">
          <span class="home__step-index">03</span>
          <h3 class="home__step-title">{{ t('home.steps.distribute.title') }}</h3>
          <p class="home__step-desc">{{ t('home.steps.distribute.desc') }}</p>
        </div>
      </section>
    </main>

    <section class="home__plaza" :aria-label="t('home.aria.plaza')">
      <div class="home__plaza-inner">
        <div class="home__section-head">
          <div>
            <h2 class="home__section-title"><el-icon :size="16"><Grid /></el-icon>{{ t('home.plaza.title') }}</h2>
            <p class="home__section-desc">{{ t('home.plaza.desc') }}</p>
          </div>
          <router-link class="home__section-link" to="/plaza">{{ t('home.plaza.viewAll') }}</router-link>
        </div>
        <PlazaMasonry />
      </div>
    </section>

    <footer class="home__footer">
      <div class="home__footer-inner">
        <div class="home__footer-brand">
          <span class="brand__mark brand__mark--small" aria-hidden="true">
            <BrandMark :size="14" />
          </span>
          <span>AXmiPic</span>
          <span class="home__footer-muted">{{ t('home.footer.muted') }}</span>
        </div>
        <nav class="home__footer-links" :aria-label="t('home.aria.footerNav')">
          <a href="https://axmipic.gpcn.cc/" target="_blank" rel="noopener noreferrer">{{ t('home.footer.docs') }}</a>
          <router-link to="/plaza">{{ t('home.footer.plaza') }}</router-link>
          <a
            href="https://github.com/AXmishell/AXmiPic"
            target="_blank"
            rel="noopener noreferrer"
          >
            GitHub
          </a>
        </nav>
      </div>
    </footer>

    <div v-if="dragActive" class="home__dropzone" aria-hidden="true">
      <el-icon :size="40"><Upload /></el-icon>
      <span>{{ t('home.upload.releaseToUpload') }}</span>
    </div>
  </div>
</template>

<style scoped>
.home {
  min-height: 100dvh;
  background: var(--ax-atmosphere), var(--ax-canvas);
}

.home__topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--ax-space-3);
  max-width: 1080px;
  margin: 0 auto;
  padding: var(--ax-space-4) var(--ax-space-5);
}

.brand {
  display: flex;
  align-items: center;
  gap: var(--ax-space-3);
  min-width: 0;
  color: inherit;
}

.brand__mark {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  background: var(--ax-accent-soft);
  border: 1px solid var(--ax-accent-ring);
  border-radius: var(--ax-radius-md);
}

.brand__mark--small {
  width: 24px;
  height: 24px;
  border-radius: var(--ax-radius-sm);
}

.brand__name {
  color: var(--ax-text);
  font-size: var(--ax-text-lg);
  font-weight: var(--ax-weight-semibold);
  letter-spacing: var(--ax-tracking-display);
  white-space: nowrap;
}

.home__topbar-actions {
  display: flex;
  flex: none;
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
  text-decoration: none;
}

.icon-btn:hover {
  color: var(--ax-text);
  background: var(--ax-tint-strong);
}

.home__main {
  max-width: 780px;
  margin: 0 auto;
  padding: var(--ax-space-6) var(--ax-space-5) var(--ax-space-10);
}

.home__db-alert {
  margin-bottom: var(--ax-space-4);
}

.home__db-alert-body {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--ax-space-2);
  margin-top: var(--ax-space-1);
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

.hero__eyebrow {
  position: relative;
  display: inline-flex;
  align-items: center;
  gap: var(--ax-space-2);
  margin-bottom: var(--ax-space-4);
  padding: 4px 10px;
  color: var(--ax-text-3);
  font-size: var(--ax-text-xs);
  background: var(--ax-tint-weak);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-full);
}

.hero__title {
  position: relative;
  margin: 0;
  color: var(--ax-text);
  font-size: clamp(1.75rem, 5vw, 2.5rem);
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
  padding: var(--ax-space-5);
  margin-bottom: var(--ax-space-4);
}

.upload-card__head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--ax-space-2);
  margin-bottom: var(--ax-space-4);
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

.home__steps {
  display: grid;
  gap: var(--ax-space-3);
  grid-template-columns: repeat(3, minmax(0, 1fr));
  margin-bottom: var(--ax-space-2);
}

.home__step {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: var(--ax-space-4);
  background: var(--ax-tint-weak);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-md);
}

.home__step-index {
  margin-bottom: var(--ax-space-1);
  color: var(--ax-accent-hover);
  font-family: var(--ax-font-mono);
  font-size: var(--ax-text-xs);
}

.home__step-title {
  color: var(--ax-text);
  font-size: var(--ax-text-sm);
  font-weight: var(--ax-weight-medium);
}

.home__step-desc {
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
  line-height: 1.45;
}

.home__plaza {
  border-top: 1px solid var(--ax-border-subtle);
}

.home__plaza-inner {
  max-width: 1080px;
  margin: 0 auto;
  padding: var(--ax-space-8) var(--ax-space-5) var(--ax-space-10);
}

.home__section-head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--ax-space-4);
  margin-bottom: var(--ax-space-4);
}

.home__section-title {
  display: inline-flex;
  align-items: center;
  gap: var(--ax-space-2);
  margin: 0;
  color: var(--ax-text);
  font-size: var(--ax-text-lg);
}

.home__section-desc {
  margin-top: var(--ax-space-1);
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.home__section-link {
  flex: none;
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
}

.home__section-link:hover {
  color: var(--ax-accent-hover);
}

.home__footer {
  border-top: 1px solid var(--ax-border-subtle);
}

.home__footer-inner {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--ax-space-3);
  max-width: 1080px;
  margin: 0 auto;
  padding: var(--ax-space-5);
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.home__footer-brand {
  display: flex;
  align-items: center;
  gap: var(--ax-space-2);
  color: var(--ax-text-3);
}

.home__footer-muted {
  color: var(--ax-text-4);
}

.home__footer-links {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--ax-space-4);
}

.home__footer-links a {
  color: var(--ax-text-3);
}

.home__footer-links a:hover {
  color: var(--ax-text);
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
  .home__main {
    padding-top: var(--ax-space-4);
  }

  .hero {
    padding-top: var(--ax-space-6);
  }

  .hero__eyebrow {
    margin-bottom: var(--ax-space-3);
  }

  .upload-card {
    padding: var(--ax-space-4);
  }

  .upload-card__hint {
    flex: 1 1 100%;
    margin-left: 0;
  }

  .home__steps {
    grid-template-columns: minmax(0, 1fr);
  }

  .upload-result {
    grid-template-columns: minmax(0, 1fr);
  }

  .home__footer-inner {
    flex-direction: column;
    align-items: flex-start;
  }
}

@media (max-width: 480px) {
  .home__github {
    display: none;
  }

  .home__section-link {
    display: none;
  }
}
</style>

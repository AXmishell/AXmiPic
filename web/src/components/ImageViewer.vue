<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import {
  ArrowLeft,
  ArrowRight,
  ArrowUp,
  Close,
  CopyDocument,
  FullScreen,
  InfoFilled,
  Link,
  RefreshLeft,
  ZoomIn,
  ZoomOut,
} from '@element-plus/icons-vue'

import type { ImageItem } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import { copyText } from '@/utils/clipboard'
import { formatBytes, formatDateTime, formatDimensions, formatMime } from '@/utils/format'

const open = defineModel<boolean>({ default: false })
const index = defineModel<number>('index', { default: 0 })

const props = defineProps<{ items: ImageItem[]; albumName?: (id?: string) => string }>()

const auth = useAuthStore()
const { t } = useI18n()

const scale = ref(1)
const offsetX = ref(0)
const offsetY = ref(0)
const dragging = ref(false)
const ready = ref(false)
const failed = ref(false)
const naturalW = ref(0)
const naturalH = ref(0)
const stageW = ref(0)
const stageH = ref(0)
const stageEl = ref<HTMLElement | null>(null)
// 移动端布局：底部信息改为可展开抽屉，默认只用一行摘要提示。
const isMobileLayout = ref(false)
const infoOpen = ref(false)
let mobileMedia: MediaQueryList | null = null
// 设备像素比。默认缩放取 1/dpr，使 1 图像 px = 1 物理像素。
const dpr = ref(typeof window !== 'undefined' && window.devicePixelRatio ? window.devicePixelRatio : 1)

const start = { x: 0, y: 0, ox: 0, oy: 0 }
// 触摸手势：记录活动指针，支持双指缩放与单指平移/左右滑动切换。
const activePointers = new Map<number, { x: number; y: number }>()
const pinchState = {
  active: false,
  startDistance: 0,
  startScale: 1,
  anchorX: 0,
  anchorY: 0,
}
const swipeState = { tracking: false, x: 0, y: 0, horizontal: false }
const SWIPE_THRESHOLD = 56
const MIN_SCALE = 0.05
const MAX_SCALE = 8

const current = computed(() => props.items[index.value])
const hasPrev = computed(() => index.value > 0)
const hasNext = computed(() => index.value < props.items.length - 1)
const title = computed(
  () => current.value?.original_name || current.value?.filename || current.value?.key || '',
)
// 已保存的默认显示模式：fit=适应窗口，actual=原始像素 1:1。
// 手机屏幕较窄，未显式设置偏好时默认适应窗口，避免一打开就是超大原图。
const defaultMode = computed<'fit' | 'actual'>(() => {
  const saved = auth.preferences?.viewer_mode
  if (saved) return saved
  const mobile = typeof window !== 'undefined' && window.matchMedia('(max-width: 768px)').matches
  return mobile ? 'fit' : 'actual'
})

// 渲染尺寸（px）= 原图像素 × 缩放。默认 scale=1，即 100% 按原始像素尺寸展示。
const renderedW = computed(() => naturalW.value * scale.value)
const renderedH = computed(() => naturalH.value * scale.value)
// 仅当图片超出舞台时才允许拖拽平移。
const canPan = computed(
  () => renderedW.value > stageW.value + 1 || renderedH.value > stageH.value + 1,
)

function clampOffset(): void {
  const maxX = Math.max(0, (renderedW.value - stageW.value) / 2)
  const maxY = Math.max(0, (renderedH.value - stageH.value) / 2)
  offsetX.value = Math.min(maxX, Math.max(-maxX, offsetX.value))
  offsetY.value = Math.min(maxY, Math.max(-maxY, offsetY.value))
}

function measureStage(): void {
  if (!stageEl.value) return
  syncDpr()
  stageW.value = stageEl.value.clientWidth
  stageH.value = stageEl.value.clientHeight
  clampOffset()
}

function setNatural(): void {
  // 100% = 原始像素尺寸：1 图像 px = 1 物理设备像素。
  dpr.value = window.devicePixelRatio || 1
  scale.value = 1 / dpr.value
  offsetX.value = 0
  offsetY.value = 0
}

function resetView(): void {
  cancelGestures()
  setNatural()
  ready.value = false
  failed.value = false
  naturalW.value = 0
  naturalH.value = 0
}

/** 缩放到刚好适应舞台的比例。 */
function fitScale(): number {
  if (!naturalW.value || !naturalH.value || !stageW.value || !stageH.value) return 1
  return Math.min(stageW.value / naturalW.value, stageH.value / naturalH.value)
}

/** 缩放到刚好适应舞台。 */
function fitToWindow(): void {
  scale.value = Math.min(MAX_SCALE, Math.max(MIN_SCALE, fitScale()))
  offsetX.value = 0
  offsetY.value = 0
}

/** 在「适应窗口」与「原始尺寸 1:1」之间切换（双击舞台触发，不改变默认）。 */
function toggleFit(): void {
  if (Math.abs(scale.value - fitScale()) < 0.01) {
    setNatural()
  } else {
    fitToWindow()
  }
}

/** 应用已保存的默认显示模式。 */
function applyDefaultMode(): void {
  if (!naturalW.value) return
  if (defaultMode.value === 'fit') {
    fitToWindow()
  } else {
    setNatural()
  }
}

/** 选择「适应窗口」并记忆为默认。 */
function chooseFit(): void {
  fitToWindow()
  void auth.savePreferences({ viewer_mode: 'fit' })
}

/** 选择「原始像素 100%」并记忆为默认。 */
function chooseActual(): void {
  setNatural()
  void auth.savePreferences({ viewer_mode: 'actual' })
}

// 单击舞台空白处关闭，但延迟一小段时间，若紧接着是双击则取消关闭，
// 从而让“双击切换 1:1/适应”在空白处也能生效。
let closeTimer: number | null = null
// 本轮指针交互是否发生了拖动位移（用于抑制拖拽结束后误触发的 click 关闭）。
let moved = false

function scheduleClose(): void {
  if (closeTimer !== null) window.clearTimeout(closeTimer)
  closeTimer = window.setTimeout(() => {
    closeTimer = null
    close()
  }, 220)
}

function cancelClose(): void {
  if (closeTimer !== null) {
    window.clearTimeout(closeTimer)
    closeTimer = null
  }
}

function onStageClick(): void {
  // 刚结束一次拖拽平移时，松开鼠标产生的 click 不应关闭查看器。
  if (moved) {
    moved = false
    return
  }
  scheduleClose()
}

function onStageDblclick(): void {
  cancelClose()
  toggleFit()
}

function close(): void {
  open.value = false
}

function go(delta: number): void {
  const next = index.value + delta
  if (next < 0 || next >= props.items.length) return
  index.value = next
  resetView()
}

function zoomBy(factor: number): void {
  scale.value = Math.min(MAX_SCALE, Math.max(MIN_SCALE, scale.value * factor))
  clampOffset()
}

// 以鼠标指针为锚点缩放：保持指针下方的图像点在屏幕上不动。
function onWheel(event: WheelEvent): void {
  event.preventDefault()
  const factor = event.deltaY < 0 ? 1.2 : 1 / 1.2
  const next = Math.min(MAX_SCALE, Math.max(MIN_SCALE, scale.value * factor))
  if (next === scale.value) return
  const rect = stageEl.value?.getBoundingClientRect()
  if (rect) {
    const px = event.clientX - (rect.left + rect.width / 2)
    const py = event.clientY - (rect.top + rect.height / 2)
    const ux = (px - offsetX.value) / scale.value
    const uy = (py - offsetY.value) / scale.value
    offsetX.value = px - ux * next
    offsetY.value = py - uy * next
  }
  scale.value = next
  clampOffset()
}

/** 客户端坐标转换为相对舞台中心的坐标。 */
function stagePoint(clientX: number, clientY: number): { x: number; y: number } {
  const rect = stageEl.value?.getBoundingClientRect()
  if (!rect) return { x: 0, y: 0 }
  return {
    x: clientX - (rect.left + rect.width / 2),
    y: clientY - (rect.top + rect.height / 2),
  }
}

/** 清理进行中的触摸手势（不重置 moved，避免滑动切换后紧跟的 click 关闭查看器）。 */
function cancelGestures(): void {
  activePointers.clear()
  pinchState.active = false
  swipeState.tracking = false
  dragging.value = false
}

function startPinch(): void {
  const points = [...activePointers.values()]
  if (points.length < 2) return
  const [a, b] = points
  const midX = (a.x + b.x) / 2
  const midY = (a.y + b.y) / 2
  const point = stagePoint(midX, midY)
  pinchState.active = true
  pinchState.startDistance = Math.max(1, Math.hypot(a.x - b.x, a.y - b.y))
  pinchState.startScale = scale.value
  pinchState.anchorX = (point.x - offsetX.value) / scale.value
  pinchState.anchorY = (point.y - offsetY.value) / scale.value
  dragging.value = false
  swipeState.tracking = false
  moved = true
}

function onPointerDown(event: PointerEvent): void {
  // 交互控件（左右切换按钮等）不参与手势，也不捕获指针，否则 click 会被
  // 重定向到舞台，导致按钮失效并误触发关闭。
  if ((event.target as HTMLElement | null)?.closest('button')) return
  if (event.pointerType === 'mouse' && event.button !== 0) return

  ;(event.currentTarget as HTMLElement).setPointerCapture?.(event.pointerId)
  activePointers.set(event.pointerId, { x: event.clientX, y: event.clientY })

  if (activePointers.size === 2) {
    startPinch()
    return
  }
  if (activePointers.size !== 1) return

  moved = false
  if (canPan.value) {
    dragging.value = true
    start.x = event.clientX
    start.y = event.clientY
    start.ox = offsetX.value
    start.oy = offsetY.value
    return
  }
  // 图片完整显示时，单指左右滑动用于切换上一张/下一张。
  swipeState.tracking = true
  swipeState.x = event.clientX
  swipeState.y = event.clientY
  swipeState.horizontal = false
}

function onPointerMove(event: PointerEvent): void {
  const pointer = activePointers.get(event.pointerId)
  if (!pointer) return
  pointer.x = event.clientX
  pointer.y = event.clientY

  if (pinchState.active && activePointers.size >= 2) {
    const points = [...activePointers.values()]
    const [a, b] = points
    const distance = Math.max(1, Math.hypot(a.x - b.x, a.y - b.y))
    const nextScale = Math.min(
      MAX_SCALE,
      Math.max(MIN_SCALE, pinchState.startScale * (distance / pinchState.startDistance)),
    )
    const point = stagePoint((a.x + b.x) / 2, (a.y + b.y) / 2)
    // 以双指中点为锚点缩放，同时允许双指整体移动来平移图片。
    scale.value = nextScale
    offsetX.value = point.x - pinchState.anchorX * nextScale
    offsetY.value = point.y - pinchState.anchorY * nextScale
    clampOffset()
    moved = true
    return
  }

  if (dragging.value) {
    const dx = event.clientX - start.x
    const dy = event.clientY - start.y
    if (Math.abs(dx) + Math.abs(dy) > 4) moved = true
    offsetX.value = start.ox + dx
    offsetY.value = start.oy + dy
    clampOffset()
    return
  }

  if (swipeState.tracking) {
    const dx = event.clientX - swipeState.x
    const dy = event.clientY - swipeState.y
    if (Math.abs(dx) + Math.abs(dy) > 8) moved = true
    if (Math.abs(dx) > 12 && Math.abs(dx) > Math.abs(dy)) {
      swipeState.horizontal = true
    }
  }
}

function onPointerUp(event: PointerEvent): void {
  activePointers.delete(event.pointerId)
  const cancelled = event.type === 'pointercancel'

  if (pinchState.active) {
    if (activePointers.size < 2) {
      pinchState.active = false
      // 双指中抬起一根后，剩下的一根手指继续平移。
      if (!cancelled && activePointers.size === 1 && canPan.value) {
        const point = [...activePointers.values()][0]
        dragging.value = true
        start.x = point.x
        start.y = point.y
        start.ox = offsetX.value
        start.oy = offsetY.value
      } else {
        dragging.value = false
      }
    }
    return
  }

  if (swipeState.tracking) {
    const dx = event.clientX - swipeState.x
    const dy = event.clientY - swipeState.y
    const shouldSwitch =
      !cancelled &&
      swipeState.horizontal &&
      Math.abs(dx) >= SWIPE_THRESHOLD &&
      Math.abs(dx) > Math.abs(dy) * 1.2
    swipeState.tracking = false
    if (shouldSwitch) {
      moved = true
      go(dx > 0 ? -1 : 1)
    }
  }
  dragging.value = false
}

function onImgLoad(event: Event): void {
  const img = event.target as HTMLImageElement
  naturalW.value = img.naturalWidth
  naturalH.value = img.naturalHeight
  failed.value = false
  measureStage()
  applyDefaultMode()
  ready.value = true
}

function onImgError(): void {
  failed.value = true
  ready.value = false
}

async function copyLink(): Promise<void> {
  const url = current.value?.url
  if (!url) return
  const copied = await copyText(url)
  if (copied) {
    ElMessage.success(t('imageViewer.messages.copied'))
  } else {
    ElMessage.error(t('imageViewer.messages.copyFailed'))
  }
}

function openOriginal(): void {
  const url = current.value?.url
  if (url) window.open(url, '_blank', 'noopener,noreferrer')
}

function onKeydown(event: KeyboardEvent): void {
  if (!open.value) return
  switch (event.key) {
    case 'ArrowLeft':
      go(-1)
      break
    case 'ArrowRight':
      go(1)
      break
    case 'Escape':
      if (isMobileLayout.value && infoOpen.value) {
        infoOpen.value = false
      } else {
        close()
      }
      break
    case 'i':
    case 'I':
      if (isMobileLayout.value) infoOpen.value = !infoOpen.value
      break
    case '+':
    case '=':
      zoomBy(1.2)
      break
    case '-':
      zoomBy(1 / 1.2)
      break
    case '0':
      setNatural()
      break
    case 'f':
    case 'F':
      fitToWindow()
      break
  }
}

let observer: ResizeObserver | null = null

function setupObserver(): void {
  if (observer || !stageEl.value) return
  observer = new ResizeObserver(() => measureStage())
  observer.observe(stageEl.value)
}

function teardownObserver(): void {
  observer?.disconnect()
  observer = null
}

let dprMedia: MediaQueryList | null = null

// DPR 变化（例如窗口移到不同缩放率的显示器）时，若当前处于“原始尺寸”
// 状态则跟随更新，始终保持 1 图像 px = 1 物理像素。
function syncDpr(): void {
  const next = window.devicePixelRatio || 1
  const prev = dpr.value || 1
  if (next === prev) return
  const wasNatural = Math.abs(scale.value - 1 / prev) < 0.001
  dpr.value = next
  if (wasNatural) {
    scale.value = 1 / next
    clampOffset()
  }
  watchDprMedia()
}

function watchDprMedia(): void {
  dprMedia?.removeEventListener('change', syncDpr)
  dprMedia = window.matchMedia(`(resolution: ${window.devicePixelRatio}dppx)`)
  dprMedia.addEventListener('change', syncDpr)
}

/** 同步移动端布局；切回桌面时收起抽屉，避免信息面板停留在隐藏状态。 */
function syncMobileLayout(event?: MediaQueryListEvent): void {
  isMobileLayout.value = event ? event.matches : Boolean(mobileMedia?.matches)
  if (!isMobileLayout.value) infoOpen.value = false
}

watch(open, async (value) => {
  document.body.style.overflow = value ? 'hidden' : ''
  moved = false
  infoOpen.value = false
  if (value) {
    resetView()
    await nextTick()
    measureStage()
    setupObserver()
  } else {
    cancelGestures()
    teardownObserver()
  }
})

watch(index, () => {
  // 通过按钮切换时由 go() 处理；此处兜底外部改动。
  ready.value = false
})

onMounted(() => {
  window.addEventListener('keydown', onKeydown)
  window.addEventListener('resize', syncDpr)
  watchDprMedia()
  mobileMedia = window.matchMedia('(max-width: 768px)')
  syncMobileLayout()
  mobileMedia.addEventListener('change', syncMobileLayout)
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
  window.removeEventListener('resize', syncDpr)
  dprMedia?.removeEventListener('change', syncDpr)
  mobileMedia?.removeEventListener('change', syncMobileLayout)
  cancelGestures()
  teardownObserver()
  cancelClose()
  document.body.style.overflow = ''
})
</script>

<template>
  <Teleport to="body">
    <Transition name="ax-viewer">
      <div
        v-if="open && current"
        class="ax-viewer"
        role="dialog"
        aria-modal="true"
        :aria-label="title"
        @click.self="close"
      >
        <header class="ax-viewer__bar">
          <p class="ax-viewer__title" :title="title">{{ title }}</p>
          <div class="ax-viewer__tools">
            <span class="ax-viewer__counter">{{ index + 1 }} / {{ items.length }}</span>
            <el-button circle :icon="ZoomOut" :title="t('imageViewer.toolbar.zoomOut')" @click="zoomBy(1 / 1.2)" />
            <span class="ax-viewer__zoom">{{ Math.round(scale * dpr * 100) }}%</span>
            <el-button circle :icon="ZoomIn" :title="t('imageViewer.toolbar.zoomIn')" @click="zoomBy(1.2)" />
            <el-button
              circle
              :icon="FullScreen"
              :type="defaultMode === 'fit' ? 'primary' : ''"
              :title="t('imageViewer.toolbar.fit')"
              @click="chooseFit"
            />
            <el-button
              circle
              :icon="RefreshLeft"
              :type="defaultMode === 'actual' ? 'primary' : ''"
              :title="t('imageViewer.toolbar.actual')"
              @click="chooseActual"
            />
            <el-button circle :icon="CopyDocument" :title="t('imageViewer.toolbar.copyLink')" @click="copyLink" />
            <el-button circle :icon="Link" :title="t('imageViewer.toolbar.openOriginal')" @click="openOriginal" />
            <el-button circle :icon="Close" :title="t('imageViewer.toolbar.close')" @click="close" />
          </div>
        </header>

        <div
          ref="stageEl"
          class="ax-viewer__stage"
          :class="{ 'is-dragging': dragging, 'is-zoomed': canPan }"
          @click.self="onStageClick"
          @dblclick="onStageDblclick"
          @wheel="onWheel"
          @pointerdown="onPointerDown"
          @pointermove="onPointerMove"
          @pointerup="onPointerUp"
          @pointercancel="onPointerUp"
        >
          <button
            v-if="hasPrev"
            class="ax-viewer__nav is-prev"
            type="button"
            :aria-label="t('imageViewer.prev')"
            @pointerdown.stop
            @dblclick.stop
            @click.stop="go(-1)"
          >
            <el-icon :size="24"><ArrowLeft /></el-icon>
          </button>

          <img
            class="ax-viewer__img"
            :class="{ 'is-ready': ready }"
            :src="current.url"
            :alt="title"
            draggable="false"
            :style="{ transform: `translate(${offsetX}px, ${offsetY}px) scale(${scale})` }"
            @load="onImgLoad"
            @error="onImgError"
          />
          <span v-if="failed" class="ax-viewer__error">{{ t('imageViewer.loadFailed') }}</span>

          <button
            v-if="hasNext"
            class="ax-viewer__nav is-next"
            type="button"
            :aria-label="t('imageViewer.next')"
            @pointerdown.stop
            @dblclick.stop
            @click.stop="go(1)"
          >
            <el-icon :size="24"><ArrowRight /></el-icon>
          </button>
        </div>

        <button
          v-if="isMobileLayout"
          class="ax-viewer__summary"
          type="button"
          :aria-expanded="infoOpen"
          aria-controls="ax-viewer-info"
          @click="infoOpen = true"
        >
          <el-icon class="ax-viewer__summary-icon"><InfoFilled /></el-icon>
          <span class="ax-viewer__summary-text">
            {{ formatDimensions(current.width, current.height) }}
            <span aria-hidden="true">·</span>
            {{ formatBytes(current.size) }}
            <span aria-hidden="true">·</span>
            {{ formatMime(current.mime_type) }}
          </span>
          <el-icon class="ax-viewer__summary-arrow" :class="{ 'is-open': infoOpen }"><ArrowUp /></el-icon>
        </button>

        <div
          v-if="isMobileLayout && infoOpen"
          class="ax-viewer__info-backdrop"
          aria-hidden="true"
          @click="infoOpen = false"
        />

        <aside
          id="ax-viewer-info"
          class="ax-viewer__info"
          :class="{ 'is-open': infoOpen }"
          :aria-hidden="isMobileLayout && !infoOpen"
        >
          <span v-if="isMobileLayout" class="ax-viewer__info-handle" aria-hidden="true" />
          <header class="ax-viewer__info-head">
            <h2 class="ax-viewer__info-title">{{ t('imageViewer.info.title') }}</h2>
            <button
              v-if="isMobileLayout"
              class="ax-viewer__info-close"
              type="button"
              :aria-label="t('imageViewer.info.close')"
              @click="infoOpen = false"
            >
              <el-icon :size="18"><Close /></el-icon>
            </button>
          </header>
          <dl class="ax-viewer__fields">
            <div class="ax-viewer__field">
              <dt>{{ t('imageViewer.info.name') }}</dt>
              <dd>{{ current.original_name || '—' }}</dd>
            </div>
            <div v-if="current.owner_username" class="ax-viewer__field">
              <dt>{{ t('imageViewer.info.owner') }}</dt>
              <dd>{{ current.owner_username }}</dd>
            </div>
            <div v-if="current.author_name || current.author_handle" class="ax-viewer__field">
              <dt>{{ t('imageViewer.info.author') }}</dt>
              <dd>
                <a
                  v-if="current.author_url"
                  class="ax-viewer__author-link"
                  :href="current.author_url"
                  target="_blank"
                  rel="noopener noreferrer nofollow"
                  :title="current.author_url"
                >
                  <span v-if="current.author_name">{{ current.author_name }}</span>
                  <span v-if="current.author_handle" class="ax-viewer__handle">
                    @{{ current.author_handle }}
                  </span>
                </a>
                <template v-else>
                  <span v-if="current.author_name">{{ current.author_name }}</span>
                  <span v-if="current.author_handle">@{{ current.author_handle }}</span>
                </template>
              </dd>
            </div>
            <div v-if="current.source_url" class="ax-viewer__field">
              <dt>{{ t('imageViewer.info.source') }}</dt>
              <dd>
                <a
                  class="ax-viewer__source-link"
                  :href="current.source_url"
                  target="_blank"
                  rel="noopener noreferrer nofollow"
                  :title="current.source_url"
                >
                  {{ t('imageViewer.info.viewPost') }}
                </a>
              </dd>
            </div>
            <div class="ax-viewer__field">
              <dt>{{ t('imageViewer.info.storageName') }}</dt>
              <dd>{{ current.filename || current.key }}</dd>
            </div>
            <div class="ax-viewer__field">
              <dt>{{ t('imageViewer.info.dimensions') }}</dt>
              <dd>{{ formatDimensions(current.width, current.height) }}</dd>
            </div>
            <div class="ax-viewer__field">
              <dt>{{ t('imageViewer.info.size') }}</dt>
              <dd>{{ formatBytes(current.size) }}</dd>
            </div>
            <div class="ax-viewer__field">
              <dt>{{ t('imageViewer.info.format') }}</dt>
              <dd>{{ formatMime(current.mime_type) }}</dd>
            </div>
            <div class="ax-viewer__field">
              <dt>{{ t('imageViewer.info.visibility') }}</dt>
              <dd>{{ current.permission === 'public' ? t('imageViewer.info.public') : t('imageViewer.info.private') }}</dd>
            </div>
            <div class="ax-viewer__field">
              <dt>{{ t('imageViewer.info.album') }}</dt>
              <dd>{{ props.albumName?.(current.album_id) || t('imageViewer.info.noAlbum') }}</dd>
            </div>
            <div class="ax-viewer__field">
              <dt>{{ t('imageViewer.info.uploadedAt') }}</dt>
              <dd>{{ formatDateTime(current.created_at) }}</dd>
            </div>
            <div class="ax-viewer__field">
              <dt>{{ t('imageViewer.info.hash') }}</dt>
              <dd class="ax-viewer__mono">{{ current.hash || '—' }}</dd>
            </div>
            <div class="ax-viewer__field">
              <dt>{{ t('imageViewer.info.link') }}</dt>
              <dd class="ax-viewer__link" :title="current.url" @click="copyLink">{{ current.url }}</dd>
            </div>
          </dl>
        </aside>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.ax-viewer {
  position: fixed;
  inset: 0;
  z-index: var(--ax-z-modal, 3000);
  display: grid;
  /* 使用 minmax(0, 1fr)：默认的 1fr 会被工具栏的 min-content 撑宽，移动端
     会导致舞台超出视口、图片看起来偏向右侧。 */
  grid-template-columns: minmax(0, 1fr) 320px;
  grid-template-rows: auto minmax(0, 1fr);
  grid-template-areas:
    'bar info'
    'stage info';
  overflow: hidden;
  color: #f5f5f5;
  background: rgba(8, 8, 10, 0.96);
  backdrop-filter: blur(4px);
}

.ax-viewer__bar {
  grid-area: bar;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px var(--ax-space-4);
  min-width: 0;
  padding: 10px 16px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}

.ax-viewer__title {
  flex: 1 1 160px;
  min-width: 0;
  margin: 0;
  overflow: hidden;
  color: #fff;
  font-size: var(--ax-text-sm);
  font-weight: var(--ax-weight-medium);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ax-viewer__tools {
  display: flex;
  flex: 1 1 auto;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: var(--ax-space-2);
  min-width: 0;
}

.ax-viewer__counter,
.ax-viewer__zoom {
  color: rgba(255, 255, 255, 0.7);
  font-size: var(--ax-text-xs);
  font-variant-numeric: tabular-nums;
}

.ax-viewer__zoom {
  min-width: 44px;
  text-align: center;
}

.ax-viewer__stage {
  position: relative;
  grid-area: stage;
  display: flex;
  align-items: center;
  justify-content: center;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
  /* 交给我们自己处理拖动、双指缩放与滑动切换，避免触发页面滚动/浏览器缩放。 */
  touch-action: none;
  overscroll-behavior: contain;
}

.ax-viewer__stage.is-zoomed {
  cursor: grab;
}

.ax-viewer__stage.is-dragging {
  cursor: grabbing;
}

.ax-viewer__img {
  /* 不限制尺寸：默认按原始像素 1:1 渲染（100%），超出舞台时可拖拽平移。 */
  max-width: none;
  max-height: none;
  object-fit: contain;
  opacity: 0;
  user-select: none;
  transform-origin: center center;
  touch-action: none;
  -webkit-user-drag: none;
  transition: opacity var(--ax-duration) var(--ax-ease);
  will-change: transform;
}

.ax-viewer__error {
  position: absolute;
  color: rgba(255, 255, 255, 0.6);
  font-size: var(--ax-text-sm);
}

.ax-viewer__img.is-ready {
  opacity: 1;
}

.ax-viewer__nav {
  position: absolute;
  top: 50%;
  z-index: 2;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  padding: 0;
  color: #fff;
  background: rgba(0, 0, 0, 0.45);
  border: 1px solid rgba(255, 255, 255, 0.15);
  border-radius: var(--ax-radius-full);
  cursor: pointer;
  transform: translateY(-50%);
  transition: background-color var(--ax-duration-fast) var(--ax-ease);
}

.ax-viewer__nav:hover {
  background: rgba(0, 0, 0, 0.7);
}

.ax-viewer__nav.is-prev {
  left: 16px;
}

.ax-viewer__nav.is-next {
  right: 16px;
}

.ax-viewer__info {
  grid-area: info;
  overflow-y: auto;
  padding: var(--ax-space-4);
  background: rgba(255, 255, 255, 0.04);
  border-left: 1px solid rgba(255, 255, 255, 0.08);
}

/* 移动端摘要与抽屉，桌面端隐藏。 */
.ax-viewer__summary,
.ax-viewer__info-backdrop,
.ax-viewer__info-handle,
.ax-viewer__info-close {
  display: none;
}

.ax-viewer__info-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--ax-space-3);
  margin-bottom: var(--ax-space-3);
}

.ax-viewer__info-title {
  margin: 0;
  color: #fff;
  font-size: var(--ax-text-sm);
  font-weight: var(--ax-weight-semibold);
}

.ax-viewer__fields {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-3);
  margin: 0;
}

.ax-viewer__field {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.ax-viewer__field dt {
  color: rgba(255, 255, 255, 0.5);
  font-size: var(--ax-text-xs);
}

.ax-viewer__field dd {
  margin: 0;
  color: rgba(255, 255, 255, 0.9);
  font-size: var(--ax-text-sm);
  overflow-wrap: anywhere;
}

.ax-viewer__mono {
  font-family: var(--ax-font-mono, monospace);
  font-size: var(--ax-text-xs) !important;
}

.ax-viewer__link {
  color: var(--ax-accent-bright);
  cursor: pointer;
}

.ax-viewer__author-link,
.ax-viewer__source-link {
  color: var(--ax-accent-bright);
  text-decoration: none;
}

.ax-viewer__author-link:hover,
.ax-viewer__source-link:hover {
  text-decoration: underline;
}

.ax-viewer__handle {
  margin-left: 6px;
  color: var(--ax-accent-bright);
  opacity: 0.9;
}

.ax-viewer-enter-active,
.ax-viewer-leave-active {
  transition: opacity var(--ax-duration) var(--ax-ease);
}

.ax-viewer-enter-from,
.ax-viewer-leave-to {
  opacity: 0;
}

@media (max-width: 768px) {
  .ax-viewer {
    grid-template-columns: minmax(0, 1fr);
    grid-template-areas:
      'bar'
      'stage'
      'summary';
    grid-template-rows: auto minmax(0, 1fr) auto;
  }

  .ax-viewer__bar {
    gap: 6px 10px;
    padding: 8px 10px;
  }

  .ax-viewer__title {
    flex-basis: 100%;
  }

  .ax-viewer__tools {
    flex: 1 1 100%;
    flex-wrap: nowrap;
    justify-content: space-between;
    gap: 2px;
  }

  .ax-viewer__tools .el-button {
    margin-left: 0;
  }

  .ax-viewer__zoom {
    min-width: 38px;
  }

  .ax-viewer__nav {
    width: 38px;
    height: 38px;
  }

  .ax-viewer__nav.is-prev {
    left: 8px;
  }

  .ax-viewer__nav.is-next {
    right: 8px;
  }

  /* 默认只显示一行关键信息，点击后展开完整抽屉。 */
  .ax-viewer__summary {
    display: flex;
    grid-area: summary;
    align-items: center;
    gap: var(--ax-space-2);
    width: 100%;
    min-width: 0;
    padding: 10px 14px calc(10px + env(safe-area-inset-bottom, 0px));
    color: rgba(255, 255, 255, 0.82);
    font: inherit;
    font-size: var(--ax-text-xs);
    text-align: left;
    background: rgba(255, 255, 255, 0.06);
    border: 0;
    border-top: 1px solid rgba(255, 255, 255, 0.1);
    cursor: pointer;
  }

  .ax-viewer__summary:active {
    background: rgba(255, 255, 255, 0.1);
  }

  .ax-viewer__summary-icon {
    flex: none;
    color: var(--ax-accent-bright);
  }

  .ax-viewer__summary-text {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .ax-viewer__summary-arrow {
    flex: none;
    color: rgba(255, 255, 255, 0.5);
    transition: transform var(--ax-duration-fast) var(--ax-ease);
  }

  .ax-viewer__summary-arrow.is-open {
    transform: rotate(180deg);
  }

  .ax-viewer__info-backdrop {
    position: absolute;
    inset: 0;
    z-index: 7;
    display: block;
    background: rgba(0, 0, 0, 0.5);
  }

  .ax-viewer__info {
    position: absolute;
    right: 0;
    bottom: 0;
    left: 0;
    z-index: 8;
    max-height: min(70vh, 620px);
    padding: 10px var(--ax-space-4) calc(var(--ax-space-4) + env(safe-area-inset-bottom, 0px));
    overflow-y: auto;
    overscroll-behavior: contain;
    -webkit-overflow-scrolling: touch;
    visibility: hidden;
    background: #141519;
    border-top: 1px solid rgba(255, 255, 255, 0.12);
    border-left: none;
    border-radius: 18px 18px 0 0;
    box-shadow: 0 -16px 48px rgba(0, 0, 0, 0.5);
    transform: translateY(calc(100% + 16px));
    transition:
      transform 0.24s var(--ax-ease),
      visibility 0.24s;
    pointer-events: none;
  }

  .ax-viewer__info.is-open {
    visibility: visible;
    transform: translateY(0);
    pointer-events: auto;
  }

  .ax-viewer__info-handle {
    display: block;
    width: 38px;
    height: 4px;
    margin: 0 auto 10px;
    background: rgba(255, 255, 255, 0.24);
    border-radius: 999px;
  }

  .ax-viewer__info-close {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    padding: 0;
    color: rgba(255, 255, 255, 0.7);
    background: rgba(255, 255, 255, 0.08);
    border: 0;
    border-radius: 50%;
    cursor: pointer;
  }

  .ax-viewer__info-close:active {
    background: rgba(255, 255, 255, 0.16);
  }
}
</style>

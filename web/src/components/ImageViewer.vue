<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import {
  ArrowLeft,
  ArrowRight,
  Close,
  CopyDocument,
  FullScreen,
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
// 设备像素比。默认缩放取 1/dpr，使 1 图像 px = 1 物理像素。
const dpr = ref(typeof window !== 'undefined' && window.devicePixelRatio ? window.devicePixelRatio : 1)

const start = { x: 0, y: 0, ox: 0, oy: 0 }
const MIN_SCALE = 0.05
const MAX_SCALE = 8

const current = computed(() => props.items[index.value])
const hasPrev = computed(() => index.value > 0)
const hasNext = computed(() => index.value < props.items.length - 1)
const title = computed(
  () => current.value?.original_name || current.value?.filename || current.value?.key || '',
)
// 已保存的默认显示模式：fit=适应窗口，actual=原始像素 1:1。
const defaultMode = computed<'fit' | 'actual'>(() => auth.preferences?.viewer_mode ?? 'actual')

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

function onPointerDown(event: PointerEvent): void {
  // 交互控件（左右切换按钮等）不参与拖拽，也不捕获指针，否则 click 会被
  // 重定向到舞台，导致按钮失效并误触发关闭。
  if (!canPan.value) return
  if ((event.target as HTMLElement | null)?.closest('button')) return
  dragging.value = true
  moved = false
  start.x = event.clientX
  start.y = event.clientY
  start.ox = offsetX.value
  start.oy = offsetY.value
  ;(event.currentTarget as HTMLElement).setPointerCapture?.(event.pointerId)
}

function onPointerMove(event: PointerEvent): void {
  if (!dragging.value) return
  const dx = event.clientX - start.x
  const dy = event.clientY - start.y
  if (Math.abs(dx) + Math.abs(dy) > 4) moved = true
  offsetX.value = start.ox + dx
  offsetY.value = start.oy + dy
  clampOffset()
}

function onPointerUp(): void {
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
    ElMessage.success('已复制链接')
  } else {
    ElMessage.error('复制失败，请手动复制')
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
      close()
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

watch(open, async (value) => {
  document.body.style.overflow = value ? 'hidden' : ''
  if (value) {
    resetView()
    await nextTick()
    measureStage()
    setupObserver()
  } else {
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
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
  window.removeEventListener('resize', syncDpr)
  dprMedia?.removeEventListener('change', syncDpr)
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
            <el-button circle :icon="ZoomOut" title="缩小 (-)" @click="zoomBy(1 / 1.2)" />
            <span class="ax-viewer__zoom">{{ Math.round(scale * dpr * 100) }}%</span>
            <el-button circle :icon="ZoomIn" title="放大 (+)" @click="zoomBy(1.2)" />
            <el-button
              circle
              :icon="FullScreen"
              :type="defaultMode === 'fit' ? 'primary' : ''"
              title="适应窗口（点击设为默认）"
              @click="chooseFit"
            />
            <el-button
              circle
              :icon="RefreshLeft"
              :type="defaultMode === 'actual' ? 'primary' : ''"
              title="原始像素 100%（点击设为默认）"
              @click="chooseActual"
            />
            <el-button circle :icon="CopyDocument" title="复制链接" @click="copyLink" />
            <el-button circle :icon="Link" title="新窗口打开" @click="openOriginal" />
            <el-button circle :icon="Close" title="关闭 (Esc)" @click="close" />
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
            aria-label="上一张"
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
          <span v-if="failed" class="ax-viewer__error">图片加载失败</span>

          <button
            v-if="hasNext"
            class="ax-viewer__nav is-next"
            type="button"
            aria-label="下一张"
            @pointerdown.stop
            @dblclick.stop
            @click.stop="go(1)"
          >
            <el-icon :size="24"><ArrowRight /></el-icon>
          </button>
        </div>

        <aside class="ax-viewer__info">
          <h2 class="ax-viewer__info-title">图片信息</h2>
          <dl class="ax-viewer__fields">
            <div class="ax-viewer__field">
              <dt>名称</dt>
              <dd>{{ current.original_name || '—' }}</dd>
            </div>
            <div v-if="current.owner_username" class="ax-viewer__field">
              <dt>上传者</dt>
              <dd>{{ current.owner_username }}</dd>
            </div>
            <div class="ax-viewer__field">
              <dt>存储名</dt>
              <dd>{{ current.filename || current.key }}</dd>
            </div>
            <div class="ax-viewer__field">
              <dt>尺寸</dt>
              <dd>{{ formatDimensions(current.width, current.height) }}</dd>
            </div>
            <div class="ax-viewer__field">
              <dt>大小</dt>
              <dd>{{ formatBytes(current.size) }}</dd>
            </div>
            <div class="ax-viewer__field">
              <dt>格式</dt>
              <dd>{{ formatMime(current.mime_type) }}</dd>
            </div>
            <div class="ax-viewer__field">
              <dt>可见性</dt>
              <dd>{{ current.permission === 'public' ? '公开' : '私有' }}</dd>
            </div>
            <div class="ax-viewer__field">
              <dt>相册</dt>
              <dd>{{ props.albumName?.(current.album_id) || '未归入相册' }}</dd>
            </div>
            <div class="ax-viewer__field">
              <dt>上传时间</dt>
              <dd>{{ formatDateTime(current.created_at) }}</dd>
            </div>
            <div class="ax-viewer__field">
              <dt>哈希</dt>
              <dd class="ax-viewer__mono">{{ current.hash || '—' }}</dd>
            </div>
            <div class="ax-viewer__field">
              <dt>链接</dt>
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
  grid-template-columns: 1fr 320px;
  grid-template-rows: auto 1fr;
  grid-template-areas:
    'bar info'
    'stage info';
  color: #f5f5f5;
  background: rgba(8, 8, 10, 0.96);
  backdrop-filter: blur(4px);
}

.ax-viewer__bar {
  grid-area: bar;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--ax-space-4);
  padding: 10px 16px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}

.ax-viewer__title {
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
  flex: none;
  align-items: center;
  gap: var(--ax-space-2);
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
  overflow: hidden;
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

.ax-viewer__info-title {
  margin: 0 0 var(--ax-space-3);
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
    grid-template-columns: 1fr;
    grid-template-areas:
      'bar'
      'stage'
      'info';
    grid-template-rows: auto 1fr auto;
  }

  .ax-viewer__info {
    max-height: 38vh;
    border-top: 1px solid rgba(255, 255, 255, 0.08);
    border-left: none;
  }
}
</style>

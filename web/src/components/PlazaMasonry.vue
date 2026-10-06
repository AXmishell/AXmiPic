<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'

import { toApiError } from '@/api/client'
import { listPlaza } from '@/api/images'
import type { ImageItem } from '@/api/types'
import ImageViewer from '@/components/ImageViewer.vue'

const props = withDefaults(defineProps<{ pageSize?: number }>(), { pageSize: 30 })

const items = ref<ImageItem[]>([])
const columns = ref<ImageItem[][]>([])
const loading = ref(false)
const hasMore = ref(true)
const errorMessage = ref('')
const failedIds = ref<Set<string>>(new Set())
let cursor = ''
let requestSeq = 0

const containerRef = ref<HTMLElement | null>(null)
const sentinel = ref<HTMLElement | null>(null)
const columnCount = ref(4)

const viewerOpen = ref(false)
const viewerIndex = ref(0)

// ---- 瀑布流：按“最短列”分配，列高更均衡，顺序也更自然 ----
function aspectRatio(item: ImageItem): number {
  if (item.width > 0 && item.height > 0) return item.height / item.width
  return 0.75 // 未知尺寸时按 4:3 估算
}

function rebuildColumns(): void {
  const n = columnCount.value
  const buckets: ImageItem[][] = Array.from({ length: n }, () => [])
  const heights = new Array<number>(n).fill(0)
  for (const item of items.value) {
    let min = 0
    for (let i = 1; i < n; i += 1) {
      if (heights[i] < heights[min]) min = i
    }
    buckets[min].push(item)
    heights[min] += aspectRatio(item)
  }
  columns.value = buckets
}

function updateColumnCount(): void {
  const width = containerRef.value?.clientWidth ?? window.innerWidth
  const next = width < 560 ? 2 : width < 880 ? 3 : width < 1200 ? 4 : 5
  if (next !== columnCount.value) {
    columnCount.value = next
    rebuildColumns()
  }
}

// ---- 游标无限滚动 ----
async function loadMore(): Promise<void> {
  if (loading.value || !hasMore.value) return
  const seq = ++requestSeq
  loading.value = true
  errorMessage.value = ''
  try {
    const data = await listPlaza({
      page: 1,
      pageSize: props.pageSize,
      order: 'newest',
      cursor: cursor || undefined,
    })
    if (seq !== requestSeq) return
    const batch = data.items ?? []
    items.value = cursor ? [...items.value, ...batch] : batch
    cursor = data.next_cursor ?? ''
    hasMore.value = Boolean(cursor) && batch.length > 0
    rebuildColumns()
  } catch (error) {
    if (seq !== requestSeq) return
    errorMessage.value = toApiError(error).message
    hasMore.value = false
  } finally {
    if (seq === requestSeq) loading.value = false
  }
}

function retry(): void {
  hasMore.value = true
  void loadMore()
}

function onPreviewError(id: string): void {
  failedIds.value.add(id)
}

function openViewer(item: ImageItem): void {
  const idx = items.value.findIndex((i) => i.id === item.id)
  viewerIndex.value = idx < 0 ? 0 : idx
  viewerOpen.value = true
}

let resizeObserver: ResizeObserver | null = null
let sentinelObserver: IntersectionObserver | null = null

onMounted(async () => {
  await nextTick()
  updateColumnCount()
  if (containerRef.value) {
    resizeObserver = new ResizeObserver(() => updateColumnCount())
    resizeObserver.observe(containerRef.value)
  }
  if (sentinel.value) {
    sentinelObserver = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) void loadMore()
      },
      { rootMargin: '600px 0px' },
    )
    sentinelObserver.observe(sentinel.value)
  }
  void loadMore()
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  sentinelObserver?.disconnect()
})
</script>

<template>
  <div ref="containerRef" class="pm">
    <el-alert
      v-if="errorMessage"
      class="pm__error"
      type="error"
      :closable="false"
      show-icon
      :title="errorMessage"
    >
      <el-button link type="primary" @click="retry">重试</el-button>
    </el-alert>

    <div v-if="items.length === 0 && loading" class="pm__masonry">
      <div v-for="c in columnCount" :key="c" class="pm__col">
        <div
          v-for="i in 4"
          :key="i"
          class="pm__skeleton"
          :style="{ height: `${140 + ((i * 47 + c * 31) % 120)}px` }"
        />
      </div>
    </div>

    <p v-else-if="items.length === 0 && !loading" class="pm__empty">暂无公开图片</p>

    <div v-else class="pm__masonry">
      <div v-for="(col, ci) in columns" :key="ci" class="pm__col">
        <article v-for="item in col" :key="item.id" class="pm__card" @click="openViewer(item)">
          <div
            class="pm__thumb"
            :style="{
              aspectRatio:
                item.width > 0 && item.height > 0 ? `${item.width} / ${item.height}` : '4 / 3',
            }"
          >
            <img
              v-if="!failedIds.has(item.id)"
              class="pm__img"
              :src="item.thumbnail || item.url"
              loading="lazy"
              :alt="item.original_name || item.key"
              @error="onPreviewError(item.id)"
            />
            <span v-else class="pm__fallback">预览不可用</span>

            <div class="pm__overlay">
              <p class="pm__name" :title="item.original_name || item.filename || item.key">
                {{ item.original_name || item.filename || item.key }}
              </p>
              <p class="pm__author">上传者：{{ item.owner_username || '匿名' }}</p>
            </div>
          </div>
        </article>
      </div>
    </div>

    <div ref="sentinel" class="pm__sentinel" aria-hidden="true">
      <span v-if="loading && items.length > 0">正在加载更多…</span>
      <span v-else-if="!hasMore && items.length > 0">已经到底啦</span>
    </div>

    <ImageViewer v-model="viewerOpen" v-model:index="viewerIndex" :items="items" />
  </div>
</template>

<style scoped>
/* 瀑布流：每列一个 flex 列，卡片按原图比例预留高度，加载后不跳动 */
.pm__masonry {
  display: flex;
  align-items: flex-start;
  gap: var(--ax-space-3);
}

.pm__col {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--ax-space-3);
  min-width: 0;
}

.pm__card {
  position: relative;
  overflow: hidden;
  background: var(--ax-tint-weak);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-lg);
  cursor: zoom-in;
  transition: border-color var(--ax-duration) var(--ax-ease);
}

.pm__card:hover {
  border-color: var(--ax-border);
}

.pm__thumb {
  position: relative;
  display: block;
  overflow: hidden;
  background: var(--ax-tint);
}

.pm__img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
  transition: transform var(--ax-duration-slow) var(--ax-ease);
}

.pm__card:hover .pm__img {
  transform: scale(1.03);
}

.pm__fallback {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.pm__overlay {
  position: absolute;
  inset: 0;
  z-index: 1;
  display: flex;
  flex-direction: column;
  justify-content: flex-end;
  gap: 2px;
  padding: var(--ax-space-3);
  color: #fff;
  background: linear-gradient(to bottom, rgba(0, 0, 0, 0) 45%, rgba(0, 0, 0, 0.78) 100%);
  opacity: 0;
  pointer-events: none;
  transition: opacity var(--ax-duration) var(--ax-ease);
}

.pm__card:hover .pm__overlay,
.pm__card:focus-within .pm__overlay {
  opacity: 1;
}

.pm__name {
  margin: 0;
  overflow: hidden;
  color: #fff;
  font-size: var(--ax-text-sm);
  font-weight: var(--ax-weight-medium);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pm__author {
  margin: 0;
  overflow: hidden;
  color: rgba(255, 255, 255, 0.75);
  font-size: var(--ax-text-xs);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pm__error {
  margin-bottom: var(--ax-space-4);
}

.pm__skeleton {
  background: var(--ax-tint-weak);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-lg);
  animation: pm-pulse 1.4s ease-in-out infinite;
}

@keyframes pm-pulse {
  0%,
  100% {
    opacity: 0.55;
  }
  50% {
    opacity: 0.9;
  }
}

.pm__empty {
  margin: 0;
  padding: var(--ax-space-10) 0;
  color: var(--ax-text-4);
  font-size: var(--ax-text-sm);
  text-align: center;
}

.pm__sentinel {
  display: flex;
  justify-content: center;
  min-height: 32px;
  padding: var(--ax-space-4) 0;
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}
</style>

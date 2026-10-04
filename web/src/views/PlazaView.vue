<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { ArrowDown, CopyDocument, Picture, Refresh, Search } from '@element-plus/icons-vue'

import { toApiError } from '@/api/client'
import { listPlaza, type ImageOrder } from '@/api/images'
import type { ImageItem } from '@/api/types'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'
import PageHeader from '@/components/PageHeader.vue'
import { copyText } from '@/utils/clipboard'
import { formatBytes, formatDateTime, formatDimensions, formatMime, formatNumber } from '@/utils/format'

const items = ref<ImageItem[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(24)
const loading = ref(false)
const errorMessage = ref('')
const failedIds = ref<Set<string>>(new Set())

const order = ref<ImageOrder>('newest')
const keyword = ref('')
const orderOptions: { value: ImageOrder; label: string }[] = [
  { value: 'newest', label: '最新' },
  { value: 'earliest', label: '最早' },
  { value: 'largest', label: '最大' },
  { value: 'smallest', label: '最小' },
]

let requestSeq = 0

async function load(): Promise<void> {
  const seq = ++requestSeq
  loading.value = true
  errorMessage.value = ''
  try {
    const data = await listPlaza({
      page: page.value,
      pageSize: pageSize.value,
      order: order.value,
      keyword: keyword.value.trim(),
    })
    if (seq !== requestSeq) return
    items.value = data.items ?? []
    total.value = data.total ?? 0
    if (items.value.length === 0 && page.value > 1) {
      page.value -= 1
      await load()
    }
  } catch (error) {
    if (seq !== requestSeq) return
    errorMessage.value = toApiError(error).message
  } finally {
    if (seq === requestSeq) loading.value = false
  }
}

function search(): void {
  page.value = 1
  void load()
}

function setOrder(value: ImageOrder): void {
  order.value = value
  page.value = 1
  void load()
}

function handleSizeChange(): void {
  page.value = 1
  void load()
}

function onPreviewError(id: string): void {
  failedIds.value.add(id)
}

async function copyLink(item: ImageItem): Promise<void> {
  const copied = await copyText(item.url)
  if (copied) {
    ElMessage.success('已复制')
  } else {
    ElMessage.error('复制失败，请手动复制')
  }
}

onMounted(load)
</script>

<template>
  <div class="ax-page">
    <PageHeader title="图片广场" description="所有用户公开分享的图片。">
      <template #actions>
        <el-input
          v-model="keyword"
          class="plaza-search"
          placeholder="搜索文件名"
          clearable
          :prefix-icon="Search"
          @keyup.enter="search"
          @clear="search"
        />
        <el-dropdown trigger="click" @command="setOrder">
          <el-button>
            {{ orderOptions.find((o) => o.value === order)?.label }}
            <el-icon class="plaza-caret"><ArrowDown /></el-icon>
          </el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item
                v-for="option in orderOptions"
                :key="option.value"
                :command="option.value"
              >
                {{ option.label }}
              </el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="load" />

    <div v-else-if="loading && items.length === 0" class="ax-grid" aria-busy="true">
      <div v-for="index in 6" :key="index" class="ax-card">
        <el-skeleton animated>
          <template #template>
            <el-skeleton-item variant="image" style="width: 100%; height: 150px" />
            <div style="padding: 12px">
              <el-skeleton-item variant="text" />
            </div>
          </template>
        </el-skeleton>
      </div>
    </div>

    <EmptyState
      v-else-if="items.length === 0"
      title="广场上还没有公开图片"
      description="在图片管理页把图片设为「公开」，它就会出现在这里。"
    />

    <template v-else>
      <p class="plaza-count">共 {{ formatNumber(total) }} 张公开图片</p>
      <section class="ax-grid plaza-grid" :aria-busy="loading" aria-label="图片广场">
        <article v-for="item in items" :key="item.id" class="plaza-card">
          <div class="plaza-card__preview">
            <el-image
              v-if="!failedIds.has(item.id)"
              class="plaza-card__img"
              :src="item.url"
              :preview-src-list="items.map((i) => i.url)"
              :initial-index="items.findIndex((i) => i.id === item.id)"
              fit="cover"
              loading="lazy"
              :alt="item.original_name || item.key"
              @error="onPreviewError(item.id)"
            />
            <span v-else class="plaza-card__fallback">
              <el-icon :size="20"><Picture /></el-icon>
              <span>预览不可用</span>
            </span>
          </div>
          <div class="plaza-card__body">
            <p class="plaza-card__name" :title="item.original_name || item.filename || item.key">
              {{ item.original_name || item.filename || item.key }}
            </p>
            <p class="plaza-card__meta">
              {{ formatMime(item.mime_type) }} · {{ formatBytes(item.size) }} ·
              {{ formatDimensions(item.width, item.height) }}
            </p>
            <p class="plaza-card__meta">作者：{{ item.owner_username || '匿名' }}</p>
            <p class="plaza-card__meta">{{ formatDateTime(item.created_at) }}</p>
            <el-button size="small" :icon="CopyDocument" @click="copyLink(item)">复制链接</el-button>
          </div>
        </article>
      </section>

      <div class="plaza-pager">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :total="total"
          :page-sizes="[12, 24, 48]"
          background
          layout="total, sizes, prev, pager, next"
          @current-change="load"
          @size-change="handleSizeChange"
        />
      </div>
    </template>
  </div>
</template>

<style scoped>
.plaza-search {
  width: 200px;
}

.plaza-caret {
  margin-left: 4px;
}

.plaza-count {
  margin-bottom: var(--ax-space-3);
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
}

.plaza-grid {
  grid-template-columns: repeat(auto-fill, minmax(min(220px, 100%), 1fr));
}

.plaza-card {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: var(--ax-tint-weak);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-lg);
}

.plaza-card__preview {
  position: relative;
  aspect-ratio: 16 / 10;
  overflow: hidden;
  background: var(--ax-checker-b);
  border-bottom: 1px solid var(--ax-border-subtle);
}

.plaza-card__img {
  width: 100%;
  height: 100%;
}

.plaza-card__fallback {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--ax-space-2);
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.plaza-card__body {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-2);
  padding: var(--ax-space-3) var(--ax-space-4) var(--ax-space-4);
}

.plaza-card__name {
  display: -webkit-box;
  overflow: hidden;
  color: var(--ax-text);
  font-size: var(--ax-text-sm);
  font-weight: var(--ax-weight-medium);
  overflow-wrap: anywhere;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.plaza-card__meta {
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.plaza-pager {
  display: flex;
  justify-content: flex-end;
  margin-top: var(--ax-space-5);
}

@media (max-width: 768px) {
  .plaza-search {
    width: 100%;
  }

  .plaza-pager {
    justify-content: center;
  }
}
</style>

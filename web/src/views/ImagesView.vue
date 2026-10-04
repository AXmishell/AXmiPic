<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { CopyDocument, Delete, Picture, Refresh } from '@element-plus/icons-vue'

import { toApiError } from '@/api/client'
import { deleteImage, listImages } from '@/api/images'
import type { ImageItem } from '@/api/types'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'
import PageHeader from '@/components/PageHeader.vue'
import { copyText } from '@/utils/clipboard'
import {
  formatBytes,
  formatDateTime,
  formatDimensions,
  formatMime,
  formatNumber,
} from '@/utils/format'

const items = ref<ImageItem[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(24)
const loading = ref(false)
const errorMessage = ref('')
const deletingId = ref<string | null>(null)
const failedIds = ref<Set<string>>(new Set())

let requestSeq = 0

const description = computed(() =>
  loading.value && items.value.length === 0
    ? '正在加载图片列表'
    : `共 ${formatNumber(total.value)} 张图片`,
)

async function load(): Promise<void> {
  const seq = ++requestSeq
  loading.value = true
  errorMessage.value = ''
  try {
    const data = await listImages(page.value, pageSize.value)
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
    ElMessage.success('链接已复制')
  } else {
    ElMessage.error('复制失败，请手动复制')
  }
}

async function confirmDelete(item: ImageItem): Promise<void> {
  try {
    await ElMessageBox.confirm(
      `图片「${item.key}」将被永久删除，该操作不可恢复。`,
      '删除图片',
      {
        confirmButtonText: '删除',
        cancelButtonText: '取消',
        type: 'warning',
        confirmButtonClass: 'el-button--danger',
      },
    )
  } catch {
    return
  }

  deletingId.value = item.id
  try {
    await deleteImage(item.id)
    ElMessage.success('图片已删除')
    if (items.value.length === 1 && page.value > 1) {
      page.value -= 1
    }
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    deletingId.value = null
  }
}

onMounted(load)
</script>

<template>
  <div class="ax-page">
    <PageHeader title="图片管理" :description="description">
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="load" />

    <div v-else-if="loading && items.length === 0" class="ax-grid" aria-busy="true">
      <div v-for="index in 6" :key="index" class="ax-card">
        <div class="image-skeleton">
          <el-skeleton animated>
            <template #template>
              <el-skeleton-item variant="image" class="image-skeleton__preview" />
              <div class="image-skeleton__rows">
                <el-skeleton-item variant="text" />
                <el-skeleton-item variant="text" style="width: 62%" />
              </div>
            </template>
          </el-skeleton>
        </div>
      </div>
    </div>

    <EmptyState
      v-else-if="items.length === 0"
      title="还没有上传图片"
      description="通过 API 使用访问令牌上传图片后，它们会显示在这里。"
    />

    <template v-else>
      <section class="ax-grid image-grid" :aria-busy="loading" aria-label="图片列表">
        <article v-for="item in items" :key="item.id" class="image-card">
          <a
            class="image-card__preview"
            :href="item.url"
            target="_blank"
            rel="noopener noreferrer"
            :aria-label="`在新窗口打开 ${item.key}`"
          >
            <img
              v-if="!failedIds.has(item.id)"
              class="image-card__img"
              :src="item.url"
              :alt="item.key"
              loading="lazy"
              decoding="async"
              @error="onPreviewError(item.id)"
            />
            <span v-else class="image-card__fallback">
              <el-icon :size="20"><Picture /></el-icon>
              <span>预览不可用</span>
            </span>
          </a>

          <div class="image-card__body">
            <p class="image-card__key" :title="item.key">{{ item.key }}</p>

            <dl class="image-card__meta">
              <div class="image-card__meta-row">
                <dt>大小</dt>
                <dd>{{ formatBytes(item.size) }}</dd>
              </div>
              <div class="image-card__meta-row">
                <dt>尺寸</dt>
                <dd>{{ formatDimensions(item.width, item.height) }}</dd>
              </div>
              <div class="image-card__meta-row">
                <dt>格式</dt>
                <dd>{{ formatMime(item.mime_type) }}</dd>
              </div>
              <div class="image-card__meta-row">
                <dt>上传时间</dt>
                <dd>{{ formatDateTime(item.created_at) }}</dd>
              </div>
            </dl>

            <div class="image-card__actions">
              <el-button size="small" :icon="CopyDocument" @click="copyLink(item)">
                复制链接
              </el-button>
              <el-button
                size="small"
                type="danger"
                plain
                :icon="Delete"
                :loading="deletingId === item.id"
                @click="confirmDelete(item)"
              >
                删除
              </el-button>
            </div>
          </div>
        </article>
      </section>

      <div class="image-pager">
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
.image-grid {
  grid-template-columns: repeat(auto-fill, minmax(min(250px, 100%), 1fr));
}

.image-card {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: rgba(255, 255, 255, 0.02);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-lg);
  transition:
    background-color var(--ax-duration) var(--ax-ease),
    border-color var(--ax-duration) var(--ax-ease);
}

.image-card:hover {
  background: rgba(255, 255, 255, 0.03);
  border-color: var(--ax-border);
}

.image-card__preview {
  position: relative;
  display: block;
  aspect-ratio: 16 / 10;
  overflow: hidden;
  background:
    linear-gradient(45deg, rgba(255, 255, 255, 0.02) 25%, transparent 25%) 0 0 / 16px 16px,
    linear-gradient(-45deg, rgba(255, 255, 255, 0.02) 25%, transparent 25%) 0 8px / 16px 16px,
    #0c0d0f;
  border-bottom: 1px solid var(--ax-border-subtle);
}

.image-card__img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  transform: scale(1);
  transition: transform var(--ax-duration-slow) var(--ax-ease);
}

.image-card:hover .image-card__img {
  transform: scale(1.03);
}

.image-card__fallback {
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

.image-card__body {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-3);
  padding: var(--ax-space-3) var(--ax-space-4) var(--ax-space-4);
}

.image-card__key {
  display: -webkit-box;
  overflow: hidden;
  color: var(--ax-text);
  font-size: var(--ax-text-sm);
  font-weight: var(--ax-weight-medium);
  overflow-wrap: anywhere;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.image-card__meta {
  display: grid;
  gap: 5px;
  margin: 0;
}

.image-card__meta-row {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--ax-space-3);
  min-width: 0;
}

.image-card__meta-row dt {
  flex: none;
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.image-card__meta-row dd {
  margin: 0;
  overflow: hidden;
  color: var(--ax-text-2);
  font-size: var(--ax-text-xs);
  font-variant-numeric: tabular-nums;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.image-card__actions {
  display: flex;
  gap: var(--ax-space-2);
  margin-top: var(--ax-space-1);
}

.image-card__actions :deep(.el-button) {
  flex: 1;
  margin-left: 0;
}

.image-pager {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  margin-top: var(--ax-space-5);
}

.image-skeleton__preview {
  width: 100%;
  height: 150px;
}

.image-skeleton__rows {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-2);
  padding: var(--ax-space-3) var(--ax-space-4) var(--ax-space-4);
}

@media (max-width: 768px) {
  .image-pager {
    justify-content: center;
  }
}
</style>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { ArrowDown, CopyDocument, Grid, Histogram, Refresh, Search } from '@element-plus/icons-vue'

import { toApiError } from '@/api/client'
import { listPlaza, type ImageOrder } from '@/api/images'
import type { ImageItem } from '@/api/types'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'
import ImageViewer from '@/components/ImageViewer.vue'
import PageHeader from '@/components/PageHeader.vue'
import PlazaCard from '@/components/PlazaCard.vue'
import { useAuthStore } from '@/stores/auth'
import { copyText } from '@/utils/clipboard'
import { formatNumber } from '@/utils/format'
import { buildMasonryColumns, masonryColumnCount } from '@/utils/masonry'

const auth = useAuthStore()
const { t } = useI18n()

const items = ref<ImageItem[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(24)
const loading = ref(false)
const errorMessage = ref('')
const failedIds = ref<Set<string>>(new Set())

const order = ref<ImageOrder>('newest')
const keyword = ref('')
const orderOptions = computed<{ value: ImageOrder; label: string }[]>(() => [
  { value: 'newest', label: t('plaza.order.newest') },
  { value: 'earliest', label: t('plaza.order.earliest') },
  { value: 'largest', label: t('plaza.order.largest') },
  { value: 'smallest', label: t('plaza.order.smallest') },
])

// 布局：来自用户偏好（跨设备），默认网格。
const layout = computed<'grid' | 'masonry'>(() =>
  auth.preferences?.plaza_layout === 'masonry' ? 'masonry' : 'grid',
)

const masonryRef = ref<HTMLElement | null>(null)
const columnCount = ref(4)
const columns = ref<ImageItem[][]>([])
let resizeObserver: ResizeObserver | null = null

function rebuildColumns(): void {
  columns.value = buildMasonryColumns(items.value, columnCount.value)
}

function updateColumnCount(): void {
  const width = masonryRef.value?.clientWidth ?? window.innerWidth
  const next = masonryColumnCount(width)
  if (next !== columnCount.value) {
    columnCount.value = next
    rebuildColumns()
  }
}

function observeMasonry(): void {
  if (resizeObserver || !masonryRef.value) return
  resizeObserver = new ResizeObserver(() => updateColumnCount())
  resizeObserver.observe(masonryRef.value)
}

function teardownMasonry(): void {
  resizeObserver?.disconnect()
  resizeObserver = null
}

function toggleLayout(): void {
  const next = layout.value === 'masonry' ? 'grid' : 'masonry'
  void auth.savePreferences({ plaza_layout: next })
}

// 全屏查看器
const viewerOpen = ref(false)
const viewerIndex = ref(0)

function openViewer(item: ImageItem): void {
  const idx = items.value.findIndex((i) => i.id === item.id)
  viewerIndex.value = idx < 0 ? 0 : idx
  viewerOpen.value = true
}

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

function clearKeyword(): void {
  keyword.value = ''
  search()
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
    ElMessage.success(t('plaza.messages.copied'))
  } else {
    ElMessage.error(t('plaza.messages.copyFailed'))
  }
}

watch(layout, async (value) => {
  if (value === 'masonry') {
    await nextTick()
    updateColumnCount()
    rebuildColumns()
    observeMasonry()
  } else {
    teardownMasonry()
  }
})

watch(items, () => {
  if (layout.value === 'masonry') rebuildColumns()
})

onMounted(async () => {
  if (!auth.preferences) void auth.loadPreferences()
  await load()
  if (layout.value === 'masonry') {
    await nextTick()
    updateColumnCount()
    rebuildColumns()
    observeMasonry()
  }
})

onBeforeUnmount(teardownMasonry)
</script>

<template>
  <div class="ax-page">
    <PageHeader :title="t('plaza.title')" :description="t('plaza.description')">
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
      </template>
      <template #toolbar>
        <el-input
          v-model="keyword"
          class="plaza-search"
          :placeholder="t('plaza.searchPlaceholder')"
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
        <el-button
          :icon="layout === 'masonry' ? Grid : Histogram"
          :title="
            layout === 'masonry' ? t('plaza.layout.switchToGrid') : t('plaza.layout.switchToMasonry')
          "
          @click="toggleLayout"
        >
          {{ layout === 'masonry' ? t('plaza.layout.grid') : t('plaza.layout.masonry') }}
        </el-button>
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
      :title="keyword.trim() ? t('plaza.empty.noMatchTitle') : t('plaza.empty.noItemsTitle')"
      :description="
        keyword.trim()
          ? t('plaza.empty.noMatchDescription')
          : t('plaza.empty.noItemsDescription')
      "
    >
      <el-button v-if="keyword.trim()" @click="clearKeyword">{{ t('plaza.empty.clearSearch') }}</el-button>
    </EmptyState>

    <template v-else>
      <p class="plaza-count">{{ t('plaza.count', { count: formatNumber(total) }) }}</p>

      <section
        v-if="layout === 'grid'"
        class="ax-grid plaza-grid"
        :aria-busy="loading"
        :aria-label="t('plaza.listLabel')"
      >
        <PlazaCard
          v-for="item in items"
          :key="item.id"
          :item="item"
          :failed="failedIds.has(item.id)"
          @open="openViewer"
          @error="onPreviewError"
        >
          <template #actions="{ item: target }">
            <el-button size="small" :icon="CopyDocument" @click="copyLink(target)">
              {{ t('plaza.actions.copyLink') }}
            </el-button>
          </template>
        </PlazaCard>
      </section>

      <div
        v-else
        ref="masonryRef"
        class="plaza-masonry"
        :aria-busy="loading"
        :aria-label="t('plaza.listLabel')"
      >
        <div v-for="(col, ci) in columns" :key="ci" class="plaza-masonry__col">
          <PlazaCard
            v-for="item in col"
            :key="item.id"
            :item="item"
            masonry
            :failed="failedIds.has(item.id)"
            @open="openViewer"
            @error="onPreviewError"
          >
            <template #actions="{ item: target }">
              <el-button size="small" :icon="CopyDocument" @click="copyLink(target)">
                {{ t('plaza.actions.copyLink') }}
              </el-button>
            </template>
          </PlazaCard>
        </div>
      </div>

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

    <!-- 全屏查看器（与图片管理共用） -->
    <ImageViewer v-model="viewerOpen" v-model:index="viewerIndex" :items="items" />
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

.plaza-masonry {
  display: flex;
  align-items: flex-start;
  gap: var(--ax-space-3);
}

.plaza-masonry__col {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--ax-space-3);
  min-width: 0;
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

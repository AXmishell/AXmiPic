<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { UploadRawFile } from 'element-plus'
import {
  ArrowDown,
  CopyDocument,
  Delete,
  EditPen,
  Folder,
  InfoFilled,
  Lock,
  Picture,
  Refresh,
  Search,
  Select,
  Unlock,
  Upload,
} from '@element-plus/icons-vue'

import { listAlbums } from '@/api/albums'
import { toApiError } from '@/api/client'
import {
  batchUpdateImages,
  deleteImage,
  listImages,
  renameImage,
  uploadImage,
  type ImageOrder,
} from '@/api/images'
import type { Album, ImageItem, ImagePermission } from '@/api/types'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'
import PageHeader from '@/components/PageHeader.vue'
import { useAuthStore } from '@/stores/auth'
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
const failedIds = ref<Set<string>>(new Set())
const auth = useAuthStore()

// 查询条件
const route = useRoute()
const order = ref<ImageOrder>('newest')
const keyword = ref('')
const albumFilter = ref('')
const permissionFilter = ref<'' | ImagePermission>('')
const albums = ref<Album[]>([])
const orderOptions: { value: ImageOrder; label: string }[] = [
  { value: 'newest', label: '最新' },
  { value: 'earliest', label: '最早' },
  { value: 'largest', label: '最大' },
  { value: 'smallest', label: '最小' },
]
const permissionOptions: { value: '' | ImagePermission; label: string }[] = [
  { value: '', label: '全部可见性' },
  { value: 'public', label: '公开' },
  { value: 'private', label: '私有' },
]

// 归入相册
const assignOpen = ref(false)
const assignAlbumId = ref('')
const assignIds = ref<string[]>([])
const assigning = ref(false)

// 上传
type UploadStatus = 'pending' | 'uploading' | 'success' | 'error'
interface UploadTask {
  id: string
  name: string
  size: number
  status: UploadStatus
  progress: number
  error?: string
}
const uploadRef = ref<{ clearFiles: () => void } | null>(null)
const dragActive = ref(false)
const uploadTasks = ref<UploadTask[]>([])
const uploadPanelOpen = ref(true)
const uploadRunning = ref(false)
const ACCEPT = 'image/jpeg,image/png,image/gif,image/webp'
const MAX_SIZE_MB = 20
const UPLOAD_CONCURRENCY = 3

// 多选
const selectedIds = ref<Set<string>>(new Set())

// 重命名
const renameOpen = ref(false)
const renameTarget = ref<ImageItem | null>(null)
const renameName = ref('')
const renaming = ref(false)

// 详情抽屉
const detailOpen = ref(false)
const detailTarget = ref<ImageItem | null>(null)

// 右键菜单
const menuOpen = ref(false)
const menuX = ref(0)
const menuY = ref(0)
const menuTarget = ref<ImageItem | null>(null)

let requestSeq = 0

const selectedCount = computed(() => selectedIds.value.size)
const previewList = computed(() => items.value.map((i) => i.url))

const description = computed(() =>
  loading.value && items.value.length === 0
    ? '正在加载图片列表'
    : `共 ${formatNumber(total.value)} 张图片`,
)

const uploadOverall = computed(() => {
  const list = uploadTasks.value
  if (list.length === 0) return 0
  const sum = list.reduce((acc, task) => {
    if (task.status === 'success' || task.status === 'error') return acc + 100
    return acc + task.progress
  }, 0)
  return Math.round(sum / list.length)
})

const uploadFinished = computed(
  () => uploadTasks.value.filter((t) => t.status === 'success' || t.status === 'error').length,
)

function uploadStatusLabel(task: UploadTask): string {
  switch (task.status) {
    case 'pending':
      return '等待中'
    case 'uploading':
      return `${task.progress}%`
    case 'success':
      return '完成'
    default:
      return task.error ? '失败' : '失败'
  }
}

function clearUploads(): void {
  if (uploadRunning.value) return
  uploadTasks.value = []
}

async function load(): Promise<void> {
  const seq = ++requestSeq
  loading.value = true
  errorMessage.value = ''
  try {
    const data = await listImages({
      page: page.value,
      pageSize: pageSize.value,
      order: order.value,
      keyword: keyword.value.trim(),
      albumId: albumFilter.value || undefined,
      permission: permissionFilter.value || undefined,
    })
    if (seq !== requestSeq) return
    items.value = data.items ?? []
    total.value = data.total ?? 0
    // 清理已不在当前页的选中项。
    selectedIds.value = new Set(
      [...selectedIds.value].filter((id) => items.value.some((i) => i.id === id)),
    )
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

/** 加载相册列表，供过滤与归档操作使用。 */
async function loadAlbums(): Promise<void> {
  try {
    albums.value = (await listAlbums()) ?? []
  } catch {
    // 相册加载失败不影响图片列表本身。
  }
}

function setAlbumFilter(value: string): void {
  albumFilter.value = value
  page.value = 1
  void load()
}

function setPermissionFilter(value: '' | ImagePermission): void {
  permissionFilter.value = value
  page.value = 1
  void load()
}

function albumName(id?: string): string {
  if (!id) return ''
  return albums.value.find((a) => a.id === id)?.name ?? ''
}

/** 批量设置可见性。 */
async function applyPermission(ids: string[], permission: ImagePermission): Promise<void> {
  if (ids.length === 0) return
  try {
    await batchUpdateImages({ ids, permission })
    ElMessage.success(permission === 'public' ? '已设为公开' : '已设为私有')
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  }
}

/** 打开「归入相册」对话框。 */
function openAssign(ids: string[], current?: string): void {
  if (ids.length === 0) return
  assignIds.value = [...ids]
  assignAlbumId.value = current ?? ''
  assignOpen.value = true
}

async function submitAssign(): Promise<void> {
  if (assigning.value || assignIds.value.length === 0) return
  assigning.value = true
  try {
    await batchUpdateImages(
      assignAlbumId.value
        ? { ids: assignIds.value, albumId: assignAlbumId.value }
        : { ids: assignIds.value, clearAlbum: true },
    )
    ElMessage.success('已更新所属相册')
    assignOpen.value = false
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    assigning.value = false
  }
}

function onPreviewError(id: string): void {
  failedIds.value.add(id)
}

// ---- 多选 ----
function isSelected(id: string): boolean {
  return selectedIds.value.has(id)
}

function toggleSelect(id: string): void {
  const next = new Set(selectedIds.value)
  if (next.has(id)) {
    next.delete(id)
  } else {
    next.add(id)
  }
  selectedIds.value = next
}

function selectAllVisible(): void {
  selectedIds.value = new Set(items.value.map((i) => i.id))
}

function clearSelection(): void {
  selectedIds.value = new Set()
}

// ---- 上传（多文件 / 拖拽 / 粘贴）----
function validateFile(file: { size: number; type: string; name: string }): string | null {
  if (!ACCEPT.split(',').includes(file.type)) {
    return `不支持的格式：${file.name}`
  }
  if (file.size > MAX_SIZE_MB * 1024 * 1024) {
    return `超出 ${MAX_SIZE_MB} MB 上限：${file.name}`
  }
  return null
}

function newTaskId(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
    return crypto.randomUUID()
  }
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`
}

function beforeUpload(file: UploadRawFile): boolean {
  const reason = validateFile(file)
  if (reason) {
    ElMessage.error(reason)
    return false
  }
  return true
}

/**
 * 把一批文件加入上传队列，并以受限并发上传。每个文件都有独立的状态与进度，
 * 单个文件失败不会中断其余文件。
 */
async function uploadFiles(files: File[]): Promise<void> {
  if (files.length === 0) return
  let candidates = files
  if (candidates.length > 1 && !auth.hasFeature('batch_upload')) {
    ElMessage.warning('当前角色未开启批量上传，仅上传第一张')
    candidates = candidates.slice(0, 1)
  }

  const jobs: { file: File; task: UploadTask }[] = []
  for (const file of candidates) {
    const reason = validateFile(file)
    if (reason) {
      ElMessage.error(reason)
      continue
    }
    const task = reactive<UploadTask>({
      id: newTaskId(),
      name: file.name,
      size: file.size,
      status: 'pending',
      progress: 0,
    })
    jobs.push({ file, task })
  }
  if (jobs.length === 0) return

  uploadTasks.value = [...jobs.map((j) => j.task), ...uploadTasks.value].slice(0, 100)
  uploadPanelOpen.value = true
  uploadRunning.value = true
  uploadRef.value?.clearFiles()

  let cursor = 0
  let ok = 0
  let failed = 0
  const worker = async (): Promise<void> => {
    for (;;) {
      const index = cursor++
      const job = jobs[index]
      if (!job) return
      job.task.status = 'uploading'
      job.task.progress = 0
      try {
        await uploadImage(job.file, (percent) => {
          job.task.progress = percent
        })
        job.task.status = 'success'
        job.task.progress = 100
        ok += 1
      } catch (error) {
        job.task.status = 'error'
        job.task.error = toApiError(error).message
        failed += 1
      }
    }
  }

  const workers = Math.min(UPLOAD_CONCURRENCY, jobs.length)
  await Promise.all(Array.from({ length: workers }, worker))
  uploadRunning.value = false

  if (ok > 0) {
    ElMessage.success(failed > 0 ? `已上传 ${ok} 张，${failed} 张失败` : `已上传 ${ok} 张图片`)
    page.value = 1
    await load()
  } else if (failed > 0) {
    ElMessage.error('上传失败')
  }
}

/** el-upload 自定义上传：接入统一的上传队列。 */
async function handleUpload(options: { file: File }): Promise<void> {
  await uploadFiles([options.file])
}

function onPaste(event: ClipboardEvent): void {
  if (!auth.hasFeature('paste_upload')) return
  const files: File[] = []
  const list = event.clipboardData?.items
  if (!list) return
  for (const item of list) {
    if (item.kind === 'file' && item.type.startsWith('image/')) {
      const file = item.getAsFile()
      if (file) files.push(file)
    }
  }
  if (files.length > 0) {
    event.preventDefault()
    void uploadFiles(files)
  }
}

function onDragOver(event: DragEvent): void {
  if (!auth.hasFeature('drag_upload')) return
  if (event.dataTransfer?.types.includes('Files')) {
    dragActive.value = true
  }
}

function onDragLeave(event: DragEvent): void {
  // 仅在真正离开页面时才取消高亮。
  if (event.relatedTarget === null) {
    dragActive.value = false
  }
}

function onDrop(event: DragEvent): void {
  dragActive.value = false
  if (!auth.hasFeature('drag_upload')) {
    ElMessage.warning('当前角色未开启拖拽上传')
    return
  }
  const dropped = event.dataTransfer?.files
  if (!dropped || dropped.length === 0) return
  event.preventDefault()
  void uploadFiles([...dropped])
}

// ---- 复制链接（多种格式）----
function linkFormats(item: ImageItem): { label: string; value: string }[] {
  const url = item.url
  const name = item.original_name || item.filename || 'image'
  const base = [{ label: '复制 URL', value: url }]
  if (!auth.hasFeature('embed_code')) {
    return base
  }
  return [
    ...base,
    { label: '复制 HTML', value: `<img src="${url}" alt="${name}">` },
    { label: '复制 BBCode', value: `[img]${url}[/img]` },
    { label: '复制 Markdown', value: `![${name}](${url})` },
  ]
}

async function copyValue(value: string): Promise<void> {
  const copied = await copyText(value)
  if (copied) {
    ElMessage.success('已复制')
  } else {
    ElMessage.error('复制失败，请手动复制')
  }
}

async function copyLink(item: ImageItem): Promise<void> {
  await copyValue(item.url)
}

// ---- 重命名 ----
function openRename(item: ImageItem): void {
  renameTarget.value = item
  renameName.value = item.original_name || item.filename || item.key
  renameOpen.value = true
}

async function submitRename(): Promise<void> {
  const target = renameTarget.value
  const name = renameName.value.trim()
  if (!target || !name || renaming.value) return
  renaming.value = true
  try {
    const updated = await renameImage(target.id, name)
    items.value = items.value.map((i) => (i.id === updated.id ? updated : i))
    ElMessage.success('已重命名')
    renameOpen.value = false
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    renaming.value = false
  }
}

// ---- 详情 ----
function openDetail(item: ImageItem): void {
  detailTarget.value = item
  detailOpen.value = true
}

// ---- 删除 ----
async function deleteOne(item: ImageItem): Promise<void> {
  try {
    await ElMessageBox.confirm('该图片将被永久删除，操作不可恢复。', '删除图片', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning',
      confirmButtonClass: 'el-button--danger',
    })
  } catch {
    return
  }
  try {
    await deleteImage(item.id)
    ElMessage.success('图片已删除')
    if (items.value.length === 1 && page.value > 1) {
      page.value -= 1
    }
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  }
}

async function deleteSelected(): Promise<void> {
  const ids = [...selectedIds.value]
  if (ids.length === 0) return
  try {
    await ElMessageBox.confirm(`将删除选中的 ${ids.length} 张图片，操作不可恢复。`, '批量删除', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning',
      confirmButtonClass: 'el-button--danger',
    })
  } catch {
    return
  }
  let ok = 0
  let failed = 0
  for (const id of ids) {
    try {
      await deleteImage(id)
      ok += 1
    } catch {
      failed += 1
    }
  }
  clearSelection()
  if (ok > 0) {
    ElMessage.success(failed > 0 ? `已删除 ${ok} 张，${failed} 张失败` : `已删除 ${ok} 张图片`)
    page.value = 1
    await load()
  } else {
    ElMessage.error('删除失败')
  }
}

// ---- 右键菜单 ----
function openMenu(event: MouseEvent, item: ImageItem | null): void {
  event.preventDefault()
  menuTarget.value = item
  // 选中当前图片（若未在多选中）。
  if (item && !selectedIds.value.has(item.id)) {
    selectedIds.value = new Set([item.id])
  }
  menuX.value = Math.min(event.clientX, window.innerWidth - 190)
  menuY.value = Math.min(event.clientY, window.innerHeight - 320)
  menuOpen.value = true
}

function closeMenu(): void {
  menuOpen.value = false
}

function runMenu(action: () => void): void {
  closeMenu()
  action()
}

function openInNewTab(url: string): void {
  window.open(url, '_blank', 'noopener,noreferrer')
}

// 基于右键菜单当前目标的操作封装，避免在模板中处理可空目标。
function menuCopy(value: string): void {
  runMenu(() => copyValue(value))
}

function menuOpenTab(): void {
  const item = menuTarget.value
  if (item) runMenu(() => openInNewTab(item.url))
}

function menuRename(): void {
  const item = menuTarget.value
  if (item) runMenu(() => openRename(item))
}

function menuDetail(): void {
  const item = menuTarget.value
  if (item) runMenu(() => openDetail(item))
}

function menuDelete(): void {
  const item = menuTarget.value
  if (item) runMenu(() => deleteOne(item))
}

function menuSetPermission(permission: ImagePermission): void {
  const item = menuTarget.value
  if (item) runMenu(() => applyPermission([item.id], permission))
}

function menuAssign(): void {
  const item = menuTarget.value
  if (item) runMenu(() => openAssign([item.id], item.album_id))
}

// ---- 键盘 ----
function onKeydown(event: KeyboardEvent): void {
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'a') {
    // 仅在图片页且焦点不在输入框内时全选。
    const target = event.target as HTMLElement | null
    if (target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA')) return
    event.preventDefault()
    selectAllVisible()
  }
  if (event.key === 'Escape') {
    clearSelection()
    closeMenu()
  }
}

onMounted(() => {
  const queryAlbum = route.query.album_id
  if (typeof queryAlbum === 'string') albumFilter.value = queryAlbum
  if (!auth.policies) void auth.loadPolicies()
  void loadAlbums()
  void load()
  document.addEventListener('paste', onPaste)
  document.addEventListener('keydown', onKeydown)
  document.addEventListener('click', closeMenu)
})

onBeforeUnmount(() => {
  document.removeEventListener('paste', onPaste)
  document.removeEventListener('keydown', onKeydown)
  document.removeEventListener('click', closeMenu)
})
</script>

<template>
  <div
    class="ax-page images-page"
    :class="{ 'is-dragging': dragActive }"
    @dragover="onDragOver"
    @dragleave="onDragLeave"
    @drop="onDrop"
  >
    <PageHeader title="图片管理" :description="description">
      <template #actions>
        <el-input
          v-model="keyword"
          class="images-search"
          placeholder="搜索文件名"
          clearable
          :prefix-icon="Search"
          @keyup.enter="search"
          @clear="search"
        />
        <el-dropdown trigger="click" @command="setOrder">
          <el-button>
            {{ orderOptions.find((o) => o.value === order)?.label }}
            <el-icon class="images-caret"><ArrowDown /></el-icon>
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
        <el-select
          :model-value="albumFilter"
          class="images-filter"
          placeholder="全部相册"
          @change="setAlbumFilter"
        >
          <el-option label="全部相册" value="" />
          <el-option v-for="album in albums" :key="album.id" :label="album.name" :value="album.id" />
        </el-select>
        <el-select
          :model-value="permissionFilter"
          class="images-filter"
          @change="setPermissionFilter"
        >
          <el-option
            v-for="option in permissionOptions"
            :key="option.value"
            :label="option.label"
            :value="option.value"
          />
        </el-select>
        <el-upload
          ref="uploadRef"
          :show-file-list="false"
          :accept="ACCEPT"
          :multiple="auth.hasFeature('batch_upload')"
          :before-upload="beforeUpload"
          :http-request="handleUpload"
        >
          <el-button type="primary" :icon="Upload" :loading="uploadRunning">上传图片</el-button>
        </el-upload>
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
      description="点击「上传图片」，或拖拽、粘贴图片到本页即可上传。"
    />

    <template v-else>
      <div v-if="selectedCount > 0" class="images-selection-bar">
        <span class="images-selection-bar__text">已选择 {{ selectedCount }} 张图片</span>
        <div class="images-selection-bar__actions">
          <el-button size="small" @click="clearSelection">取消选择</el-button>
          <el-button
            size="small"
            :icon="Unlock"
            @click="applyPermission([...selectedIds], 'public')"
          >
            设为公开
          </el-button>
          <el-button
            size="small"
            :icon="Lock"
            @click="applyPermission([...selectedIds], 'private')"
          >
            设为私有
          </el-button>
          <el-button size="small" :icon="Folder" @click="openAssign([...selectedIds])">
            归入相册
          </el-button>
          <el-button
            size="small"
            type="danger"
            plain
            :icon="Delete"
            @click="deleteSelected"
          >
            删除所选
          </el-button>
        </div>
      </div>

      <section
        class="ax-grid image-grid"
        :aria-busy="loading"
        aria-label="图片列表"
        @contextmenu.self.prevent="openMenu($event, null)"
      >
        <article
          v-for="item in items"
          :key="item.id"
          class="image-card"
          :class="{ 'is-selected': isSelected(item.id) }"
          @contextmenu="openMenu($event, item)"
        >
          <div class="image-card__preview">
            <el-image
              v-if="!failedIds.has(item.id)"
              class="image-card__img"
              :src="item.url"
              :preview-src-list="previewList"
              :initial-index="items.findIndex((i) => i.id === item.id)"
              fit="cover"
              loading="lazy"
              :alt="item.original_name || item.key"
              @error="onPreviewError(item.id)"
            />
            <span v-else class="image-card__fallback">
              <el-icon :size="20"><Picture /></el-icon>
              <span>预览不可用</span>
            </span>

            <button
              class="image-card__select"
              type="button"
              :aria-pressed="isSelected(item.id)"
              :aria-label="isSelected(item.id) ? '取消选择' : '选择'"
              @click.stop="toggleSelect(item.id)"
            >
              <el-icon :size="14"><Select /></el-icon>
            </button>
          </div>

          <div class="image-card__body">
            <p class="image-card__key" :title="item.original_name || item.filename || item.key">
              {{ item.original_name || item.filename || item.key }}
            </p>

            <div class="image-card__badges">
              <el-tag v-if="item.permission === 'public'" size="small" type="success" effect="dark">
                公开
              </el-tag>
              <el-tag v-else size="small" type="info" effect="plain">私有</el-tag>
              <el-tag v-if="item.album_id" size="small" effect="plain">
                {{ albumName(item.album_id) }}
              </el-tag>
            </div>

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
              <el-button size="small" :icon="EditPen" @click="openRename(item)">重命名</el-button>
              <el-button
                size="small"
                type="danger"
                plain
                :icon="Delete"
                @click="deleteOne(item)"
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

    <!-- 拖拽提示层 -->
    <div v-if="dragActive" class="images-dropzone" aria-hidden="true">
      <el-icon :size="40"><Upload /></el-icon>
      <span>松开以上传图片</span>
    </div>

    <!-- 上传队列 -->
    <div v-if="uploadTasks.length > 0" class="upload-queue" aria-live="polite">
      <header class="upload-queue__head">
        <span class="upload-queue__title">
          上传队列
          <span class="ax-muted">{{ uploadFinished }}/{{ uploadTasks.length }}</span>
        </span>
        <div class="upload-queue__actions">
          <el-button link size="small" @click="uploadPanelOpen = !uploadPanelOpen">
            {{ uploadPanelOpen ? '收起' : '展开' }}
          </el-button>
          <el-button link size="small" :disabled="uploadRunning" @click="clearUploads">清空</el-button>
        </div>
      </header>
      <el-progress
        :percentage="uploadOverall"
        :show-text="false"
        :status="
          !uploadRunning && uploadTasks.every((task) => task.status === 'success')
            ? 'success'
            : undefined
        "
      />
      <ul v-show="uploadPanelOpen" class="upload-queue__list">
        <li v-for="task in uploadTasks" :key="task.id" class="upload-queue__item">
          <span class="upload-queue__name" :title="task.name">{{ task.name }}</span>
          <span class="upload-queue__status" :class="`is-${task.status}`" :title="task.error">
            {{ uploadStatusLabel(task) }}
          </span>
        </li>
      </ul>
    </div>

    <!-- 右键菜单 -->
    <ul
      v-if="menuOpen"
      class="images-context-menu"
      :style="{ left: `${menuX}px`, top: `${menuY}px` }"
      @click.stop
    >
      <template v-if="menuTarget">
        <li class="images-context-menu__head">图片操作</li>
        <li
          v-for="fmt in linkFormats(menuTarget)"
          :key="fmt.label"
          @click="menuCopy(fmt.value)"
        >
          <el-icon><CopyDocument /></el-icon>{{ fmt.label }}
        </li>
        <li @click="menuOpenTab"><el-icon><Picture /></el-icon>新窗口打开</li>
        <li v-if="menuTarget.permission !== 'public'" @click="menuSetPermission('public')">
          <el-icon><Unlock /></el-icon>设为公开
        </li>
        <li v-else @click="menuSetPermission('private')">
          <el-icon><Lock /></el-icon>设为私有
        </li>
        <li @click="menuAssign"><el-icon><Folder /></el-icon>归入相册</li>
        <li class="images-context-menu__divider" />
        <li @click="menuRename"><el-icon><EditPen /></el-icon>重命名</li>
        <li @click="menuDetail"><el-icon><InfoFilled /></el-icon>详细信息</li>
        <li class="is-danger" @click="menuDelete"><el-icon><Delete /></el-icon>删除</li>
      </template>
      <template v-else>
        <li @click="runMenu(load)"><el-icon><Refresh /></el-icon>刷新</li>
        <li @click="runMenu(selectAllVisible)"><el-icon><Select /></el-icon>全选</li>
      </template>
    </ul>

    <!-- 重命名 -->
    <el-dialog
      v-model="renameOpen"
      title="重命名图片"
      width="min(420px, 92vw)"
      append-to-body
      :close-on-click-modal="false"
    >
      <el-input v-model="renameName" maxlength="120" placeholder="请输入新的图片名称" @keyup.enter="submitRename" />
      <template #footer>
        <el-button @click="renameOpen = false">取消</el-button>
        <el-button type="primary" :loading="renaming" @click="submitRename">确认</el-button>
      </template>
    </el-dialog>

    <!-- 归入相册 -->
    <el-dialog
      v-model="assignOpen"
      title="归入相册"
      width="min(420px, 92vw)"
      append-to-body
      :close-on-click-modal="false"
    >
      <el-select v-model="assignAlbumId" placeholder="不归入相册" style="width: 100%">
        <el-option label="不归入相册" value="" />
        <el-option v-for="album in albums" :key="album.id" :label="album.name" :value="album.id" />
      </el-select>
      <template #footer>
        <el-button @click="assignOpen = false">取消</el-button>
        <el-button type="primary" :loading="assigning" @click="submitAssign">确认</el-button>
      </template>
    </el-dialog>

    <!-- 详情 -->
    <el-drawer v-model="detailOpen" title="图片详情" size="360px" append-to-body>
      <dl v-if="detailTarget" class="image-detail">
        <div class="image-detail__row">
          <dt>名称</dt>
          <dd>{{ detailTarget.original_name || '—' }}</dd>
        </div>
        <div class="image-detail__row">
          <dt>存储名</dt>
          <dd>{{ detailTarget.filename || detailTarget.key }}</dd>
        </div>
        <div class="image-detail__row">
          <dt>哈希</dt>
          <dd class="ax-mono">{{ detailTarget.hash || '—' }}</dd>
        </div>
        <div class="image-detail__row">
          <dt>大小</dt>
          <dd>{{ formatBytes(detailTarget.size) }}</dd>
        </div>
        <div class="image-detail__row">
          <dt>类型</dt>
          <dd>{{ detailTarget.mime_type }}</dd>
        </div>
        <div class="image-detail__row">
          <dt>尺寸</dt>
          <dd>{{ formatDimensions(detailTarget.width, detailTarget.height) }}</dd>
        </div>
        <div class="image-detail__row">
          <dt>链接</dt>
          <dd class="image-detail__link" @click="copyValue(detailTarget.url)">
            {{ detailTarget.url }}
          </dd>
        </div>
        <div class="image-detail__row">
          <dt>可见性</dt>
          <dd>{{ detailTarget.permission === 'public' ? '公开' : '私有' }}</dd>
        </div>
        <div class="image-detail__row">
          <dt>相册</dt>
          <dd>{{ albumName(detailTarget.album_id) || '未归入相册' }}</dd>
        </div>
        <div class="image-detail__row">
          <dt>上传时间</dt>
          <dd>{{ formatDateTime(detailTarget.created_at) }}</dd>
        </div>
      </dl>
    </el-drawer>
  </div>
</template>

<style scoped>
.image-grid {
  grid-template-columns: repeat(auto-fill, minmax(min(250px, 100%), 1fr));
}

/* 让上传按钮与旁边按钮在头部对齐（el-upload 默认是 inline-block）。 */
:deep(.el-upload) {
  display: inline-flex;
}

.images-search {
  width: 200px;
}

.images-filter {
  width: 150px;
}

.images-caret {
  margin-left: 4px;
}

.image-card {
  position: relative;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: var(--ax-tint-weak);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-lg);
  transition:
    background-color var(--ax-duration) var(--ax-ease),
    border-color var(--ax-duration) var(--ax-ease);
}

.image-card:hover {
  background: var(--ax-tint);
  border-color: var(--ax-border);
}

.image-card.is-selected {
  border-color: var(--ax-accent-bright);
  box-shadow: 0 0 0 1px var(--ax-accent-ring);
}

.image-card__preview {
  position: relative;
  display: block;
  aspect-ratio: 16 / 10;
  overflow: hidden;
  background:
    linear-gradient(45deg, var(--ax-checker-a) 25%, transparent 25%) 0 0 / 16px 16px,
    linear-gradient(-45deg, var(--ax-checker-a) 25%, transparent 25%) 0 8px / 16px 16px,
    var(--ax-checker-b);
  border-bottom: 1px solid var(--ax-border-subtle);
}

.image-card__img {
  width: 100%;
  height: 100%;
  transform: scale(1);
  transition: transform var(--ax-duration-slow) var(--ax-ease);
}

.image-card:hover .image-card__img {
  transform: scale(1.03);
}

.image-card__select {
  position: absolute;
  top: 6px;
  right: 6px;
  z-index: 2;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  padding: 0;
  color: transparent;
  background: var(--ax-panel);
  border: 1px solid var(--ax-border-strong);
  border-radius: var(--ax-radius-full);
  cursor: pointer;
  opacity: 0;
  transition:
    opacity var(--ax-duration-fast) var(--ax-ease),
    background-color var(--ax-duration-fast) var(--ax-ease);
}

.image-card:hover .image-card__select,
.image-card.is-selected .image-card__select {
  opacity: 1;
}

.image-card.is-selected .image-card__select {
  color: var(--ax-on-accent);
  background: var(--ax-accent);
  border-color: var(--ax-accent);
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

.image-card__badges {
  display: flex;
  flex-wrap: wrap;
  gap: var(--ax-space-2);
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
  flex-wrap: wrap;
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

/* 选择操作条 */
.images-selection-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--ax-space-3);
  margin-bottom: var(--ax-space-4);
  padding: var(--ax-space-2) var(--ax-space-4);
  background: var(--ax-accent-soft);
  border: 1px solid var(--ax-accent-ring);
  border-radius: var(--ax-radius-md);
}

.images-selection-bar__text {
  color: var(--ax-text);
  font-size: var(--ax-text-sm);
  font-weight: var(--ax-weight-medium);
}

.images-selection-bar__actions {
  display: flex;
  gap: var(--ax-space-2);
}

/* 拖拽提示层 */
.images-dropzone {
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

/* 上传队列 */
.upload-queue {
  position: fixed;
  right: 16px;
  bottom: 16px;
  z-index: var(--ax-z-drawer);
  width: min(360px, 92vw);
  padding: var(--ax-space-3) var(--ax-space-4);
  background: var(--ax-elevated);
  border: 1px solid var(--ax-border);
  border-radius: var(--ax-radius-md);
  box-shadow: var(--ax-shadow-md);
}

.upload-queue__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--ax-space-2);
  margin-bottom: var(--ax-space-2);
}

.upload-queue__title {
  color: var(--ax-text);
  font-size: var(--ax-text-sm);
  font-weight: var(--ax-weight-medium);
}

.upload-queue__actions {
  display: flex;
  gap: var(--ax-space-2);
}

.upload-queue__list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 200px;
  margin: var(--ax-space-2) 0 0;
  padding: 0;
  overflow-y: auto;
  list-style: none;
}

.upload-queue__item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--ax-space-3);
  font-size: var(--ax-text-xs);
}

.upload-queue__name {
  overflow: hidden;
  color: var(--ax-text-3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.upload-queue__status {
  flex: none;
  color: var(--ax-text-4);
}

.upload-queue__status.is-success {
  color: var(--ax-success);
}

.upload-queue__status.is-error {
  color: var(--ax-danger);
}

.upload-queue__status.is-uploading {
  color: var(--ax-accent-hover);
}

/* 右键菜单 */
.images-context-menu {
  position: fixed;
  z-index: var(--ax-z-drawer);
  min-width: 176px;
  margin: 0;
  padding: var(--ax-space-1);
  list-style: none;
  background: var(--ax-elevated);
  border: 1px solid var(--ax-border);
  border-radius: var(--ax-radius-md);
  box-shadow: var(--ax-shadow-md);
}

.images-context-menu__head {
  padding: 4px 10px;
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.images-context-menu li:not(.images-context-menu__head):not(.images-context-menu__divider) {
  display: flex;
  align-items: center;
  gap: var(--ax-space-2);
  padding: 6px 10px;
  color: var(--ax-text-2);
  font-size: var(--ax-text-sm);
  border-radius: var(--ax-radius-xs);
  cursor: pointer;
}

.images-context-menu li:not(.images-context-menu__head):not(.images-context-menu__divider):hover {
  background: var(--ax-tint-strong);
  color: var(--ax-text);
}

.images-context-menu li.is-danger:hover {
  color: var(--ax-danger);
}

.images-context-menu__divider {
  height: 1px;
  margin: var(--ax-space-1) 0;
  background: var(--ax-border-subtle);
}

/* 详情抽屉 */
.image-detail {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-4);
  margin: 0;
}

.image-detail__row {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.image-detail__row dt {
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.image-detail__row dd {
  margin: 0;
  color: var(--ax-text-2);
  font-size: var(--ax-text-sm);
  overflow-wrap: anywhere;
}

.image-detail__link {
  color: var(--ax-accent-hover);
  cursor: pointer;
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

  .images-search {
    width: 100%;
  }
}
</style>

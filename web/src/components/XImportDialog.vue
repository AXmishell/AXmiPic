<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Check } from '@element-plus/icons-vue'

import { importX, previewXImport, type XImportResult, type XMedia, type XPost } from '@/api/import'
import { toApiError } from '@/api/client'
import type { ImagePermission } from '@/api/types'
import { formatDimensions } from '@/utils/format'

const open = defineModel<boolean>({ default: false })
const emit = defineEmits<{ imported: [result: XImportResult] }>()

const url = ref('')
const post = ref<XPost | null>(null)
const selected = ref<Set<string>>(new Set())
const permission = ref<ImagePermission>('public')
const parsing = ref(false)
const importing = ref(false)
const error = ref('')
const result = ref<XImportResult | null>(null)

const media = computed<XMedia[]>(() => post.value?.media ?? [])
const allHaveIDs = computed(() => media.value.every((item) => !!item.id))
const selectedCount = computed(() => selected.value.size)
const allSelected = computed(
  () => media.value.length > 0 && selected.value.size === media.value.length,
)
const canImport = computed(() => !!post.value && selectedCount.value > 0 && !importing.value)
const failures = computed(
  () => result.value?.items.filter((item) => item.status === 'failed' && item.error) ?? [],
)

function mediaKey(item: XMedia, index: number): string {
  return item.id || `index-${index}`
}

function isSelected(item: XMedia, index: number): boolean {
  return selected.value.has(mediaKey(item, index))
}

function toggleMedia(item: XMedia, index: number): void {
  if (!allHaveIDs.value) return
  const key = mediaKey(item, index)
  const next = new Set(selected.value)
  if (next.has(key)) {
    next.delete(key)
  } else {
    next.add(key)
  }
  selected.value = next
}

function toggleAll(): void {
  if (!allHaveIDs.value) return
  selected.value = allSelected.value ? new Set() : new Set(media.value.map((item) => item.id!))
}

async function parse(): Promise<void> {
  const value = url.value.trim()
  if (!value) {
    ElMessage.warning('请粘贴 X 帖子链接')
    return
  }
  parsing.value = true
  error.value = ''
  result.value = null
  post.value = null
  try {
    const data = await previewXImport(value)
    post.value = data
    selected.value = new Set(data.media.map((item) => item.id).filter((id): id is string => !!id))
    permission.value = 'public'
  } catch (err) {
    error.value = toApiError(err).message
  } finally {
    parsing.value = false
  }
}

async function submit(): Promise<void> {
  if (!post.value || selectedCount.value === 0) return
  importing.value = true
  error.value = ''
  try {
    const data = await importX({
      url: post.value.source_url,
      mediaIds: allSelected.value ? undefined : [...selected.value],
      permission: permission.value,
    })
    result.value = data
    emit('imported', data)
    const parts = [`导入 ${data.imported} 张`]
    if (data.existing > 0) parts.push(`已存在 ${data.existing} 张`)
    if (data.blocked > 0) parts.push(`审核拦截 ${data.blocked} 张`)
    if (data.failed > 0) parts.push(`失败 ${data.failed} 张`)
    if (data.failed > 0 && data.imported === 0 && data.existing === 0) {
      ElMessage.error(parts.join('，'))
    } else if (data.blocked > 0 || data.failed > 0) {
      ElMessage.warning(parts.join('，'))
    } else {
      ElMessage.success(parts.join('，'))
    }
    // 成功导入后关闭弹窗；有失败项时保留，便于查看原因。
    if (data.failed === 0) {
      open.value = false
    }
  } catch (err) {
    error.value = toApiError(err).message
    ElMessage.error(error.value)
  } finally {
    importing.value = false
  }
}

watch(open, (value) => {
  if (!value) return
  url.value = ''
  post.value = null
  selected.value = new Set()
  permission.value = 'public'
  error.value = ''
  result.value = null
})
</script>

<template>
  <el-dialog
    v-model="open"
    title="从 X 导入"
    width="min(720px, 94vw)"
    append-to-body
    :close-on-click-modal="false"
  >
    <div class="x-import">
      <div class="x-import__url">
        <el-input
          v-model="url"
          placeholder="粘贴 X 帖子链接，例如 https://x.com/user/status/123..."
          clearable
          @keyup.enter="parse"
        />
        <el-button type="primary" :loading="parsing" :disabled="!url.trim()" @click="parse">
          解析
        </el-button>
      </div>

      <el-alert
        v-if="error"
        class="x-import__error"
        type="error"
        :title="error"
        :closable="false"
        show-icon
      />

      <template v-if="post">
        <div class="x-import__post">
          <div class="x-import__author">
            <span class="x-import__name">
              {{ post.author.name || `@${post.author.handle}` }}
            </span>
            <a
              v-if="post.author.handle"
              class="x-import__link"
              :href="`https://x.com/${post.author.handle}`"
              target="_blank"
              rel="noopener noreferrer nofollow"
            >
              @{{ post.author.handle }}
            </a>
            <a
              class="x-import__link x-import__source"
              :href="post.source_url"
              target="_blank"
              rel="noopener noreferrer nofollow"
            >
              原帖 ↗
            </a>
          </div>
          <p v-if="post.text" class="x-import__text">{{ post.text }}</p>
          <el-alert
            v-if="post.sensitive"
            type="warning"
            :closable="false"
            show-icon
            title="该帖子被标记为敏感内容，导入后仍需经过站内审核。"
          />
        </div>

        <div class="x-import__media-head">
          <span>选择图片（{{ selectedCount }} / {{ media.length }}）</span>
          <el-button link type="primary" :disabled="!allHaveIDs" @click="toggleAll">
            {{ allSelected ? '取消全选' : '全选' }}
          </el-button>
        </div>
        <div class="x-import__media">
          <button
            v-for="(item, index) in media"
            :key="mediaKey(item, index)"
            class="x-import__media-item"
            :class="{ 'is-selected': isSelected(item, index) }"
            type="button"
            @click="toggleMedia(item, index)"
          >
            <img
              :src="item.url"
              :alt="item.alt || 'X 图片'"
              loading="lazy"
              referrerpolicy="no-referrer"
            />
            <span class="x-import__check">
              <el-icon v-if="isSelected(item, index)"><Check /></el-icon>
            </span>
            <span v-if="item.width && item.height" class="x-import__size">
              {{ formatDimensions(item.width, item.height) }}
            </span>
          </button>
        </div>

        <div class="x-import__options">
          <span class="x-import__options-label">导入后</span>
          <el-radio-group v-model="permission">
            <el-radio-button value="public">公开到广场</el-radio-button>
            <el-radio-button value="private">先私有</el-radio-button>
          </el-radio-group>
          <span class="x-import__hint">
            私有导入后可在图片管理中确认公开
          </span>
        </div>

        <div v-if="result" class="x-import__result">
          <span>成功 {{ result.imported }}</span>
          <span>已存在 {{ result.existing }}</span>
          <span v-if="result.blocked">审核拦截 {{ result.blocked }}</span>
          <span v-if="result.failed">失败 {{ result.failed }}</span>
        </div>
        <ul v-if="failures.length" class="x-import__failures">
          <li v-for="(item, index) in failures" :key="index">{{ item.error }}</li>
        </ul>
      </template>

      <p v-else class="x-import__empty">
        支持 x.com / twitter.com 的状态链接。解析成功后可选择图片与导入后的可见性。
      </p>
    </div>

    <template #footer>
      <el-button @click="open = false">取消</el-button>
      <el-button type="primary" :loading="importing" :disabled="!canImport" @click="submit">
        导入 {{ selectedCount }} 张
      </el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.x-import {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.x-import__url {
  display: flex;
  gap: 8px;
}

.x-import__url :deep(.el-input) {
  flex: 1;
}

.x-import__post {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px;
  background: var(--ax-tint-weak);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-md);
}

.x-import__author {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 8px;
}

.x-import__name {
  font-weight: 600;
}

.x-import__link {
  color: var(--ax-accent-bright);
  font-size: var(--ax-text-sm);
  text-decoration: none;
}

.x-import__link:hover {
  text-decoration: underline;
}

.x-import__source {
  margin-left: auto;
}

.x-import__text {
  margin: 0;
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.x-import__media-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
}

.x-import__media {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: 10px;
}

.x-import__media-item {
  position: relative;
  padding: 0;
  overflow: hidden;
  background: var(--ax-tint-weak);
  border: 2px solid transparent;
  border-radius: var(--ax-radius-md);
  cursor: pointer;
}

.x-import__media-item.is-selected {
  border-color: var(--ax-accent-bright);
}

.x-import__media-item img {
  display: block;
  width: 100%;
  aspect-ratio: 1;
  object-fit: cover;
}

.x-import__check {
  position: absolute;
  top: 6px;
  right: 6px;
  display: grid;
  width: 22px;
  height: 22px;
  color: #fff;
  background: var(--ax-accent-bright);
  border: 2px solid #fff;
  border-radius: 50%;
  place-items: center;
}

.x-import__size {
  position: absolute;
  right: 6px;
  bottom: 6px;
  padding: 2px 6px;
  color: #fff;
  font-size: 11px;
  background: rgba(0, 0, 0, 0.55);
  border-radius: 999px;
}

.x-import__options {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
}

.x-import__options-label {
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
}

.x-import__hint {
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.x-import__result {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
}

.x-import__failures {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin: 0;
  padding-left: 18px;
  color: var(--ax-danger, #d9534f);
  font-size: var(--ax-text-xs);
  overflow-wrap: anywhere;
}

.x-import__empty {
  margin: 0;
  color: var(--ax-text-4);
  font-size: var(--ax-text-sm);
}
</style>

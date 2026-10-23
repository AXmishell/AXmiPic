<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { Check } from '@element-plus/icons-vue'

import { importX, previewXImport, type XImportResult, type XMedia, type XPost } from '@/api/import'
import { toApiError } from '@/api/client'
import type { ImagePermission } from '@/api/types'
import { formatDimensions } from '@/utils/format'

const open = defineModel<boolean>({ default: false })
const emit = defineEmits<{ imported: [result: XImportResult] }>()
const { t } = useI18n()

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
    ElMessage.warning(t('xImportDialog.validation.urlRequired'))
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
    const parts = [t('xImportDialog.messages.imported', { count: data.imported })]
    if (data.existing > 0) parts.push(t('xImportDialog.messages.existing', { count: data.existing }))
    if (data.blocked > 0) parts.push(t('xImportDialog.messages.blocked', { count: data.blocked }))
    if (data.failed > 0) parts.push(t('xImportDialog.messages.failed', { count: data.failed }))
    const summary = parts.join(t('xImportDialog.messages.separator'))
    if (data.failed > 0 && data.imported === 0 && data.existing === 0) {
      ElMessage.error(summary)
    } else if (data.blocked > 0 || data.failed > 0) {
      ElMessage.warning(summary)
    } else {
      ElMessage.success(summary)
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
    :title="t('xImportDialog.title')"
    width="min(720px, 94vw)"
    append-to-body
    :close-on-click-modal="false"
  >
    <div class="x-import">
      <div class="x-import__url">
        <el-input
          v-model="url"
          :placeholder="t('xImportDialog.urlPlaceholder')"
          clearable
          @keyup.enter="parse"
        />
        <el-button type="primary" :loading="parsing" :disabled="!url.trim()" @click="parse">
          {{ t('xImportDialog.parse') }}
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
              {{ t('xImportDialog.originalPost') }}
            </a>
          </div>
          <p v-if="post.text" class="x-import__text">{{ post.text }}</p>
          <el-alert
            v-if="post.sensitive"
            type="warning"
            :closable="false"
            show-icon
            :title="t('xImportDialog.sensitive')"
          />
        </div>

        <div class="x-import__media-head">
          <span>{{ t('xImportDialog.mediaHead', { selected: selectedCount, total: media.length }) }}</span>
          <el-button link type="primary" :disabled="!allHaveIDs" @click="toggleAll">
            {{ allSelected ? t('xImportDialog.deselectAll') : t('xImportDialog.selectAll') }}
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
              :alt="item.alt || t('xImportDialog.mediaAlt')"
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
          <span class="x-import__options-label">{{ t('xImportDialog.optionsLabel') }}</span>
          <el-radio-group v-model="permission">
            <el-radio-button value="public">{{ t('xImportDialog.publicToPlaza') }}</el-radio-button>
            <el-radio-button value="private">{{ t('xImportDialog.privateFirst') }}</el-radio-button>
          </el-radio-group>
          <span class="x-import__hint">
            {{ t('xImportDialog.privateHint') }}
          </span>
        </div>

        <div v-if="result" class="x-import__result">
          <span>{{ t('xImportDialog.result.imported', { count: result.imported }) }}</span>
          <span>{{ t('xImportDialog.result.existing', { count: result.existing }) }}</span>
          <span v-if="result.blocked">{{ t('xImportDialog.result.blocked', { count: result.blocked }) }}</span>
          <span v-if="result.failed">{{ t('xImportDialog.result.failed', { count: result.failed }) }}</span>
        </div>
        <ul v-if="failures.length" class="x-import__failures">
          <li v-for="(item, index) in failures" :key="index">{{ item.error }}</li>
        </ul>
      </template>

      <p v-else class="x-import__empty">
        {{ t('xImportDialog.empty') }}
      </p>
    </div>

    <template #footer>
      <el-button @click="open = false">{{ t('common.cancel') }}</el-button>
      <el-button type="primary" :loading="importing" :disabled="!canImport" @click="submit">
        {{ t('xImportDialog.importCount', { count: selectedCount }) }}
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

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { CopyDocument, Refresh } from '@element-plus/icons-vue'

import { toApiError } from '@/api/client'
import { listImages, transformUrl, type TransformParams } from '@/api/images'
import type { ImageItem } from '@/api/types'
import CopyField from '@/components/CopyField.vue'
import EmptyState from '@/components/EmptyState.vue'
import PageHeader from '@/components/PageHeader.vue'
import { copyText } from '@/utils/clipboard'

const { t } = useI18n()

const images = ref<ImageItem[]>([])
const loading = ref(false)
const errorMessage = ref('')
const selectedId = ref('')

const selected = computed(() => images.value.find((i) => i.id === selectedId.value) ?? null)

const form = reactive({
  width: 0,
  height: 0,
  fit: 'contain' as TransformParams['fit'],
  quality: 0,
  format: '',
  rotate: 0 as 0 | 90 | 180 | 270,
  flip: '' as '' | 'h' | 'v' | 'hv',
  grayscale: false,
  blur: 0,
  sharpen: 0,
  enlarge: false,
  watermark: '',
  watermarkPosition: 'bottom-right',
  watermarkOpacity: 70,
  watermarkSize: 0,
  watermarkColor: '#ffffff',
})

const params = computed<TransformParams>(() => ({
  width: form.width || undefined,
  height: form.height || undefined,
  fit: form.fit,
  quality: form.quality || undefined,
  format: form.format || undefined,
  rotate: form.rotate,
  flip: form.flip || undefined,
  grayscale: form.grayscale,
  blur: form.blur || undefined,
  sharpen: form.sharpen || undefined,
  enlarge: form.enlarge,
  watermark: form.watermark.trim() || undefined,
  watermarkPosition: form.watermarkPosition,
  watermarkOpacity: form.watermarkOpacity,
  watermarkSize: form.watermarkSize || undefined,
  watermarkColor: form.watermarkColor,
}))

const previewUrl = computed(() => (selected.value ? transformUrl(selected.value.url, params.value) : ''))
const previewFailed = ref(false)

const formatOptions = computed(() => [
  { value: '', label: t('processing.format.keepOriginal') },
  { value: 'jpeg', label: 'JPEG' },
  { value: 'png', label: 'PNG' },
  { value: 'gif', label: 'GIF' },
  { value: 'webp', label: 'WebP' },
  { value: 'avif', label: 'AVIF' },
])

const positionOptions = ['top-left', 'top-right', 'bottom-left', 'bottom-right', 'center']

async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    const data = await listImages({ page: 1, pageSize: 48, order: 'newest' })
    images.value = data.items ?? []
    if (images.value.length > 0 && !selectedId.value) {
      selectedId.value = images.value[0]!.id
    }
  } catch (error) {
    errorMessage.value = toApiError(error).message
  } finally {
    loading.value = false
  }
}

function reset(): void {
  Object.assign(form, {
    width: 0,
    height: 0,
    fit: 'contain',
    quality: 0,
    format: '',
    rotate: 0,
    flip: '',
    grayscale: false,
    blur: 0,
    sharpen: 0,
    enlarge: false,
    watermark: '',
    watermarkPosition: 'bottom-right',
    watermarkOpacity: 70,
    watermarkSize: 0,
    watermarkColor: '#ffffff',
  })
}

async function copyPreview(): Promise<void> {
  const ok = await copyText(previewUrl.value)
  if (ok) ElMessage.success(t('processing.messages.copied'))
  else ElMessage.error(t('processing.messages.copyFailed'))
}

watch(previewUrl, () => {
  previewFailed.value = false
})

onMounted(load)
</script>

<template>
  <div class="ax-page">
    <PageHeader :title="t('processing.title')" :description="t('processing.description')">
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
        <el-button @click="reset">{{ t('processing.actions.resetParams') }}</el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="load" />

    <EmptyState
      v-else-if="!loading && images.length === 0"
      :title="t('processing.empty.title')"
      :description="t('processing.empty.description')"
    />

    <div v-else class="editor">
      <aside class="editor__panel ax-card">
        <div class="ax-card__body">
          <div class="field">
            <label class="field__label">{{ t('processing.labels.selectImage') }}</label>
            <el-select v-model="selectedId" filterable style="width: 100%">
              <el-option
                v-for="image in images"
                :key="image.id"
                :label="image.original_name || image.filename || image.key"
                :value="image.id"
              />
            </el-select>
          </div>

          <div class="field-grid">
            <div class="field">
              <label class="field__label">{{ t('processing.labels.width') }}</label>
              <el-input-number v-model="form.width" :min="0" :max="4096" />
            </div>
            <div class="field">
              <label class="field__label">{{ t('processing.labels.height') }}</label>
              <el-input-number v-model="form.height" :min="0" :max="4096" />
            </div>
            <div class="field">
              <label class="field__label">{{ t('processing.labels.fit') }}</label>
              <el-select v-model="form.fit">
                <el-option label="contain" value="contain" />
                <el-option label="cover" value="cover" />
                <el-option label="fill" value="fill" />
              </el-select>
            </div>
            <div class="field">
              <label class="field__label">{{ t('processing.labels.format') }}</label>
              <el-select v-model="form.format">
                <el-option v-for="o in formatOptions" :key="o.value" :label="o.label" :value="o.value" />
              </el-select>
            </div>
            <div class="field">
              <label class="field__label">{{ t('processing.labels.quality') }}</label>
              <el-input-number v-model="form.quality" :min="0" :max="100" />
            </div>
            <div class="field">
              <label class="field__label">{{ t('processing.labels.rotate') }}</label>
              <el-select v-model="form.rotate">
                <el-option :label="t('processing.rotate.none')" :value="0" />
                <el-option label="90°" :value="90" />
                <el-option label="180°" :value="180" />
                <el-option label="270°" :value="270" />
              </el-select>
            </div>
            <div class="field">
              <label class="field__label">{{ t('processing.labels.flip') }}</label>
              <el-select v-model="form.flip">
                <el-option :label="t('processing.flip.none')" value="" />
                <el-option :label="t('processing.flip.horizontal')" value="h" />
                <el-option :label="t('processing.flip.vertical')" value="v" />
                <el-option :label="t('processing.flip.both')" value="hv" />
              </el-select>
            </div>
            <div class="field">
              <label class="field__label">{{ t('processing.labels.enlarge') }}</label>
              <el-switch v-model="form.enlarge" />
            </div>
          </div>

          <div class="field-grid">
            <div class="field">
              <label class="field__label">{{ t('processing.labels.grayscale') }}</label>
              <el-switch v-model="form.grayscale" />
            </div>
            <div class="field">
              <label class="field__label">{{ t('processing.labels.blur') }}</label>
              <el-slider v-model="form.blur" :min="0" :max="20" :step="0.5" />
            </div>
            <div class="field">
              <label class="field__label">{{ t('processing.labels.sharpen') }}</label>
              <el-slider v-model="form.sharpen" :min="0" :max="20" :step="0.5" />
            </div>
          </div>

          <el-divider content-position="left">{{ t('processing.labels.watermark') }}</el-divider>
          <div class="field">
            <label class="field__label">{{ t('processing.labels.watermarkText') }}</label>
            <el-input
              v-model="form.watermark"
              :placeholder="t('processing.watermarkPlaceholder')"
              maxlength="200"
            />
          </div>
          <div class="field-grid">
            <div class="field">
              <label class="field__label">{{ t('processing.labels.position') }}</label>
              <el-select v-model="form.watermarkPosition">
                <el-option v-for="p in positionOptions" :key="p" :label="p" :value="p" />
              </el-select>
            </div>
            <div class="field">
              <label class="field__label">{{ t('processing.labels.opacity') }}</label>
              <el-slider v-model="form.watermarkOpacity" :min="0" :max="100" />
            </div>
            <div class="field">
              <label class="field__label">{{ t('processing.labels.fontSize') }}</label>
              <el-input-number v-model="form.watermarkSize" :min="0" :max="400" />
            </div>
            <div class="field">
              <label class="field__label">{{ t('processing.labels.color') }}</label>
              <el-color-picker v-model="form.watermarkColor" />
            </div>
          </div>
        </div>
      </aside>

      <section class="editor__preview ax-card">
        <header class="ax-card__head">
          <h2 class="ax-card__title">{{ t('processing.preview.title') }}</h2>
          <el-button size="small" :icon="CopyDocument" :disabled="!previewUrl" @click="copyPreview">
            {{ t('processing.actions.copyLink') }}
          </el-button>
        </header>
        <div class="ax-card__body preview">
          <img
            v-if="previewUrl && !previewFailed"
            :src="previewUrl"
            :alt="selected?.original_name || t('processing.preview.alt')"
            @error="previewFailed = true"
          />
          <p v-else class="ax-muted">{{ t('processing.preview.unavailable') }}</p>
          <CopyField :label="t('processing.preview.linkLabel')" :value="previewUrl" />
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.editor {
  display: grid;
  grid-template-columns: minmax(280px, 380px) minmax(0, 1fr);
  gap: var(--ax-space-4);
  align-items: start;
}

.editor__panel :deep(.el-input-number),
.editor__panel :deep(.el-select) {
  width: 100%;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: var(--ax-space-3);
}

.field__label {
  color: var(--ax-text-3);
  font-size: var(--ax-text-xs);
  font-weight: var(--ax-weight-medium);
}

.field-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 var(--ax-space-3);
}

.preview {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--ax-space-4);
}

.preview img {
  max-width: 100%;
  max-height: 60vh;
  border-radius: var(--ax-radius-md);
}

.preview :deep(.copy-field) {
  width: 100%;
}

@media (max-width: 960px) {
  .editor {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>

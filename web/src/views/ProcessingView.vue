<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { CopyDocument, Refresh } from '@element-plus/icons-vue'

import { toApiError } from '@/api/client'
import { listImages, transformUrl, type TransformParams } from '@/api/images'
import type { ImageItem } from '@/api/types'
import CopyField from '@/components/CopyField.vue'
import EmptyState from '@/components/EmptyState.vue'
import PageHeader from '@/components/PageHeader.vue'
import { copyText } from '@/utils/clipboard'

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

const formatOptions = [
  { value: '', label: '保持原格式' },
  { value: 'jpeg', label: 'JPEG' },
  { value: 'png', label: 'PNG' },
  { value: 'gif', label: 'GIF' },
  { value: 'webp', label: 'WebP' },
  { value: 'avif', label: 'AVIF' },
]

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
  if (ok) ElMessage.success('已复制处理后的链接')
  else ElMessage.error('复制失败')
}

watch(previewUrl, () => {
  previewFailed.value = false
})

onMounted(load)
</script>

<template>
  <div class="ax-page">
    <PageHeader title="图片处理" description="通过 URL 参数实时缩放、裁剪、旋转、翻转、滤镜与水印。">
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
        <el-button @click="reset">重置参数</el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="load" />

    <EmptyState
      v-else-if="!loading && images.length === 0"
      title="还没有可处理的图片"
      description="先上传图片，再回到这里调试处理参数。"
    />

    <div v-else class="editor">
      <aside class="editor__panel ax-card">
        <div class="ax-card__body">
          <div class="field">
            <label class="field__label">选择图片</label>
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
              <label class="field__label">宽度</label>
              <el-input-number v-model="form.width" :min="0" :max="4096" />
            </div>
            <div class="field">
              <label class="field__label">高度</label>
              <el-input-number v-model="form.height" :min="0" :max="4096" />
            </div>
            <div class="field">
              <label class="field__label">适配</label>
              <el-select v-model="form.fit">
                <el-option label="contain" value="contain" />
                <el-option label="cover" value="cover" />
                <el-option label="fill" value="fill" />
              </el-select>
            </div>
            <div class="field">
              <label class="field__label">输出格式</label>
              <el-select v-model="form.format">
                <el-option v-for="o in formatOptions" :key="o.value" :label="o.label" :value="o.value" />
              </el-select>
            </div>
            <div class="field">
              <label class="field__label">质量</label>
              <el-input-number v-model="form.quality" :min="0" :max="100" />
            </div>
            <div class="field">
              <label class="field__label">旋转</label>
              <el-select v-model="form.rotate">
                <el-option label="不旋转" :value="0" />
                <el-option label="90°" :value="90" />
                <el-option label="180°" :value="180" />
                <el-option label="270°" :value="270" />
              </el-select>
            </div>
            <div class="field">
              <label class="field__label">翻转</label>
              <el-select v-model="form.flip">
                <el-option label="不翻转" value="" />
                <el-option label="水平" value="h" />
                <el-option label="垂直" value="v" />
                <el-option label="水平+垂直" value="hv" />
              </el-select>
            </div>
            <div class="field">
              <label class="field__label">允许放大</label>
              <el-switch v-model="form.enlarge" />
            </div>
          </div>

          <div class="field-grid">
            <div class="field">
              <label class="field__label">灰度</label>
              <el-switch v-model="form.grayscale" />
            </div>
            <div class="field">
              <label class="field__label">模糊强度</label>
              <el-slider v-model="form.blur" :min="0" :max="20" :step="0.5" />
            </div>
            <div class="field">
              <label class="field__label">锐化强度</label>
              <el-slider v-model="form.sharpen" :min="0" :max="20" :step="0.5" />
            </div>
          </div>

          <el-divider content-position="left">文字水印</el-divider>
          <div class="field">
            <label class="field__label">水印文字</label>
            <el-input v-model="form.watermark" placeholder="例如 AXmiPic" maxlength="200" />
          </div>
          <div class="field-grid">
            <div class="field">
              <label class="field__label">位置</label>
              <el-select v-model="form.watermarkPosition">
                <el-option v-for="p in positionOptions" :key="p" :label="p" :value="p" />
              </el-select>
            </div>
            <div class="field">
              <label class="field__label">不透明度</label>
              <el-slider v-model="form.watermarkOpacity" :min="0" :max="100" />
            </div>
            <div class="field">
              <label class="field__label">字号（0 自适应）</label>
              <el-input-number v-model="form.watermarkSize" :min="0" :max="400" />
            </div>
            <div class="field">
              <label class="field__label">颜色</label>
              <el-color-picker v-model="form.watermarkColor" />
            </div>
          </div>
        </div>
      </aside>

      <section class="editor__preview ax-card">
        <header class="ax-card__head">
          <h2 class="ax-card__title">实时预览</h2>
          <el-button size="small" :icon="CopyDocument" :disabled="!previewUrl" @click="copyPreview">
            复制链接
          </el-button>
        </header>
        <div class="ax-card__body preview">
          <img
            v-if="previewUrl && !previewFailed"
            :src="previewUrl"
            :alt="selected?.original_name || '预览'"
            @error="previewFailed = true"
          />
          <p v-else class="ax-muted">预览不可用，请检查处理参数。</p>
          <CopyField label="处理后的链接" :value="previewUrl" />
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

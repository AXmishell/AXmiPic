<script setup lang="ts">
import { computed, ref, onBeforeUnmount } from 'vue'
import { ElMessage } from 'element-plus'
import { CopyDocument, Select } from '@element-plus/icons-vue'

import { copyText } from '@/utils/clipboard'

const props = defineProps<{
  value: string
  label?: string
}>()

const copied = ref(false)
let timer: number | undefined

const hasValue = computed(() => props.value.length > 0)

async function handleCopy(): Promise<void> {
  const ok = await copyText(props.value)
  if (!ok) {
    ElMessage.error('复制失败，请手动选择文本复制')
    return
  }
  ElMessage.success('已复制到剪贴板')
  copied.value = true
  window.clearTimeout(timer)
  timer = window.setTimeout(() => {
    copied.value = false
  }, 2000)
}

onBeforeUnmount(() => {
  window.clearTimeout(timer)
})
</script>

<template>
  <div class="copy-field">
    <span v-if="label" class="copy-field__label">{{ label }}</span>
    <div class="copy-field__row">
      <code class="copy-field__value">{{ hasValue ? value : '—' }}</code>
      <el-button class="copy-field__btn" :disabled="!hasValue" @click="handleCopy">
        <el-icon>
          <Select v-if="copied" />
          <CopyDocument v-else />
        </el-icon>
        {{ copied ? '已复制' : '复制' }}
      </el-button>
    </div>
  </div>
</template>

<style scoped>
.copy-field {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-2);
  min-width: 0;
}

.copy-field__label {
  color: var(--ax-text-3);
  font-size: var(--ax-text-xs);
  font-weight: var(--ax-weight-medium);
}

.copy-field__row {
  display: flex;
  align-items: stretch;
  gap: var(--ax-space-2);
  min-width: 0;
}

.copy-field__value {
  flex: 1;
  min-width: 0;
  padding: 9px var(--ax-space-3);
  overflow-x: auto;
  background: rgba(255, 255, 255, 0.02);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-sm);
  color: var(--ax-text-2);
  font-family: var(--ax-font-mono);
  font-size: var(--ax-text-xs);
  line-height: 1.6;
  white-space: nowrap;
}

.copy-field__btn {
  flex: none;
}
</style>

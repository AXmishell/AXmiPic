<script setup lang="ts">
import { WarningFilled } from '@element-plus/icons-vue'

defineProps<{
  message: string
  title?: string
}>()

defineEmits<{ (event: 'retry'): void }>()
</script>

<template>
  <div class="error-state" role="alert">
    <span class="error-state__icon" aria-hidden="true">
      <el-icon :size="18"><WarningFilled /></el-icon>
    </span>
    <div class="error-state__text">
      <h3 class="error-state__title">{{ title ?? '加载失败' }}</h3>
      <p class="error-state__message">{{ message }}</p>
    </div>
    <el-button @click="$emit('retry')">重试</el-button>
  </div>
</template>

<style scoped>
.error-state {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--ax-space-4);
  padding: var(--ax-space-5);
  background: rgba(255, 255, 255, 0.02);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-lg);
}

.error-state__icon {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  color: #e8a5a1;
  background: var(--ax-danger-soft);
  border-radius: var(--ax-radius-full);
}

.error-state__text {
  flex: 1;
  min-width: 200px;
}

.error-state__title {
  color: var(--ax-text);
  font-size: var(--ax-text-base);
  font-weight: var(--ax-weight-medium);
}

.error-state__message {
  margin-top: 2px;
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
  overflow-wrap: anywhere;
}
</style>

<script setup lang="ts">
import { Document } from '@element-plus/icons-vue'

defineProps<{
  title: string
  description?: string
}>()
</script>

<template>
  <div class="empty-state">
    <span class="empty-state__icon" aria-hidden="true">
      <el-icon :size="20"><Document /></el-icon>
    </span>
    <h3 class="empty-state__title">{{ title }}</h3>
    <p v-if="description" class="empty-state__desc">{{ description }}</p>
    <div class="empty-state__actions">
      <slot />
    </div>
  </div>
</template>

<style scoped>
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: var(--ax-space-10) var(--ax-space-6);
  text-align: center;
}

.empty-state__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  margin-bottom: var(--ax-space-4);
  color: var(--ax-text-3);
  background: var(--ax-tint);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-full);
}

.empty-state__title {
  color: var(--ax-text);
  font-size: var(--ax-text-md);
  font-weight: var(--ax-weight-medium);
}

.empty-state__desc {
  margin-top: var(--ax-space-1);
  max-width: 46ch;
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
}

.empty-state__actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: var(--ax-space-2);
  margin-top: var(--ax-space-5);
}

.empty-state__actions:empty {
  display: none;
}
</style>

<script setup lang="ts">
import { useSlots } from 'vue'

defineProps<{
  title: string
  description?: string
}>()

const slots = useSlots()
</script>

<template>
  <header class="page-header">
    <div class="page-header__text">
      <h1 class="page-header__title">{{ title }}</h1>
      <p v-if="description" class="page-header__desc">{{ description }}</p>
    </div>
    <div v-if="slots.actions" class="page-header__actions">
      <slot name="actions" />
    </div>
  </header>

  <!--
    次级工具行：搜索、筛选、视图切换等放在这里，而不是和主操作挤在标题右侧。
    这样所有页面的「标题 → 操作 → 筛选」节奏一致，标题区也不会被控件撑高。
  -->
  <div v-if="slots.toolbar" class="page-header__toolbar">
    <slot name="toolbar" />
  </div>
</template>

<style scoped>
.page-header {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--ax-space-3) var(--ax-space-4);
  margin-bottom: var(--ax-space-5);
}

.page-header__text {
  min-width: 0;
}

.page-header__title {
  font-size: var(--ax-text-2xl);
  letter-spacing: var(--ax-tracking-display);
}

.page-header__desc {
  max-width: 68ch;
  margin-top: var(--ax-space-1);
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
  overflow-wrap: anywhere;
}

.page-header__actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--ax-space-2);
}

.page-header__toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--ax-space-2);
  margin: calc(-1 * var(--ax-space-2)) 0 var(--ax-space-5);
}

@media (max-width: 640px) {
  .page-header {
    margin-bottom: var(--ax-space-4);
  }

  .page-header__actions {
    width: 100%;
  }

  .page-header__toolbar {
    margin-bottom: var(--ax-space-4);
  }
}
</style>

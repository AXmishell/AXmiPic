<script setup lang="ts">
defineProps<{
  title: string
  description?: string
}>()
</script>

<template>
  <header class="page-header">
    <div class="page-header__text">
      <h1 class="page-header__title">{{ title }}</h1>
      <p v-if="description" class="page-header__desc">{{ description }}</p>
    </div>
    <div class="page-header__actions">
      <slot name="actions" />
    </div>
  </header>
</template>

<style scoped>
.page-header {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--ax-space-4);
  margin-bottom: var(--ax-space-6);
}

.page-header__text {
  min-width: 0;
}

.page-header__title {
  font-size: var(--ax-text-2xl);
  letter-spacing: var(--ax-tracking-display);
}

.page-header__desc {
  margin-top: var(--ax-space-1);
  max-width: 60ch;
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
</style>

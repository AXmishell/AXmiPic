<script setup lang="ts">
import type { Component } from 'vue'

defineProps<{
  label: string
  value: string
  hint?: string
  icon?: Component
  tone?: 'accent' | 'success' | 'warning'
}>()
</script>

<template>
  <article class="stat-card">
    <div class="stat-card__top">
      <span class="stat-card__label">{{ label }}</span>
      <span v-if="icon" class="stat-card__icon" :class="`stat-card__icon--${tone ?? 'accent'}`">
        <el-icon :size="15"><component :is="icon" /></el-icon>
      </span>
    </div>
    <p class="stat-card__value">{{ value }}</p>
    <p v-if="hint" class="stat-card__hint">{{ hint }}</p>
    <slot />
  </article>
</template>

<style scoped>
.stat-card {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-2);
  min-width: 0;
  padding: var(--ax-space-5);
  background: rgba(255, 255, 255, 0.02);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-lg);
  box-shadow: var(--ax-ring-inset);
}

.stat-card__top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--ax-space-3);
}

.stat-card__label {
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
  font-weight: var(--ax-weight-medium);
}

.stat-card__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: var(--ax-radius-sm);
}

.stat-card__icon--accent {
  color: #b9bdff;
  background: var(--ax-accent-soft);
}

.stat-card__icon--success {
  color: #6fe08b;
  background: var(--ax-success-soft);
}

.stat-card__icon--warning {
  color: #e8c169;
  background: var(--ax-warning-soft);
}

.stat-card__value {
  color: var(--ax-text);
  font-size: var(--ax-text-2xl);
  font-weight: var(--ax-weight-semibold);
  line-height: 1.15;
  letter-spacing: var(--ax-tracking-display);
  overflow-wrap: anywhere;
}

.stat-card__hint {
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}
</style>

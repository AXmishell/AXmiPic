<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    name: string
    size?: 'sm' | 'md' | 'lg'
  }>(),
  { size: 'md' },
)

const initial = computed(() => {
  const first = Array.from(props.name.trim())[0]
  return first ? first.toUpperCase() : '?'
})
</script>

<template>
  <span class="ax-avatar" :class="`ax-avatar--${size}`" aria-hidden="true">{{ initial }}</span>
</template>

<style scoped>
.ax-avatar {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  color: #b9bdff;
  background: var(--ax-accent-soft);
  border: 1px solid rgba(113, 112, 255, 0.28);
  border-radius: var(--ax-radius-md);
  font-weight: var(--ax-weight-semibold);
  letter-spacing: 0;
  user-select: none;
}

.ax-avatar--sm {
  width: 24px;
  height: 24px;
  font-size: var(--ax-text-xs);
}

.ax-avatar--md {
  width: 32px;
  height: 32px;
  font-size: var(--ax-text-sm);
}

.ax-avatar--lg {
  width: 44px;
  height: 44px;
  font-size: var(--ax-text-lg);
}
</style>

<script setup lang="ts">
import { computed } from 'vue'

import { formatBytes, usagePercent } from '@/utils/format'

const props = withDefaults(
  defineProps<{
    used: number | null | undefined
    quota: number | null | undefined
    compact?: boolean
  }>(),
  { compact: false },
)

const percent = computed(() => usagePercent(props.used, props.quota))
const unlimited = computed(() => !props.quota || props.quota <= 0)
const remaining = computed(() => Math.max(0, (props.quota ?? 0) - (props.used ?? 0)))
const tone = computed(() => (percent.value >= 90 ? 'danger' : percent.value >= 70 ? 'warning' : 'ok'))
const fillStyle = computed(() => ({ transform: `scaleX(${percent.value / 100})` }))
</script>

<template>
  <div class="quota" :class="{ 'quota--compact': compact }">
    <div class="quota__head">
      <span class="quota__label">存储用量</span>
      <span class="quota__numbers">
        <strong>{{ formatBytes(used) }}</strong>
        <span class="quota__sep">/</span>
        <span>{{ unlimited ? '不限' : formatBytes(quota) }}</span>
      </span>
    </div>
    <div
      v-if="!unlimited"
      class="quota__track"
      role="progressbar"
      :aria-valuenow="percent"
      aria-valuemin="0"
      aria-valuemax="100"
      aria-label="存储用量"
    >
      <div class="quota__fill" :class="`quota__fill--${tone}`" :style="fillStyle" />
    </div>
    <p v-if="!compact && !unlimited" class="quota__hint">
      剩余 {{ formatBytes(remaining) }}，已使用 {{ percent }}%
    </p>
    <p v-else-if="!compact" class="quota__hint">当前账号未设置存储配额</p>
  </div>
</template>

<style scoped>
.quota {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-2);
  min-width: 0;
}

.quota__head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--ax-space-2);
  min-width: 0;
}

.quota__label {
  color: var(--ax-text-3);
  font-size: var(--ax-text-xs);
  font-weight: var(--ax-weight-medium);
  letter-spacing: 0.02em;
}

.quota__numbers {
  overflow: hidden;
  color: var(--ax-text-4);
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.quota__numbers strong {
  color: var(--ax-text-2);
  font-weight: var(--ax-weight-medium);
}

.quota__sep {
  margin-inline: 2px;
  color: var(--ax-text-4);
}

.quota__track {
  height: 4px;
  overflow: hidden;
  background: rgba(255, 255, 255, 0.06);
  border-radius: var(--ax-radius-full);
}

.quota__fill {
  width: 100%;
  height: 100%;
  background: var(--ax-accent);
  border-radius: inherit;
  transform-origin: left center;
  transition: transform var(--ax-duration-slow) var(--ax-ease);
}

.quota__fill--warning {
  background: var(--ax-warning);
}

.quota__fill--danger {
  background: var(--ax-danger);
}

.quota__hint {
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.quota--compact .quota__label,
.quota--compact .quota__numbers {
  font-size: 11px;
}
</style>

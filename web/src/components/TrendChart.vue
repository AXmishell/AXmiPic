<script setup lang="ts">
import { computed } from 'vue'

import type { DailyStat } from '@/api/types'
import { formatBytes, formatNumber } from '@/utils/format'

const props = withDefaults(
  defineProps<{
    points: DailyStat[]
    height?: number
  }>(),
  { height: 148 },
)

const maxImages = computed(() => Math.max(1, ...props.points.map((point) => point.images)))
const totalImages = computed(() => props.points.reduce((sum, point) => sum + point.images, 0))
const totalBytes = computed(() => props.points.reduce((sum, point) => sum + point.bytes, 0))
const today = new Date().toLocaleDateString('sv-SE')

function dayLabel(date: string): string {
  if (date === today) return '今天'
  const [, month, day] = date.split('-')
  return `${Number(month)}/${Number(day)}`
}

function barHeight(point: DailyStat): string {
  if (point.images <= 0) return '3px'
  return `${Math.max(8, Math.round((point.images / maxImages.value) * 100))}%`
}

function describe(point: DailyStat): string {
  return `${point.date}：${formatNumber(point.images)} 张，${formatBytes(point.bytes)}`
}
</script>

<template>
  <div class="trend" :style="{ '--trend-height': `${height}px` }">
    <div class="trend__plot">
      <div class="trend__grid" aria-hidden="true" />
      <div class="trend__cols" role="list" :aria-label="`近 ${points.length} 日上传趋势`">
        <div
          v-for="point in points"
          :key="point.date"
          class="trend__col"
          role="listitem"
          tabindex="0"
          :aria-label="describe(point)"
          :title="describe(point)"
          :class="{ 'is-today': point.date === today }"
        >
          <div class="trend__track">
            <span class="trend__bar" :style="{ height: barHeight(point) }" />
            <span class="trend__tooltip" aria-hidden="true">{{ describe(point) }}</span>
          </div>
          <span class="trend__day">{{ dayLabel(point.date) }}</span>
        </div>
      </div>
    </div>
    <p class="trend__summary">
      <span>近 {{ points.length }} 日新增 <strong>{{ formatNumber(totalImages) }}</strong> 张</span>
      <span class="trend__dot" aria-hidden="true" />
      <span>约 {{ formatBytes(totalBytes) }}</span>
    </p>
  </div>
</template>

<style scoped>
.trend {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-3);
}

.trend__plot {
  position: relative;
  height: var(--trend-height);
}

.trend__grid {
  position: absolute;
  inset: 0 0 22px;
  background: repeating-linear-gradient(
    to top,
    transparent 0,
    transparent calc(33.333% - 1px),
    var(--ax-grid-line) calc(33.333% - 1px),
    var(--ax-grid-line) 33.333%
  );
  pointer-events: none;
}

.trend__cols {
  position: absolute;
  inset: 0;
  display: flex;
  gap: var(--ax-space-2);
}

.trend__col {
  display: flex;
  flex: 1 1 0;
  flex-direction: column;
  gap: var(--ax-space-2);
  min-width: 0;
  border-radius: var(--ax-radius-sm);
  outline: none;
}

.trend__col:focus-visible {
  box-shadow: 0 0 0 2px var(--ax-focus);
}

.trend__track {
  position: relative;
  display: flex;
  flex: 1;
  align-items: flex-end;
  justify-content: center;
  min-height: 0;
}

.trend__bar {
  width: min(100%, 34px);
  min-height: 3px;
  background: linear-gradient(180deg, var(--ax-accent-bright), var(--ax-accent));
  border-radius: var(--ax-radius-xs) var(--ax-radius-xs) 2px 2px;
  opacity: 0.72;
  transition:
    opacity var(--ax-duration-fast) var(--ax-ease),
    transform var(--ax-duration-fast) var(--ax-ease);
  transform-origin: bottom;
}

.trend__col:hover .trend__bar,
.trend__col:focus-visible .trend__bar,
.trend__col.is-today .trend__bar {
  opacity: 1;
}

.trend__col:hover .trend__bar {
  transform: scaleY(1.015);
}

.trend__tooltip {
  position: absolute;
  bottom: calc(100% + 6px);
  left: 50%;
  z-index: 1;
  padding: 4px 8px;
  color: var(--ax-text-2);
  font-size: var(--ax-text-xs);
  white-space: nowrap;
  background: var(--ax-elevated);
  border: 1px solid var(--ax-border);
  border-radius: var(--ax-radius-sm);
  box-shadow: var(--ax-shadow-sm);
  opacity: 0;
  pointer-events: none;
  transform: translate(-50%, 4px);
  transition:
    opacity var(--ax-duration-fast) var(--ax-ease),
    transform var(--ax-duration-fast) var(--ax-ease);
}

.trend__col:hover .trend__tooltip,
.trend__col:focus-visible .trend__tooltip {
  opacity: 1;
  transform: translate(-50%, 0);
}

.trend__day {
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
  text-align: center;
  font-variant-numeric: tabular-nums;
}

.trend__col.is-today .trend__day {
  color: var(--ax-text-2);
  font-weight: var(--ax-weight-medium);
}

.trend__summary {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--ax-space-2);
  color: var(--ax-text-3);
  font-size: var(--ax-text-xs);
}

.trend__summary strong {
  color: var(--ax-text);
  font-weight: var(--ax-weight-semibold);
  font-variant-numeric: tabular-nums;
}

.trend__dot {
  width: 3px;
  height: 3px;
  background: var(--ax-text-4);
  border-radius: var(--ax-radius-full);
}

@media (max-width: 480px) {
  .trend__bar {
    width: min(100%, 22px);
  }
}
</style>

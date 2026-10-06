<script setup lang="ts">
import { computed } from 'vue'
import { Picture } from '@element-plus/icons-vue'

import type { ImageItem } from '@/api/types'
import { formatBytes, formatDateTime, formatDimensions, formatMime } from '@/utils/format'

const props = defineProps<{ item: ImageItem; masonry?: boolean; failed?: boolean }>()
const emit = defineEmits<{ open: [item: ImageItem]; error: [id: string] }>()

const name = computed(() => props.item.original_name || props.item.filename || props.item.key)
const thumbStyle = computed(() =>
  props.masonry && props.item.width > 0 && props.item.height > 0
    ? { aspectRatio: `${props.item.width} / ${props.item.height}` }
    : {},
)
</script>

<template>
  <article class="pc" :class="{ 'pc--masonry': masonry }" @click="emit('open', item)">
    <div class="pc__thumb" :style="thumbStyle">
      <div
        v-if="!masonry && !failed"
        class="pc__blur"
        :style="{ backgroundImage: `url(${item.thumbnail || item.url})` }"
        aria-hidden="true"
      />
      <img
        v-if="!failed"
        class="pc__img"
        :src="item.thumbnail || item.url"
        loading="lazy"
        :alt="name"
        @error="emit('error', item.id)"
      />
      <span v-else class="pc__fallback">
        <el-icon :size="20"><Picture /></el-icon>
        <span>预览不可用</span>
      </span>

      <div class="pc__overlay">
        <div class="pc__head">
          <p class="pc__name" :title="name">{{ name }}</p>
          <p class="pc__author">上传者：{{ item.owner_username || '匿名' }}</p>
        </div>
        <dl class="pc__meta">
          <div class="pc__meta-row">
            <dt>尺寸</dt>
            <dd>{{ formatDimensions(item.width, item.height) }}</dd>
          </div>
          <div class="pc__meta-row">
            <dt>大小</dt>
            <dd>{{ formatBytes(item.size) }}</dd>
          </div>
          <div class="pc__meta-row">
            <dt>格式</dt>
            <dd>{{ formatMime(item.mime_type) }}</dd>
          </div>
          <div class="pc__meta-row">
            <dt>上传</dt>
            <dd>{{ formatDateTime(item.created_at) }}</dd>
          </div>
        </dl>
        <div class="pc__actions" @click.stop>
          <slot name="actions" :item="item" />
        </div>
      </div>
    </div>
  </article>
</template>

<style scoped>
.pc {
  position: relative;
  overflow: hidden;
  background: var(--ax-tint-weak);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-lg);
  cursor: zoom-in;
  transition: border-color var(--ax-duration) var(--ax-ease);
}

.pc:hover {
  border-color: var(--ax-border);
}

.pc__thumb {
  position: relative;
  display: block;
  height: 220px;
  overflow: hidden;
  background: var(--ax-tint);
}

/* 瀑布流：高度由原图比例决定，不固定 */
.pc--masonry .pc__thumb {
  height: auto;
}

.pc__blur {
  position: absolute;
  /* 外扩距离大于模糊半径，避免模糊渐隐的透明边露出容器底色。 */
  inset: -40px;
  background-position: center;
  background-size: cover;
  filter: blur(28px) brightness(0.85) saturate(1.05);
}

.pc__img {
  position: relative;
  display: block;
  width: 100%;
  height: 100%;
  object-fit: contain;
  transition: transform var(--ax-duration-slow) var(--ax-ease);
}

.pc--masonry .pc__img {
  object-fit: cover;
}

.pc:hover .pc__img {
  transform: scale(1.02);
}

.pc__fallback {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--ax-space-2);
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.pc__overlay {
  position: absolute;
  inset: 0;
  z-index: 1;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  gap: var(--ax-space-2);
  padding: var(--ax-space-3);
  color: #fff;
  background: linear-gradient(
    to bottom,
    rgba(0, 0, 0, 0.55) 0%,
    rgba(0, 0, 0, 0.12) 38%,
    rgba(0, 0, 0, 0.8) 100%
  );
  opacity: 0;
  pointer-events: none;
  transition: opacity var(--ax-duration) var(--ax-ease);
}

.pc:hover .pc__overlay,
.pc:focus-within .pc__overlay {
  opacity: 1;
  pointer-events: auto;
}

.pc__head {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-1);
  min-width: 0;
}

.pc__name {
  margin: 0;
  overflow: hidden;
  color: #fff;
  font-size: var(--ax-text-sm);
  font-weight: var(--ax-weight-medium);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pc__author {
  margin: 0;
  overflow: hidden;
  color: rgba(255, 255, 255, 0.75);
  font-size: var(--ax-text-xs);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pc__meta {
  display: grid;
  gap: 3px;
  margin: 0;
}

.pc__meta-row {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--ax-space-3);
  min-width: 0;
}

.pc__meta-row dt {
  flex: none;
  color: rgba(255, 255, 255, 0.6);
  font-size: var(--ax-text-xs);
}

.pc__meta-row dd {
  margin: 0;
  overflow: hidden;
  color: rgba(255, 255, 255, 0.92);
  font-size: var(--ax-text-xs);
  font-variant-numeric: tabular-nums;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pc__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--ax-space-1);
}

.pc__actions :deep(.el-button) {
  flex: 1;
  min-width: 0;
  margin-left: 0;
}
</style>

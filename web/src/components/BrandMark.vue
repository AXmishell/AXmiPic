<script setup lang="ts">
import { useId } from 'vue'

/**
 * AXmiPic 品牌标志（图形部分）。
 *
 * 两张叠放的照片 + 山峦与太阳，呼应图床/图库的定位。此处只绘制图形本体，
 * 不含深色底片，以便放进各处已有的 `accent-soft` 徽标容器里；带底片的应用
 * 图标版本见 `public/favicon.svg`。
 */
withDefaults(
  defineProps<{
    /** 渲染尺寸（px） */
    size?: number | string
  }>(),
  { size: 18 },
)

const uid = useId()
const gradientId = `axmipic-mark-g-${uid}`
const clipId = `axmipic-mark-c-${uid}`
</script>

<template>
  <svg
    class="brand-mark"
    viewBox="0 0 32 32"
    :width="size"
    :height="size"
    role="img"
    aria-label="AXmiPic"
  >
    <defs>
      <linearGradient :id="gradientId" x1="0" y1="0" x2="1" y2="1">
        <stop offset="0" stop-color="#828fff" />
        <stop offset="1" stop-color="#5e6ad2" />
      </linearGradient>
      <clipPath :id="clipId">
        <rect x="8.5" y="9.5" width="16.5" height="16.5" rx="4.2" />
      </clipPath>
    </defs>

    <g transform="translate(16 16) scale(1.12) translate(-16.5 -17.5)">
      <!-- 后一张照片：倾斜叠放，形成图库/图床的层次 -->
      <rect
        x="8.5"
        y="9.5"
        width="16.5"
        height="16.5"
        rx="4.2"
        :fill="`url(#${gradientId})`"
        opacity="0.45"
        transform="rotate(-13 16.75 17.75)"
      />
      <!-- 前一张照片：山峦与太阳，代表被托管的图片 -->
      <g :clip-path="`url(#${clipId})`">
        <rect x="8.5" y="9.5" width="16.5" height="16.5" :fill="`url(#${gradientId})`" />
        <circle cx="20.6" cy="14" r="2.1" fill="#f7f8f8" />
        <path d="M6.5 27 L13.6 18.2 L17.2 22.2 L19.2 19.8 L27 27 Z" fill="#0f1011" />
      </g>
      <!-- 前后照片之间的分隔 -->
      <rect
        x="9.25"
        y="10.25"
        width="15"
        height="15"
        rx="3.5"
        fill="none"
        stroke="#0f1011"
        stroke-width="1.3"
      />
    </g>
  </svg>
</template>

<style scoped>
.brand-mark {
  display: block;
  flex: none;
}
</style>

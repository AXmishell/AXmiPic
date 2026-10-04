<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'

import { toApiError } from '@/api/client'
import { getPage } from '@/api/site'
import type { Page } from '@/api/types'

const route = useRoute()
const slug = computed(() => String(route.params.slug ?? ''))

const page = ref<Page | null>(null)
const loading = ref(true)
const errorMessage = ref('')

// 轻量 Markdown 渲染：转义 HTML，支持标题、列表、粗体、行内代码、链接与段落。
function escapeHtml(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

function renderMarkdown(source: string): string {
  const lines = escapeHtml(source).replace(/\r\n/g, '\n').split('\n')
  const html: string[] = []
  let inList = false
  const closeList = (): void => {
    if (inList) {
      html.push('</ul>')
      inList = false
    }
  }
  for (const raw of lines) {
    const line = raw.trimEnd()
    if (line === '') {
      closeList()
      continue
    }
    const heading = /^(#{1,6})\s+(.*)$/.exec(line)
    if (heading) {
      closeList()
      const level = heading[1]!.length
      html.push(`<h${level}>${inline(heading[2])}</h${level}>`)
      continue
    }
    const bullet = /^[-*]\s+(.*)$/.exec(line)
    if (bullet) {
      if (!inList) {
        html.push('<ul>')
        inList = true
      }
      html.push(`<li>${inline(bullet[1])}</li>`)
      continue
    }
    closeList()
    html.push(`<p>${inline(line)}</p>`)
  }
  closeList()
  return html.join('\n')
}

function inline(text: string): string {
  return text
    .replace(/`([^`]+)`/g, '<code>$1</code>')
    .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
    .replace(/\*([^*]+)\*/g, '<em>$1</em>')
    .replace(/\[([^\]]+)\]\((https?:[^)\s]+)\)/g, '<a href="$2" rel="noopener noreferrer">$1</a>')
}

const rendered = computed(() => (page.value ? renderMarkdown(page.value.content) : ''))

async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    page.value = await getPage(slug.value)
  } catch (error) {
    errorMessage.value = toApiError(error).message
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <main class="page-view">
    <article class="page-view__card">
      <div v-if="loading" class="page-view__state">正在加载…</div>
      <div v-else-if="errorMessage" class="page-view__state page-view__state--error">{{ errorMessage }}</div>
      <template v-else-if="page">
        <h1 class="page-view__title">{{ page.title }}</h1>
        <!-- eslint-disable-next-line vue/no-v-html -->
        <div class="page-view__content" v-html="rendered" />
      </template>
    </article>
  </main>
</template>

<style scoped>
.page-view {
  min-height: 100dvh;
  padding: clamp(var(--ax-space-4), 5vw, var(--ax-space-7));
  background: var(--ax-bg);
}

.page-view__card {
  max-width: 760px;
  margin: 0 auto;
  padding: clamp(var(--ax-space-4), 4vw, var(--ax-space-6));
  background: var(--ax-panel);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-lg);
  box-shadow: var(--ax-shadow-md);
}

.page-view__state {
  padding: var(--ax-space-7) 0;
  color: var(--ax-text-3);
  text-align: center;
}

.page-view__state--error {
  color: var(--ax-danger);
}

.page-view__title {
  margin: 0 0 var(--ax-space-4);
  color: var(--ax-text);
  font-size: var(--ax-text-2xl);
}

.page-view__content {
  color: var(--ax-text-2);
  font-size: var(--ax-text-base);
  line-height: 1.75;
}

.page-view__content :deep(h1),
.page-view__content :deep(h2),
.page-view__content :deep(h3) {
  margin: var(--ax-space-5) 0 var(--ax-space-2);
  color: var(--ax-text);
}

.page-view__content :deep(p) {
  margin: 0 0 var(--ax-space-3);
}

.page-view__content :deep(ul) {
  margin: 0 0 var(--ax-space-3);
  padding-left: var(--ax-space-5);
}

.page-view__content :deep(code) {
  padding: 1px 5px;
  background: var(--ax-tint-weak);
  border-radius: var(--ax-radius-xs);
  font-family: var(--ax-font-mono);
  font-size: 0.9em;
}

.page-view__content :deep(a) {
  color: var(--ax-accent-hover);
}
</style>

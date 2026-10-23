<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { HomeFilled, Management, Search, Setting, User } from '@element-plus/icons-vue'

import { useAuthStore } from '@/stores/auth'
import { navGroupsFor, type NavItem } from '@/utils/navigation'

interface CommandItem extends NavItem {
  id: string
  groupKey: string
}

interface PaletteGroup {
  labelKey: string
  items: CommandItem[]
}

const { t } = useI18n()
const auth = useAuthStore()
const route = useRoute()
const router = useRouter()

const open = ref(false)
const query = ref('')
const activeIndex = ref(0)
const inputRef = ref<HTMLInputElement | null>(null)
let lastFocused: HTMLElement | null = null

const variant = computed<'user' | 'admin'>(() =>
  route.path.startsWith('/admin') ? 'admin' : 'user',
)

const extras = computed<NavItem[]>(() => {
  const items: NavItem[] = [
    { labelKey: 'nav.items.home', to: '/', icon: HomeFilled, keywords: 'home 首页' },
    {
      labelKey: 'nav.items.settings',
      to: variant.value === 'admin' ? '/admin/settings' : '/user/settings',
      icon: Setting,
      keywords: 'settings 设置 密码 邮箱 二次验证',
    },
  ]
  if (auth.isAdmin) {
    items.push(
      variant.value === 'admin'
        ? { labelKey: 'nav.items.userCenterShort', to: '/user', icon: User, keywords: 'user center 用户中心' }
        : { labelKey: 'nav.items.adminConsole', to: '/admin', icon: Management, keywords: 'admin 管理控制台' },
    )
  }
  return items
})

/** 搜索索引：侧栏导航 + 快捷操作，保证与菜单同源。 */
const visibleGroups = computed<PaletteGroup[]>(() => {
  const keyword = query.value.trim().toLowerCase()
  const source = [
    ...navGroupsFor(variant.value),
    { labelKey: 'nav.items.shortcuts', items: extras.value },
  ]
  return source
    .map((group) => ({
      labelKey: group.labelKey,
      items: group.items
        .filter((item) => {
          if (!keyword) return true
          return `${t(item.labelKey)} ${t(group.labelKey)} ${item.keywords ?? ''}`
            .toLowerCase()
            .includes(keyword)
        })
        .map((item) => ({
          ...item,
          id: `${group.labelKey}:${item.to}`,
          groupKey: group.labelKey,
        })),
    }))
    .filter((group) => group.items.length > 0)
})

const flat = computed<CommandItem[]>(() => visibleGroups.value.flatMap((group) => group.items))

function indexOf(item: CommandItem): number {
  return flat.value.findIndex((candidate) => candidate.id === item.id)
}

function isActive(item: CommandItem): boolean {
  return flat.value[activeIndex.value]?.id === item.id
}

watch(query, () => {
  activeIndex.value = 0
})

watch(activeIndex, async () => {
  await nextTick()
  document.getElementById(`palette-item-${activeIndex.value}`)?.scrollIntoView({ block: 'nearest' })
})

function show(): void {
  if (open.value) return
  lastFocused = document.activeElement instanceof HTMLElement ? document.activeElement : null
  open.value = true
  query.value = ''
  activeIndex.value = 0
  document.body.style.overflow = 'hidden'
  void nextTick(() => inputRef.value?.focus())
}

function close(): void {
  if (!open.value) return
  open.value = false
  document.body.style.overflow = ''
  lastFocused?.focus()
}

function move(delta: number): void {
  const total = flat.value.length
  if (total === 0) return
  activeIndex.value = (activeIndex.value + delta + total) % total
}

function go(item: CommandItem | undefined): void {
  if (!item) return
  close()
  if (route.path !== item.to) void router.push(item.to)
}

function onKeydown(event: KeyboardEvent): void {
  switch (event.key) {
    case 'ArrowDown':
      event.preventDefault()
      move(1)
      break
    case 'ArrowUp':
      event.preventDefault()
      move(-1)
      break
    case 'Home':
      event.preventDefault()
      activeIndex.value = 0
      break
    case 'End':
      event.preventDefault()
      activeIndex.value = Math.max(0, flat.value.length - 1)
      break
    case 'Enter':
      event.preventDefault()
      go(flat.value[activeIndex.value])
      break
    case 'Escape':
      event.preventDefault()
      close()
      break
  }
}

function onGlobalKeydown(event: KeyboardEvent): void {
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
    event.preventDefault()
    if (open.value) close()
    else show()
  }
}

onMounted(() => window.addEventListener('keydown', onGlobalKeydown))
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onGlobalKeydown)
  document.body.style.overflow = ''
})

defineExpose({ show })
</script>

<template>
  <Teleport to="body">
    <Transition name="palette">
      <div v-if="open" class="palette" @click.self="close">
        <div class="palette__panel" role="dialog" aria-modal="true" :aria-label="t('commandPalette.dialogLabel')">
          <div class="palette__field">
            <el-icon :size="16" class="palette__search-icon"><Search /></el-icon>
            <input
              ref="inputRef"
              v-model="query"
              class="palette__input"
              type="text"
              :placeholder="t('commandPalette.placeholder')"
              role="combobox"
              aria-controls="palette-results"
              :aria-expanded="open"
              :aria-activedescendant="flat.length > 0 ? `palette-item-${activeIndex}` : undefined"
              @keydown="onKeydown"
            />
            <kbd class="palette__esc">Esc</kbd>
          </div>

          <div id="palette-results" class="palette__results" role="listbox">
            <p v-if="flat.length === 0" class="palette__empty">{{ t('commandPalette.empty') }}</p>
            <template v-else>
              <div v-for="group in visibleGroups" :key="group.labelKey" class="palette__group">
                <p class="palette__group-label">{{ t(group.labelKey) }}</p>
                <button
                  v-for="item in group.items"
                  :id="`palette-item-${indexOf(item)}`"
                  :key="item.id"
                  type="button"
                  class="palette__item"
                  :class="{ 'is-active': isActive(item) }"
                  role="option"
                  :aria-selected="isActive(item)"
                  @mouseenter="activeIndex = indexOf(item)"
                  @click="go(item)"
                >
                  <el-icon :size="15" class="palette__item-icon">
                    <component :is="item.icon" />
                  </el-icon>
                  <span class="palette__item-label">{{ t(item.labelKey) }}</span>
                  <span class="palette__item-hint">{{ item.to }}</span>
                </button>
              </div>
            </template>
          </div>

          <footer class="palette__footer">
            <span><kbd>↑</kbd><kbd>↓</kbd> {{ t('commandPalette.selectHint') }}</span>
            <span><kbd>Enter</kbd> {{ t('commandPalette.openHint') }}</span>
            <span><kbd>Esc</kbd> {{ t('common.close') }}</span>
          </footer>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.palette {
  position: fixed;
  inset: 0;
  z-index: 2500;
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding: 12vh var(--ax-space-4) var(--ax-space-4);
  background: var(--ax-scrim);
  backdrop-filter: blur(6px);
}

.palette__panel {
  display: flex;
  flex-direction: column;
  width: min(560px, 100%);
  max-height: min(60vh, 520px);
  overflow: hidden;
  background: var(--ax-elevated);
  border: 1px solid var(--ax-border);
  border-radius: var(--ax-radius-lg);
  box-shadow: var(--ax-shadow-lg);
}

.palette__field {
  display: flex;
  align-items: center;
  gap: var(--ax-space-3);
  padding: var(--ax-space-3) var(--ax-space-4);
  border-bottom: 1px solid var(--ax-border-subtle);
}

.palette__search-icon {
  flex: none;
  color: var(--ax-text-3);
}

.palette__input {
  flex: 1;
  min-width: 0;
  padding: 4px 0;
  color: var(--ax-text);
  background: transparent;
  border: 0;
  outline: none;
  font-family: inherit;
  font-size: var(--ax-text-md);
}

.palette__input::placeholder {
  color: var(--ax-text-4);
}

.palette__esc {
  flex: none;
}

.palette__results {
  flex: 1;
  min-height: 0;
  padding: var(--ax-space-2);
  overflow-y: auto;
}

.palette__group + .palette__group {
  margin-top: var(--ax-space-2);
}

.palette__group-label {
  padding: var(--ax-space-2) var(--ax-space-3) var(--ax-space-1);
  color: var(--ax-text-4);
  font-size: var(--ax-text-2xs);
  font-weight: var(--ax-weight-medium);
  letter-spacing: 0.06em;
}

.palette__item {
  display: flex;
  align-items: center;
  gap: var(--ax-space-3);
  width: 100%;
  padding: 9px var(--ax-space-3);
  color: var(--ax-text-2);
  text-align: left;
  background: transparent;
  border: 0;
  border-radius: var(--ax-radius-sm);
  cursor: pointer;
}

.palette__item.is-active {
  color: var(--ax-text);
  background: var(--ax-accent-soft);
}

.palette__item-icon {
  flex: none;
  color: var(--ax-text-3);
}

.palette__item.is-active .palette__item-icon {
  color: var(--ax-accent-hover);
}

.palette__item-label {
  flex: 1;
  min-width: 0;
  font-size: var(--ax-text-sm);
  font-weight: var(--ax-weight-medium);
}

.palette__item-hint {
  flex: none;
  color: var(--ax-text-4);
  font-family: var(--ax-font-mono);
  font-size: var(--ax-text-2xs);
}

.palette__empty {
  padding: var(--ax-space-8) var(--ax-space-4);
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
  text-align: center;
}

.palette__footer {
  display: flex;
  flex-wrap: wrap;
  gap: var(--ax-space-4);
  padding: var(--ax-space-2) var(--ax-space-4);
  border-top: 1px solid var(--ax-border-subtle);
  color: var(--ax-text-4);
  font-size: var(--ax-text-2xs);
}

kbd {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  margin-right: 2px;
  color: var(--ax-text-3);
  background: var(--ax-tint);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-xs);
  font-family: var(--ax-font-mono);
  font-size: 10px;
}

.palette-enter-active,
.palette-leave-active {
  transition: opacity var(--ax-duration) var(--ax-ease);
}

.palette-enter-active .palette__panel,
.palette-leave-active .palette__panel {
  transition: transform var(--ax-duration) var(--ax-ease);
}

.palette-enter-from,
.palette-leave-to {
  opacity: 0;
}

.palette-enter-from .palette__panel,
.palette-leave-to .palette__panel {
  transform: translateY(-8px) scale(0.98);
}

@media (max-width: 640px) {
  .palette {
    padding-top: 8vh;
  }

  .palette__item-hint {
    display: none;
  }
}
</style>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRoute } from 'vue-router'

import AppSidebar from '@/components/AppSidebar.vue'
import AppTopbar from '@/components/AppTopbar.vue'
import CommandPalette from '@/components/CommandPalette.vue'

withDefaults(defineProps<{ variant?: 'user' | 'admin' }>(), { variant: 'admin' })

const route = useRoute()
const drawerOpen = ref(false)
const collapsed = ref(localStorage.getItem('ax.sidebar.collapsed') === '1')
const mainRef = ref<HTMLElement | null>(null)
const paletteRef = ref<{ show: () => void } | null>(null)

watch(collapsed, (value) => {
  localStorage.setItem('ax.sidebar.collapsed', value ? '1' : '0')
})

function toggleSidebar(): void {
  collapsed.value = !collapsed.value
}

watch(
  () => route.fullPath,
  () => {
    drawerOpen.value = false
    // 主滚动容器是 .app-main，而不是 window；路由切换时必须手动回到顶部，
    // 否则从长页面跳到短页面会停在半空。
    if (mainRef.value) mainRef.value.scrollTop = 0
  },
)
</script>

<template>
  <div class="app-shell" :class="{ 'is-collapsed': collapsed }">
    <a class="skip-link" href="#main-content">跳到主要内容</a>

    <AppSidebar
      :open="drawerOpen"
      :variant="variant"
      :collapsed="collapsed"
      @close="drawerOpen = false"
    />

    <div class="app-frame">
      <AppTopbar
        :sidebar-collapsed="collapsed"
        @toggle-menu="drawerOpen = !drawerOpen"
        @toggle-sidebar="toggleSidebar"
        @open-search="paletteRef?.show()"
      />
      <main id="main-content" ref="mainRef" class="app-main" tabindex="-1">
        <div class="app-main__inner" :class="{ 'app-main__inner--wide': variant === 'admin' }">
          <router-view />
        </div>
      </main>
    </div>

    <transition name="scrim">
      <div v-if="drawerOpen" class="app-scrim" @click="drawerOpen = false" />
    </transition>

    <CommandPalette ref="paletteRef" />
  </div>
</template>

<style scoped>
.app-shell {
  display: grid;
  grid-template-columns: var(--ax-sidebar-w) minmax(0, 1fr);
  height: 100dvh;
  transition: grid-template-columns var(--ax-duration) var(--ax-ease);
}

.app-shell.is-collapsed {
  grid-template-columns: var(--ax-sidebar-collapsed-w) minmax(0, 1fr);
}

.app-frame {
  display: grid;
  grid-template-rows: var(--ax-topbar-h) minmax(0, 1fr);
  min-width: 0;
  height: 100dvh;
}

.app-main {
  min-block-size: 0;
  overflow-y: auto;
  scroll-behavior: smooth;
}

.app-main:focus {
  outline: none;
}

.app-main__inner {
  max-width: var(--ax-content-max);
  margin-inline: auto;
  padding: var(--ax-space-6) var(--ax-space-8) var(--ax-space-12);
}

.app-main__inner--wide {
  max-width: var(--ax-content-max-wide);
}

.skip-link {
  position: fixed;
  top: var(--ax-space-2);
  left: var(--ax-space-2);
  z-index: calc(var(--ax-z-drawer) + 1);
  padding: var(--ax-space-2) var(--ax-space-4);
  background: var(--ax-elevated);
  border: 1px solid var(--ax-border);
  border-radius: var(--ax-radius-sm);
  color: var(--ax-text);
  font-size: var(--ax-text-sm);
  transform: translateY(-200%);
}

.skip-link:focus-visible {
  transform: none;
}

.app-scrim {
  position: fixed;
  inset: 0;
  z-index: var(--ax-z-scrim);
  background: var(--ax-scrim);
}

.scrim-enter-active,
.scrim-leave-active {
  transition: opacity var(--ax-duration) var(--ax-ease);
}

.scrim-enter-from,
.scrim-leave-to {
  opacity: 0;
}

@media (max-width: 960px) {
  .app-shell,
  .app-shell.is-collapsed {
    grid-template-columns: minmax(0, 1fr);
  }

  .app-main__inner {
    padding: var(--ax-space-5) var(--ax-space-4) var(--ax-space-10);
  }
}
</style>

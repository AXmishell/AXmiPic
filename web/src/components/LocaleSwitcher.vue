<script setup lang="ts">
import { useI18n } from 'vue-i18n'

import { useLocaleStore } from '@/stores/locale'

withDefaults(defineProps<{ variant?: 'inline' | 'floating' }>(), { variant: 'inline' })

const { t } = useI18n()
const localeStore = useLocaleStore()

function handleCommand(value: string | number | object): void {
  if (typeof value === 'string') localeStore.setLocale(value)
}
</script>

<template>
  <el-dropdown trigger="click" placement="bottom-end" @command="handleCommand">
    <button
      class="locale-switcher"
      :class="`locale-switcher--${variant}`"
      type="button"
      :aria-label="t('locale.switch')"
      :title="t('locale.switch')"
    >
      <span class="locale-switcher__short">{{ localeStore.current.short }}</span>
    </button>
    <template #dropdown>
      <el-dropdown-menu>
        <el-dropdown-item
          v-for="option in localeStore.options"
          :key="option.value"
          :command="option.value"
          :disabled="option.value === localeStore.locale"
        >
          <span class="locale-switcher__option">
            <span>{{ option.label }}</span>
            <span class="locale-switcher__code">{{ option.value }}</span>
          </span>
        </el-dropdown-item>
      </el-dropdown-menu>
    </template>
  </el-dropdown>
</template>

<style scoped>
.locale-switcher {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 32px;
  height: 32px;
  padding: 0 8px;
  color: var(--ax-text-3);
  background: var(--ax-tint);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-sm);
  cursor: pointer;
  transition:
    color var(--ax-duration-fast) var(--ax-ease),
    background-color var(--ax-duration-fast) var(--ax-ease);
}

.locale-switcher:hover {
  color: var(--ax-text);
  background: var(--ax-tint-strong);
}

.locale-switcher__short {
  font-family: var(--ax-font-mono);
  font-size: var(--ax-text-xs);
  font-weight: var(--ax-weight-semibold);
  letter-spacing: 0.02em;
  line-height: 1;
}

.locale-switcher--floating {
  position: fixed;
  right: var(--ax-space-4);
  bottom: var(--ax-space-4);
  z-index: var(--ax-z-drawer);
  min-width: 36px;
  height: 36px;
  background: var(--ax-panel);
  border-color: var(--ax-border);
  border-radius: var(--ax-radius-full);
  box-shadow: var(--ax-shadow-sm, 0 2px 8px rgb(0 0 0 / 12%));
  backdrop-filter: blur(12px);
}

.locale-switcher__option {
  display: inline-flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--ax-space-6);
  min-width: 140px;
}

.locale-switcher__code {
  color: var(--ax-text-4);
  font-family: var(--ax-font-mono);
  font-size: var(--ax-text-2xs);
}
</style>

import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

export type ThemeMode = 'dark' | 'light'

const STORAGE_KEY = 'axmipic.theme.v1'

function readStored(): ThemeMode | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw === 'dark' || raw === 'light') return raw
    return null
  } catch {
    return null
  }
}

function applyTheme(mode: ThemeMode): void {
  const root = document.documentElement
  root.classList.toggle('theme-dark', mode === 'dark')
  root.classList.toggle('theme-light', mode === 'light')
// Element Plus 会读取 `dark` 类以使用其内置的深色变量。
  root.classList.toggle('dark', mode === 'dark')
  root.style.colorScheme = mode
}

/**
 * 主题 store。默认深色；已存储的偏好或系统的 `prefers-color-scheme`
 * 在首次加载时优先生效。解析出的模式同时驱动 AX 令牌类与 Element Plus 的
 * `dark` 类。
 */
export const useThemeStore = defineStore('theme', () => {
  const stored = readStored()
  const prefersLight =
    typeof window !== 'undefined' &&
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-color-scheme: light)').matches
  const mode = ref<ThemeMode>(stored ?? (prefersLight ? 'light' : 'dark'))

  const isDark = computed(() => mode.value === 'dark')

  function persist(): void {
    try {
      localStorage.setItem(STORAGE_KEY, mode.value)
    } catch {
      // 存储可能不可用（隐私模式）；主题仅保留在内存中。
    }
  }

  function setMode(next: ThemeMode): void {
    mode.value = next
    applyTheme(next)
    persist()
  }

  function toggle(): void {
    setMode(mode.value === 'dark' ? 'light' : 'dark')
  }

  /** 应用初始主题；在挂载前调用一次。 */
  function init(): void {
    applyTheme(mode.value)
  }

  return { mode, isDark, init, setMode, toggle }
})

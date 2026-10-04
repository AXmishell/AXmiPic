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
  // Element Plus reads the `dark` class for its built-in dark variables.
  root.classList.toggle('dark', mode === 'dark')
  root.style.colorScheme = mode
}

/**
 * Theme store. Dark is the default; a stored preference or the OS
 * `prefers-color-scheme` wins on first load. The resolved mode drives both the
 * AX token classes and Element Plus's `dark` class.
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
      // Storage can be unavailable (private mode); theme stays in memory.
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

  /** Applies the initial theme; call once before mounting. */
  function init(): void {
    applyTheme(mode.value)
  }

  return { mode, isDark, init, setMode, toggle }
})

import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import {
  LOCALE_OPTIONS,
  detectLocale,
  i18n,
  isAppLocale,
  setI18nLocale,
  type AppLocale,
} from '@/i18n'

/** 界面语言 store：负责持久化、同步 <html lang> 与 Element Plus 语言包。 */
export const useLocaleStore = defineStore('locale', () => {
  const locale = ref<AppLocale>(detectLocale())

  const current = computed(
    () => LOCALE_OPTIONS.find((option) => option.value === locale.value) ?? LOCALE_OPTIONS[0],
  )

  function setLocale(next: string): void {
    if (!isAppLocale(next) || next === locale.value) return
    locale.value = next
    setI18nLocale(next)
  }

  function toggle(): void {
    setLocale(locale.value === 'zh-CN' ? 'en-US' : 'zh-CN')
  }

  /** 在挂载前应用语言，避免首屏文案闪烁。 */
  function init(): void {
    setI18nLocale(locale.value)
  }

  return {
    locale,
    current,
    options: LOCALE_OPTIONS,
    setLocale,
    toggle,
    init,
    i18n,
  }
})

import { createI18n } from 'vue-i18n'

import enUS from './locales/en-US'
import zhCN from './locales/zh-CN'

export const SUPPORTED_LOCALES = ['zh-CN', 'en-US'] as const
export type AppLocale = (typeof SUPPORTED_LOCALES)[number]

export const DEFAULT_LOCALE: AppLocale = 'zh-CN'
export const LOCALE_STORAGE_KEY = 'axmipic.locale.v1'

export interface LocaleOption {
  value: AppLocale
  /** 语言自身的名称，切换菜单中始终使用该名称。 */
  label: string
  /** 紧凑展示用的短标签。 */
  short: string
}

export const LOCALE_OPTIONS: LocaleOption[] = [
  { value: 'zh-CN', label: '简体中文', short: '中' },
  { value: 'en-US', label: 'English', short: 'EN' },
]

export function isAppLocale(value: unknown): value is AppLocale {
  return typeof value === 'string' && (SUPPORTED_LOCALES as readonly string[]).includes(value)
}

function readStoredLocale(): AppLocale | null {
  try {
    const raw = localStorage.getItem(LOCALE_STORAGE_KEY)
    return isAppLocale(raw) ? raw : null
  } catch {
    return null
  }
}

/** 根据浏览器语言推断默认语言；非中文环境使用英文。 */
export function detectLocale(): AppLocale {
  const stored = readStoredLocale()
  if (stored) return stored
  if (typeof navigator === 'undefined') return DEFAULT_LOCALE
  const languages = navigator.languages?.length ? navigator.languages : [navigator.language]
  const preferred = languages.find((value): value is string => typeof value === 'string') ?? ''
  return preferred.toLowerCase().startsWith('zh') ? 'zh-CN' : 'en-US'
}

export const i18n = createI18n({
  legacy: false,
  locale: detectLocale(),
  fallbackLocale: DEFAULT_LOCALE,
  messages: {
    'zh-CN': zhCN,
    'en-US': enUS,
  },
  // 回退到中文时保持安静，避免在英文环境下刷缺失键告警。
  missingWarn: false,
  fallbackWarn: false,
})

/** 切换语言并同步 <html lang> 与本地偏好。 */
export function setI18nLocale(locale: AppLocale): void {
  i18n.global.locale.value = locale
  if (typeof document !== 'undefined') {
    document.documentElement.lang = locale
  }
  try {
    localStorage.setItem(LOCALE_STORAGE_KEY, locale)
  } catch {
    // 隐私模式下仅保留在内存中。
  }
}

/** 当前语言，供非 setup 上下文（工具函数、store）使用。 */
export function getLocale(): AppLocale {
  return isAppLocale(i18n.global.locale.value) ? i18n.global.locale.value : DEFAULT_LOCALE
}

/** 非 setup 上下文中的翻译入口。 */
export function translate(key: string, named?: Record<string, unknown>): string {
  return named ? i18n.global.t(key, named) : i18n.global.t(key)
}

export default i18n

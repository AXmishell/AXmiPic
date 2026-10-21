import { ref, watch, type Ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

/**
 * 把页签状态同步到 URL query：刷新、返回或分享链接时仍停留在同一页签。
 * 非法的 query 值会回退到默认页签。
 */
export function useTabQuery<T extends string>(
  key: string,
  fallback: T,
  valid: readonly T[],
) {
  const route = useRoute()
  const router = useRouter()

  function normalize(value: unknown): T {
    return typeof value === 'string' && (valid as readonly string[]).includes(value)
      ? (value as T)
      : fallback
  }

  const tab = ref<T>(normalize(route.query[key])) as Ref<T>

  watch(tab, (value) => {
    if (route.query[key] === value) return
    void router.replace({ query: { ...route.query, [key]: value } })
  })

  watch(
    () => route.query[key],
    (value) => {
      const next = normalize(value)
      if (next !== tab.value) tab.value = next
    },
  )

  return tab
}

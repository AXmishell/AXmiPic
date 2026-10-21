/** 健康检查中数据库的状态。 */
export interface DatabaseHealth {
  status: 'ok' | 'error' | string
  message?: string
}

/** `GET /healthz` 返回的数据体。 */
export interface HealthStatus {
  status: 'ok' | 'degraded' | string
  database: DatabaseHealth
}

/**
 * 探测服务与数据库健康状态。
 *
 * 使用原生 fetch 而非 api client：`/healthz` 不在 `/api/v1` 前缀下，且需要容忍
 * 非 2xx 响应（数据库异常时后端仍返回 200，但网络层可能直接失败）。
 */
export async function fetchHealthStatus(timeoutMs = 5000): Promise<HealthStatus> {
  const controller = new AbortController()
  const timer = window.setTimeout(() => controller.abort(), timeoutMs)
  try {
    const response = await fetch('/healthz', {
      method: 'GET',
      headers: { Accept: 'application/json' },
      cache: 'no-store',
      signal: controller.signal,
    })
    const body = (await response.json()) as { data?: HealthStatus }
    if (!response.ok || !body?.data) {
      throw new Error('健康检查失败')
    }
    return body.data
  } finally {
    window.clearTimeout(timer)
  }
}

/** 数据库是否处于可用状态。 */
export function isDatabaseHealthy(health: HealthStatus): boolean {
  return health.status === 'ok' && health.database?.status === 'ok'
}

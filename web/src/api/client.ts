import axios, { type AxiosError, type AxiosRequestConfig } from 'axios'
import { ElMessage } from 'element-plus'

import type { ApiEnvelope } from './types'

/** 由传输失败与非零 API 状态码归一化而来的错误结构。 */
export class ApiError extends Error {
  readonly code: number
  readonly status: number | null

  constructor(message: string, code: number, status: number | null) {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.status = status
  }
}

let tokenGetter: () => string | null = () => null
let unauthorizedHandler: (() => void) | null = null

/** 在 main.ts 中一次性接线，使 client 永不导入 store（避免循环依赖）。 */
export function configureAuth(options: {
  getToken: () => string | null
  onUnauthorized: () => void
}): void {
  tokenGetter = options.getToken
  unauthorizedHandler = options.onUnauthorized
}

const http = axios.create({
  baseURL: '/api/v1',
  timeout: 20000,
  headers: { Accept: 'application/json' },
})

http.interceptors.request.use((config) => {
  const token = tokenGetter()
  if (token) {
    config.headers.set('Authorization', `Bearer ${token}`)
  }
  return config
})

http.interceptors.response.use(
  (response) => response,
  (error: AxiosError<ApiEnvelope<unknown>>) => {
    const status = error.response?.status ?? null
    const url = error.config?.url ?? ''
    const isAuthEntry = url.includes('/auth/login') || url.includes('/auth/register')

    if (status === 401 && !isAuthEntry) {
      unauthorizedHandler?.()
    }
    if (status === 403) {
      ElMessage.warning('权限不足')
    }
    return Promise.reject(error)
  },
)

function isEnvelope(value: unknown): value is ApiEnvelope<unknown> {
  return typeof value === 'object' && value !== null && 'code' in value
}

export function toApiError(error: unknown): ApiError {
  if (error instanceof ApiError) return error

  if (axios.isAxiosError(error)) {
    const axiosError = error as AxiosError<ApiEnvelope<unknown>>
    const status = axiosError.response?.status ?? null
    const payload = axiosError.response?.data
    const serverMessage =
      payload && typeof payload === 'object' && typeof payload.message === 'string'
        ? payload.message
        : ''

    let fallback = '网络请求失败，请稍后重试'
    if (axiosError.code === 'ECONNABORTED' || axiosError.code === 'ETIMEDOUT') {
      fallback = '请求超时，请稍后重试'
    } else if (status === 401) {
      fallback = '登录状态已失效，请重新登录'
    } else if (status === 403) {
      fallback = '权限不足'
    } else if (status === 404) {
      fallback = '请求的资源不存在'
    } else if (status !== null && status >= 500) {
      fallback = '服务器暂时不可用，请稍后重试'
    } else if (status === null && axiosError.code === 'ERR_NETWORK') {
      fallback = '无法连接到服务器，请检查网络'
    }

    const code = payload && typeof payload.code === 'number' ? payload.code : -1
    return new ApiError(serverMessage || fallback, code, status)
  }

  return new ApiError(error instanceof Error ? error.message : '未知错误', -1, null)
}

/**
 * 解包 `{ code, message, data }` 信封。成功为 HTTP 2xx 且 `code: 0`；
 * 其余情况会抛出携带服务端消息的 ApiError。
 */
export async function request<T>(config: AxiosRequestConfig): Promise<T> {
  try {
    const response = await http.request<ApiEnvelope<T>>(config)
    const body: unknown = response.data
    if (isEnvelope(body)) {
      if (body.code !== 0) {
        throw new ApiError(body.message || '请求失败', body.code, response.status)
      }
      return body.data as T
    }
    return body as T
  } catch (error) {
    throw toApiError(error)
  }
}

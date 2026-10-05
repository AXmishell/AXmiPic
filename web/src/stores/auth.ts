import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import {
  fetchMe as fetchMeApi,
  fetchPolicies as fetchPoliciesApi,
  adminLogin as adminLoginApi,
  login as loginApi,
  register as registerApi,
  verifyTOTPLogin as verifyTOTPLoginApi,
} from '@/api/auth'
import type { Credentials, EffectivePolicies, LoginResult, RegisterPayload, User } from '@/api/types'

/** 登录结果：直接成功，或需要完成 TOTP 二次验证。 */
export type LoginOutcome =
  | { totpRequired: false; user: User }
  | { totpRequired: true; challengeToken: string }

const STORAGE_KEY = 'axmipic.session.v1'

interface PersistedSession {
  token: string
  expiresAt: string
  user: User
}

function readSession(): PersistedSession | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<PersistedSession>
    if (typeof parsed.token !== 'string' || !parsed.token || !parsed.user) return null
    return {
      token: parsed.token,
      expiresAt: typeof parsed.expiresAt === 'string' ? parsed.expiresAt : '',
      user: parsed.user,
    }
  } catch {
    return null
  }
}

function isExpired(expiresAt: string): boolean {
  if (!expiresAt) return false
  const time = new Date(expiresAt).getTime()
  return Number.isFinite(time) && time <= Date.now()
}

export const useAuthStore = defineStore('auth', () => {
  const token = ref<string | null>(null)
  const expiresAt = ref('')
  const user = ref<User | null>(null)
  const policies = ref<EffectivePolicies | null>(null)
  const hydrated = ref(false)

  const isAuthenticated = computed(() => Boolean(token.value))
  const isAdmin = computed(() => user.value?.role === 'admin')
  const username = computed(() => user.value?.username ?? '')
  const features = computed(() => new Set(policies.value?.features ?? []))

  /** 当前角色是否启用了某个功能开关；策略尚未加载时默认放行。 */
  function hasFeature(name: string): boolean {
    if (!policies.value) return true
    return features.value.has(name)
  }

  /** 拉取当前账户生效的角色策略。 */
  async function loadPolicies(): Promise<EffectivePolicies | null> {
    if (!token.value) return null
    try {
      policies.value = await fetchPoliciesApi()
      return policies.value
    } catch {
      return null
    }
  }

  function persist(): void {
    if (!token.value || !user.value) {
      localStorage.removeItem(STORAGE_KEY)
      return
    }
    try {
      const payload: PersistedSession = {
        token: token.value,
        expiresAt: expiresAt.value,
        user: user.value,
      }
      localStorage.setItem(STORAGE_KEY, JSON.stringify(payload))
    } catch {
      // 存储可能不可用（隐私模式）；会话仅保留在内存中。
    }
  }

  function clearSession(): void {
    token.value = null
    expiresAt.value = ''
    user.value = null
    policies.value = null
    try {
      localStorage.removeItem(STORAGE_KEY)
    } catch {
      // 忽略存储失败。
    }
  }

  /** 仅恢复一次已持久化的会话，并丢弃过期令牌。 */
  function hydrate(): void {
    if (hydrated.value) return
    hydrated.value = true
    const session = readSession()
    if (!session) return
    if (isExpired(session.expiresAt)) {
      clearSession()
      return
    }
    token.value = session.token
    expiresAt.value = session.expiresAt
    user.value = session.user
    void loadPolicies()
  }

  async function login(credentials: Credentials): Promise<LoginOutcome> {
    const result = await loginApi(credentials)
    return handleLoginResult(result)
  }

  /** 通过独立的管理员入口登录。 */
  async function adminLogin(credentials: Credentials): Promise<LoginOutcome> {
    const result = await adminLoginApi(credentials)
    return handleLoginResult(result)
  }

  /** 登录响应可能是直接会话，也可能是 TOTP 挑战。 */
  function handleLoginResult(result: LoginResult): LoginOutcome {
    if (result.totp_required && result.challenge_token) {
      return { totpRequired: true, challengeToken: result.challenge_token }
    }
    return { totpRequired: false, user: applySession(result) }
  }

  /** 完成 TOTP 二次验证并建立会话。 */
  async function verifyTOTP(challengeToken: string, code: string): Promise<User> {
    const result = await verifyTOTPLoginApi(challengeToken, code)
    return applySession(result)
  }

  function applySession(result: LoginResult): User {
    if (!result.token || !result.user) {
      throw new Error('登录响应缺少会话令牌')
    }
    token.value = result.token
    expiresAt.value = result.expires_at ?? ''
    user.value = result.user
    persist()
    void loadPolicies()
    return result.user
  }

  /** 注册账号后使用相同凭证自动登录。 */
  async function register(payload: RegisterPayload): Promise<LoginOutcome> {
    await registerApi(payload)
    return login({ username: payload.username, password: payload.password })
  }

  async function refreshUser(): Promise<User> {
    const me = await fetchMeApi()
    user.value = me
    persist()
    void loadPolicies()
    return me
  }

  function logout(): void {
    clearSession()
  }

  return {
    token,
    expiresAt,
    user,
    policies,
    features,
    hydrated,
    isAuthenticated,
    isAdmin,
    username,
    havePolicies: computed(() => policies.value !== null),
    hydrate,
    login,
    adminLogin,
    register,
    verifyTOTP,
    refreshUser,
    loadPolicies,
    hasFeature,
    logout,
    clearSession,
  }
})

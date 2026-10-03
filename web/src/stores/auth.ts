import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import { fetchMe as fetchMeApi, login as loginApi, register as registerApi } from '@/api/auth'
import type { Credentials, User } from '@/api/types'

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
  const hydrated = ref(false)

  const isAuthenticated = computed(() => Boolean(token.value))
  const isAdmin = computed(() => user.value?.role === 'admin')
  const username = computed(() => user.value?.username ?? '')

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
      // Storage can be unavailable (private mode); session stays in memory.
    }
  }

  function clearSession(): void {
    token.value = null
    expiresAt.value = ''
    user.value = null
    try {
      localStorage.removeItem(STORAGE_KEY)
    } catch {
      // Ignore storage failures.
    }
  }

  /** Restores a persisted session exactly once, dropping expired tokens. */
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
  }

  async function login(credentials: Credentials): Promise<User> {
    const result = await loginApi(credentials)
    token.value = result.token
    expiresAt.value = result.expires_at
    user.value = result.user
    persist()
    return result.user
  }

  /** Registers the account, then signs it in with the same credentials. */
  async function register(credentials: Credentials): Promise<User> {
    await registerApi(credentials)
    return login(credentials)
  }

  async function refreshUser(): Promise<User> {
    const me = await fetchMeApi()
    user.value = me
    persist()
    return me
  }

  function logout(): void {
    clearSession()
  }

  return {
    token,
    expiresAt,
    user,
    hydrated,
    isAuthenticated,
    isAdmin,
    username,
    hydrate,
    login,
    register,
    refreshUser,
    logout,
    clearSession,
  }
})

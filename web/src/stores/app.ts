import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import { fetchAdminEntryStatus, fetchInstallStatus } from '@/api/auth'

/** 应用级状态：初始化状态、站点信息与管理员安全入口。 */
export const useAppStore = defineStore('app', () => {
  const installed = ref<boolean | null>(null)
  const lockFile = ref('')
  const checking = ref(false)
  // 管理员安全入口：enabled 表示开启；unlocked 表示当前浏览器持有入口 Cookie。
  const adminEntryEnabled = ref(false)
  const adminEntryUnlocked = ref(false)
  const adminEntryChecked = ref(false)
  const adminEntryChecking = ref(false)

  const isInstalled = computed(() => installed.value === true)
  const needsInstall = computed(() => installed.value === false)
  const adminEntryActive = computed(
    () => adminEntryEnabled.value && !adminEntryUnlocked.value,
  )

  /** 拉取初始化状态；失败时保守地视为已初始化。 */
  async function checkInstall(): Promise<boolean> {
    if (checking.value) return installed.value === true
    checking.value = true
    try {
      const status = await fetchInstallStatus()
      installed.value = status.installed
      lockFile.value = status.lock_file ?? ''
      return status.installed
    } catch {
      installed.value = true
      return true
    } finally {
      checking.value = false
    }
  }

  /** 拉取管理员安全入口状态；失败时保持上次结果。 */
  async function checkAdminEntry(force = false): Promise<void> {
    if (adminEntryChecking.value || (adminEntryChecked.value && !force)) return
    adminEntryChecking.value = true
    try {
      const status = await fetchAdminEntryStatus()
      adminEntryEnabled.value = status.enabled
      adminEntryUnlocked.value = status.unlocked
      adminEntryChecked.value = true
    } catch {
      // 状态接口失败不应阻断普通页面访问。
    } finally {
      adminEntryChecking.value = false
    }
  }

  /** 初始化完成后标记状态。 */
  function markInstalled(): void {
    installed.value = true
  }

  return {
    installed,
    lockFile,
    checking,
    adminEntryEnabled,
    adminEntryUnlocked,
    adminEntryChecked,
    adminEntryActive,
    isInstalled,
    needsInstall,
    checkInstall,
    checkAdminEntry,
    markInstalled,
  }
})

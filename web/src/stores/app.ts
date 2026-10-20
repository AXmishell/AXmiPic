import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import { fetchInstallStatus } from '@/api/auth'

/** 应用级状态：初始化状态与站点信息。 */
export const useAppStore = defineStore('app', () => {
  const installed = ref<boolean | null>(null)
  const lockFile = ref('')
  const checking = ref(false)

  const isInstalled = computed(() => installed.value === true)
  const needsInstall = computed(() => installed.value === false)

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

  /** 初始化完成后标记状态。 */
  function markInstalled(): void {
    installed.value = true
  }

  return { installed, lockFile, checking, isInstalled, needsInstall, checkInstall, markInstalled }
})

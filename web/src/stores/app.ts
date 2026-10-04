import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import { fetchInstallStatus } from '@/api/auth'

/** 应用级状态：安装状态与站点信息。 */
export const useAppStore = defineStore('app', () => {
  const installed = ref<boolean | null>(null)
  const lockFile = ref('')
  const checking = ref(false)

  const isInstalled = computed(() => installed.value === true)
  const needsInstall = computed(() => installed.value === false)

  /** 拉取安装状态；失败时保守地视为已安装，避免误跳转到安装向导。 */
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

  /** 安装完成后标记状态。 */
  function markInstalled(): void {
    installed.value = true
  }

  return { installed, lockFile, checking, isInstalled, needsInstall, checkInstall, markInstalled }
})

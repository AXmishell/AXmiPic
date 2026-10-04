import { createApp, type Component } from 'vue'
import { createPinia } from 'pinia'
import {
  ElAlert,
  ElButton,
  ElCheckbox,
  ElConfigProvider,
  ElDialog,
  ElDropdown,
  ElDropdownItem,
  ElDropdownMenu,
  ElForm,
  ElFormItem,
  ElIcon,
  ElInput,
  ElOption,
  ElPagination,
  ElRadioButton,
  ElRadioGroup,
  ElSelect,
  ElSkeleton,
  ElSkeletonItem,
  ElSwitch,
  ElTable,
  ElTableColumn,
  ElTabs,
  ElTabPane,
  ElTag,
  ElTooltip,
} from 'element-plus'

import '@fontsource-variable/inter/wght.css'
import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css'
import '@/styles/tokens.css'
import '@/styles/base.css'
import '@/styles/element.css'

import App from './App.vue'
import router from './router'
import { configureAuth } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import { useThemeStore } from '@/stores/theme'

const app = createApp(App)
const pinia = createPinia()

app.use(pinia)
app.use(router)

// 在挂载前应用已持久化（或系统偏好）的主题。
useThemeStore(pinia).init()

const globalComponents: Component[] = [
  ElAlert,
  ElButton,
  ElCheckbox,
  ElConfigProvider,
  ElDialog,
  ElDropdown,
  ElDropdownItem,
  ElDropdownMenu,
  ElForm,
  ElFormItem,
  ElIcon,
  ElInput,
  ElOption,
  ElPagination,
  ElRadioButton,
  ElRadioGroup,
  ElSelect,
  ElSkeleton,
  ElSkeletonItem,
  ElSwitch,
  ElTable,
  ElTableColumn,
  ElTabs,
  ElTabPane,
  ElTag,
  ElTooltip,
]

for (const component of globalComponents) {
  app.component((component as { name?: string }).name ?? 'UnnamedComponent', component)
}

// store 从不导入 API client 的 store hook；接线在此处完成。
const auth = useAuthStore(pinia)
configureAuth({
  getToken: () => auth.token,
  onUnauthorized: () => {
    if (!auth.isAuthenticated) return
    auth.clearSession()
    const current = router.currentRoute.value
    if (current.name !== 'login') {
      void router.replace({ path: '/login', query: { redirect: current.fullPath } })
    }
  },
})
auth.hydrate()

app.mount('#app')

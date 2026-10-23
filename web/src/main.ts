import { createApp, type Component } from 'vue'
import { createPinia } from 'pinia'
import {
  ElAlert,
  ElButton,
  ElCheckbox,
  ElColorPicker,
  ElConfigProvider,
  ElDatePicker,
  ElDialog,
  ElDivider,
  ElDrawer,
  ElDropdown,
  ElDropdownItem,
  ElDropdownMenu,
  ElForm,
  ElFormItem,
  ElIcon,
  ElImage,
  ElInput,
  ElInputNumber,
  ElOption,
  ElPagination,
  ElProgress,
  ElRadioButton,
  ElRadioGroup,
  ElSelect,
  ElSkeleton,
  ElSkeletonItem,
  ElSlider,
  ElStep,
  ElSteps,
  ElSwitch,
  ElTable,
  ElTableColumn,
  ElTabs,
  ElTabPane,
  ElTag,
  ElTooltip,
  ElUpload,
  ElLoading,
} from 'element-plus'

import '@fontsource-variable/inter/wght.css'
import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css'
import '@/styles/tokens.css'
import '@/styles/base.css'
import '@/styles/element.css'

import App from './App.vue'
import router from './router'
import i18n from '@/i18n'
import { configureAuth } from '@/api/client'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { useLocaleStore } from '@/stores/locale'
import { useThemeStore } from '@/stores/theme'

const app = createApp(App)
const pinia = createPinia()

app.use(pinia)
app.use(i18n)
app.use(router)
// 注册 v-loading 指令；此前只有组件被手动注册，表格上的 v-loading 实际不生效。
app.use(ElLoading)

// 在挂载前应用已持久化（或系统偏好）的主题与语言。
useThemeStore(pinia).init()
useLocaleStore(pinia).init()

const globalComponents: Component[] = [
  ElAlert,
  ElButton,
  ElCheckbox,
  ElColorPicker,
  ElConfigProvider,
  ElDatePicker,
  ElDialog,
  ElDivider,
  ElDrawer,
  ElDropdown,
  ElDropdownItem,
  ElDropdownMenu,
  ElForm,
  ElFormItem,
  ElIcon,
  ElImage,
  ElInput,
  ElInputNumber,
  ElOption,
  ElPagination,
  ElProgress,
  ElRadioButton,
  ElRadioGroup,
  ElSelect,
  ElSkeleton,
  ElSkeletonItem,
  ElSlider,
  ElStep,
  ElSteps,
  ElSwitch,
  ElTable,
  ElTableColumn,
  ElTabs,
  ElTabPane,
  ElTag,
  ElTooltip,
  ElUpload,
]

for (const component of globalComponents) {
  app.component((component as { name?: string }).name ?? 'UnnamedComponent', component)
}

// store 从不导入 API client 的 store hook；接线在此处完成。
const auth = useAuthStore(pinia)
const appState = useAppStore(pinia)
configureAuth({
  getToken: () => auth.token,
  onUnauthorized: () => {
    if (!auth.isAuthenticated) return
    const wasAdminArea = auth.isAdmin || Boolean(router.currentRoute.value.meta.requiresAdmin)
    auth.clearSession()
    const current = router.currentRoute.value
    if (current.name !== 'login' && current.name !== 'admin-login') {
      // 开启安全入口后，管理员会话过期不能再直接回到普通登录页。
      if (wasAdminArea && appState.adminEntryActive) {
        void router.replace({
          name: 'not-found',
          params: { pathMatch: current.path.replace(/^\/+|\/+$/g, '').split('/').filter(Boolean) },
          replace: true,
        })
        return
      }
      void router.replace({
        path: wasAdminArea ? '/admin/login' : '/login',
        query: { redirect: current.fullPath },
      })
    }
  },
})
auth.hydrate()

app.mount('#app')

import { createApp, type Component } from 'vue'
import { createPinia } from 'pinia'
import {
  ElAlert,
  ElButton,
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
  ElSelect,
  ElSkeleton,
  ElSkeletonItem,
  ElSwitch,
  ElTable,
  ElTableColumn,
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

// Single locked theme: the console is dark-native.
document.documentElement.classList.add('dark')

const app = createApp(App)
const pinia = createPinia()

app.use(pinia)
app.use(router)

const globalComponents: Component[] = [
  ElAlert,
  ElButton,
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
  ElSelect,
  ElSkeleton,
  ElSkeletonItem,
  ElSwitch,
  ElTable,
  ElTableColumn,
  ElTag,
  ElTooltip,
]

for (const component of globalComponents) {
  app.component((component as { name?: string }).name ?? 'UnnamedComponent', component)
}

// The store never imports the API client's store hook; wiring happens here.
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

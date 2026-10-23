import { ElMessage } from 'element-plus'
import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'

import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { translate } from '@/i18n'

declare module 'vue-router' {
  interface RouteMeta {
    title?: string
    /** 无需登录即可访问。 */
    public?: boolean
    /** 需要管理员角色。 */
    requiresAdmin?: boolean
    /** 需要登录（普通用户即可）。 */
    requiresAuth?: boolean
    /** 面包屑中的分组层级（取侧栏分组名）。 */
    group?: string
  }
}

const routes: RouteRecordRaw[] = [
  // ---- 公开：首页 / 分享 / 独立页面 / 登录 / 初始化 ----
  {
    path: '/',
    name: 'home',
    component: () => import('@/views/HomeView.vue'),
    meta: { title: 'routes.home', public: true },
  },
  {
    path: '/s/:token',
    name: 'share',
    component: () => import('@/views/ShareView.vue'),
    meta: { title: 'routes.share', public: true },
  },
  {
    path: '/p/:slug',
    name: 'page',
    component: () => import('@/views/PublicPageView.vue'),
    meta: { title: 'routes.page', public: true },
  },
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/LoginView.vue'),
    meta: { title: 'routes.login', public: true },
  },
  {
    path: '/admin/login',
    name: 'admin-login',
    component: () => import('@/views/LoginView.vue'),
    props: { adminOnly: true },
    meta: { title: 'routes.adminLogin', public: true },
  },
  {
    path: '/install',
    name: 'install',
    component: () => import('@/views/InstallView.vue'),
    meta: { title: 'routes.install', public: true },
  },
  {
    path: '/plaza',
    name: 'public-plaza',
    component: () => import('@/views/PublicPlazaView.vue'),
    meta: { title: 'routes.publicPlaza', public: true },
  },

  // ---- 普通用户控制台 ----
  {
    path: '/user',
    component: () => import('@/layouts/UserLayout.vue'),
    meta: { requiresAuth: true },
    children: [
      {
        path: '',
        name: 'user-dashboard',
        component: () => import('@/views/user/UserDashboardView.vue'),
        meta: { title: 'routes.userDashboard' },
      },
      {
        path: 'images',
        name: 'user-images',
        component: () => import('@/views/ImagesView.vue'),
        meta: { title: 'routes.userImages' },
      },
      {
        path: 'processing',
        name: 'user-processing',
        component: () => import('@/views/ProcessingView.vue'),
        meta: { title: 'routes.userProcessing' },
      },
      {
        path: 'albums',
        name: 'user-albums',
        component: () => import('@/views/AlbumsView.vue'),
        meta: { title: 'routes.userAlbums' },
      },
      {
        path: 'plaza',
        name: 'user-plaza',
        component: () => import('@/views/PlazaView.vue'),
        meta: { title: 'routes.userPlaza' },
      },
      {
        path: 'shares',
        name: 'user-shares',
        component: () => import('@/views/SharesView.vue'),
        meta: { title: 'routes.userShares' },
      },
      {
        path: 'tokens',
        name: 'user-tokens',
        component: () => import('@/views/TokensView.vue'),
        meta: { title: 'routes.userTokens' },
      },
      {
        path: 'pricing',
        name: 'user-pricing',
        component: () => import('@/views/PricingView.vue'),
        meta: { title: 'routes.userPricing' },
      },
      {
        path: 'tickets',
        name: 'user-tickets',
        component: () => import('@/views/TicketsView.vue'),
        meta: { title: 'routes.userTickets' },
      },
      {
        path: 'settings',
        name: 'user-settings',
        component: () => import('@/views/SettingsView.vue'),
        meta: { title: 'routes.userSettings' },
      },
    ],
  },

  // ---- 管理员控制台 ----
  {
    path: '/admin',
    component: () => import('@/layouts/AdminLayout.vue'),
    meta: { requiresAdmin: true },
    children: [
      {
        path: '',
        name: 'admin-dashboard',
        component: () => import('@/views/DashboardView.vue'),
        meta: { title: 'routes.adminDashboard', group: 'nav.groups.overview' },
      },
      {
        path: 'users',
        name: 'admin-users',
        component: () => import('@/views/UsersView.vue'),
        meta: { title: 'routes.adminUsers', group: 'nav.groups.content' },
      },
      {
        path: 'plaza',
        name: 'admin-plaza',
        component: () => import('@/views/PlazaView.vue'),
        meta: { title: 'routes.adminPlaza', group: 'nav.groups.content' },
      },
      {
        path: 'site',
        name: 'admin-site',
        component: () => import('@/views/SiteView.vue'),
        meta: { title: 'routes.adminSite', group: 'nav.groups.content' },
      },
      {
        path: 'policies',
        name: 'admin-policies',
        component: () => import('@/views/PoliciesView.vue'),
        meta: { title: 'routes.adminPolicies', group: 'nav.groups.operations' },
      },
      {
        path: 'billing',
        name: 'admin-billing',
        component: () => import('@/views/BillingView.vue'),
        meta: { title: 'routes.adminBilling', group: 'nav.groups.operations' },
      },
      {
        path: 'storage',
        name: 'admin-storage',
        component: () => import('@/views/StorageView.vue'),
        meta: { title: 'routes.adminStorage', group: 'nav.groups.operations' },
      },
      {
        path: 'system',
        name: 'admin-system',
        component: () => import('@/views/SystemView.vue'),
        meta: { title: 'routes.adminSystem', group: 'nav.groups.system' },
      },
      {
        path: 'plugins',
        name: 'admin-plugins',
        component: () => import('@/views/PluginsView.vue'),
        meta: { title: 'routes.adminPlugins', group: 'nav.groups.system' },
      },
      {
        path: 'mail-logs',
        name: 'admin-mail-logs',
        component: () => import('@/views/NotifyLogsView.vue'),
        meta: { title: 'routes.adminMailLogs', group: 'nav.groups.system' },
      },
      {
        path: 'settings',
        name: 'admin-settings',
        component: () => import('@/views/SettingsView.vue'),
        meta: { title: 'routes.adminSettings', group: 'nav.groups.account' },
      },
    ],
  },

  {
    path: '/:pathMatch(.*)*',
    name: 'not-found',
    component: () => import('@/views/NotFoundView.vue'),
    meta: { title: 'routes.notFound', public: true },
  },
]

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
  scrollBehavior: () => ({ top: 0 }),
})

/** 登录后按角色决定跳转目标。 */
export function homeForRole(isAdmin: boolean): string {
  return isAdmin ? '/admin' : '/user'
}

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  const app = useAppStore()
  auth.hydrate()

  // 管理员安全入口：开启且当前浏览器没有入口 Cookie 时，普通管理员登录页
  // 与未登录的管理后台都按 404 处理；已登录管理员仍可正常使用后台。
  if (to.name === 'admin-login' || to.meta.requiresAdmin) {
    await app.checkAdminEntry()
    if (app.adminEntryActive && (to.name === 'admin-login' || !auth.isAuthenticated)) {
      return {
        name: 'not-found',
        params: { pathMatch: to.path.replace(/^\/+|\/+$/g, '').split('/').filter(Boolean) },
        replace: true,
      }
    }
  }

  // 初始化入口只对未初始化的实例开放。初始化完成后再次访问 /install 应视为
  // 不存在的页面（404），而不是跳转到管理员登录或控制台。
  if (to.name === 'install') {
    const installed = app.installed === null ? await app.checkInstall() : app.isInstalled
    if (installed) {
      return {
        name: 'not-found',
        params: { pathMatch: to.path.replace(/^\/+|\/+$/g, '').split('/').filter(Boolean) },
        replace: true,
      }
    }
  }

  // 不再在路由层检测初始化锁文件：未初始化时也正常访问各页面，
  // 由用户自行访问 /install 完成程序与数据库的初始化。

  if (to.meta.public) {
    if ((to.name === 'login' || to.name === 'admin-login') && auth.isAuthenticated) {
      return { path: homeForRole(auth.isAdmin) }
    }
    return true
  }

  if (!auth.isAuthenticated) {
    // 管理后台的未登录访问导向独立的管理员登录页。
    const loginPath = to.meta.requiresAdmin ? '/admin/login' : '/login'
    return { path: loginPath, query: to.fullPath === '/' ? {} : { redirect: to.fullPath } }
  }

  if (to.meta.requiresAdmin && !auth.isAdmin) {
    ElMessage.warning(translate('errors.insufficientPermission'))
    return { path: '/user' }
  }

  if (to.meta.requiresAuth && to.path.startsWith('/user') && auth.isAdmin) {
    // 管理员访问用户中心时放行（可用于预览普通用户视图）。
    return true
  }

  return true
})

export default router

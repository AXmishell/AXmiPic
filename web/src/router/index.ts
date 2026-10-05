import { ElMessage } from 'element-plus'
import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'

import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'

declare module 'vue-router' {
  interface RouteMeta {
    title?: string
    /** 无需登录即可访问。 */
    public?: boolean
    /** 需要管理员角色。 */
    requiresAdmin?: boolean
    /** 需要登录（普通用户即可）。 */
    requiresAuth?: boolean
  }
}

const routes: RouteRecordRaw[] = [
  // ---- 公开：首页 / 分享 / 独立页面 / 登录 / 安装 ----
  {
    path: '/',
    name: 'home',
    component: () => import('@/views/HomeView.vue'),
    meta: { title: '首页', public: true },
  },
  {
    path: '/s/:token',
    name: 'share',
    component: () => import('@/views/ShareView.vue'),
    meta: { title: '分享', public: true },
  },
  {
    path: '/p/:slug',
    name: 'page',
    component: () => import('@/views/PublicPageView.vue'),
    meta: { title: '页面', public: true },
  },
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/LoginView.vue'),
    meta: { title: '登录', public: true },
  },
  {
    path: '/admin/login',
    name: 'admin-login',
    component: () => import('@/views/LoginView.vue'),
    props: { adminOnly: true },
    meta: { title: '管理员登录', public: true },
  },
  {
    path: '/install',
    name: 'install',
    component: () => import('@/views/InstallView.vue'),
    meta: { title: '安装向导', public: true },
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
        meta: { title: '用户中心' },
      },
      {
        path: 'images',
        name: 'user-images',
        component: () => import('@/views/ImagesView.vue'),
        meta: { title: '我的图片' },
      },
      {
        path: 'processing',
        name: 'user-processing',
        component: () => import('@/views/ProcessingView.vue'),
        meta: { title: '图片处理' },
      },
      {
        path: 'albums',
        name: 'user-albums',
        component: () => import('@/views/AlbumsView.vue'),
        meta: { title: '我的相册' },
      },
      {
        path: 'plaza',
        name: 'user-plaza',
        component: () => import('@/views/PlazaView.vue'),
        meta: { title: '图片广场' },
      },
      {
        path: 'shares',
        name: 'user-shares',
        component: () => import('@/views/SharesView.vue'),
        meta: { title: '我的分享' },
      },
      {
        path: 'tokens',
        name: 'user-tokens',
        component: () => import('@/views/TokensView.vue'),
        meta: { title: '访问令牌' },
      },
      {
        path: 'pricing',
        name: 'user-pricing',
        component: () => import('@/views/PricingView.vue'),
        meta: { title: '套餐' },
      },
      {
        path: 'tickets',
        name: 'user-tickets',
        component: () => import('@/views/TicketsView.vue'),
        meta: { title: '工单' },
      },
      {
        path: 'settings',
        name: 'user-settings',
        component: () => import('@/views/SettingsView.vue'),
        meta: { title: '账号设置' },
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
        meta: { title: '仪表盘' },
      },
      {
        path: 'users',
        name: 'admin-users',
        component: () => import('@/views/UsersView.vue'),
        meta: { title: '用户管理' },
      },
      {
        path: 'plaza',
        name: 'admin-plaza',
        component: () => import('@/views/PlazaView.vue'),
        meta: { title: '图片广场' },
      },
      {
        path: 'site',
        name: 'admin-site',
        component: () => import('@/views/SiteView.vue'),
        meta: { title: '站点内容' },
      },
      {
        path: 'policies',
        name: 'admin-policies',
        component: () => import('@/views/PoliciesView.vue'),
        meta: { title: '角色策略' },
      },
      {
        path: 'billing',
        name: 'admin-billing',
        component: () => import('@/views/BillingView.vue'),
        meta: { title: '计费管理' },
      },
      {
        path: 'storage',
        name: 'admin-storage',
        component: () => import('@/views/StorageView.vue'),
        meta: { title: '存储配置' },
      },
      {
        path: 'system',
        name: 'admin-system',
        component: () => import('@/views/SystemView.vue'),
        meta: { title: '系统设置' },
      },
      {
        path: 'settings',
        name: 'admin-settings',
        component: () => import('@/views/SettingsView.vue'),
        meta: { title: '账号设置' },
      },
    ],
  },

  {
    path: '/:pathMatch(.*)*',
    name: 'not-found',
    component: () => import('@/views/NotFoundView.vue'),
    meta: { title: '页面不存在', public: true },
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
  const app = useAppStore()
  const auth = useAuthStore()
  auth.hydrate()
  document.title = to.meta.title ? `${to.meta.title} · AXmiPic` : 'AXmiPic 图床'

  // 安装门禁：除安装向导外，未安装时一律跳转到 /install。
  const installed = await app.checkInstall()
  if (!installed && to.name !== 'install') {
    return { path: '/install' }
  }
  if (installed && to.name === 'install') {
    return { path: '/' }
  }

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
    ElMessage.warning('权限不足')
    return { path: '/user' }
  }

  if (to.meta.requiresAuth && to.path.startsWith('/user') && auth.isAdmin) {
    // 管理员访问用户中心时放行（可用于预览普通用户视图）。
    return true
  }

  return true
})

export default router

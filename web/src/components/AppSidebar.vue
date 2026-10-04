<script setup lang="ts">
import { computed, type Component } from 'vue'
import { useRoute } from 'vue-router'
import { ArrowRight, Bell, Close, Coin, Folder, Grid, Key, MagicStick, Management, Odometer, Picture, Service, Setting, Share, ShoppingCart, User } from '@element-plus/icons-vue'

import QuotaMeter from '@/components/QuotaMeter.vue'
import { useAuthStore } from '@/stores/auth'

const props = withDefaults(defineProps<{ open: boolean; variant?: 'user' | 'admin' }>(), {
  variant: 'user',
})
const emit = defineEmits<{ (event: 'close'): void }>()

interface NavItem {
  label: string
  to: string
  icon: Component
  exact?: boolean
}

interface NavGroup {
  label: string
  items: NavItem[]
}

const auth = useAuthStore()
const route = useRoute()

/** 普通用户控制台导航。 */
const userGroups: NavGroup[] = [
  {
    label: '概览',
    items: [{ label: '用户中心', to: '/user', icon: Odometer, exact: true }],
  },
  {
    label: '资源',
    items: [
      { label: '我的图片', to: '/user/images', icon: Picture },
      { label: '图片处理', to: '/user/processing', icon: MagicStick },
      { label: '我的相册', to: '/user/albums', icon: Folder },
      { label: '图片广场', to: '/user/plaza', icon: Grid },
      { label: '我的分享', to: '/user/shares', icon: Share },
      { label: '访问令牌', to: '/user/tokens', icon: Key },
    ],
  },
  {
    label: '账户',
    items: [
      { label: '套餐', to: '/user/pricing', icon: ShoppingCart },
      { label: '工单', to: '/user/tickets', icon: Service },
    ],
  },
]

/** 管理员控制台导航。 */
const adminGroups: NavGroup[] = [
  {
    label: '概览',
    items: [{ label: '仪表盘', to: '/admin', icon: Odometer, exact: true }],
  },
  {
    label: '内容',
    items: [
      { label: '用户管理', to: '/admin/users', icon: User },
      { label: '图片广场', to: '/admin/plaza', icon: Grid },
      { label: '站点内容', to: '/admin/site', icon: Bell },
    ],
  },
  {
    label: '运营',
    items: [
      { label: '角色策略', to: '/admin/policies', icon: Management },
      { label: '计费管理', to: '/admin/billing', icon: ShoppingCart },
      { label: '存储配置', to: '/admin/storage', icon: Coin },
    ],
  },
  {
    label: '系统',
    items: [{ label: '系统设置', to: '/admin/system', icon: Setting }],
  },
]

const groups = computed<NavGroup[]>(() => (props.variant === 'admin' ? adminGroups : userGroups))

function isActive(item: NavItem): boolean {
  return item.exact ? route.path === item.to : route.path.startsWith(item.to)
}
</script>

<template>
  <aside class="sidebar" :class="{ 'is-open': open }" aria-label="主导航">
    <div class="sidebar__brand">
      <router-link :to="variant === 'admin' ? '/admin' : '/user'" class="brand" aria-label="AXmiPic 控制台首页">
        <span class="brand__mark" aria-hidden="true">
          <svg viewBox="0 0 32 32" width="18" height="18">
            <defs>
              <linearGradient id="sidebar-mark" x1="0" y1="0" x2="1" y2="1">
                <stop offset="0" stop-color="#828fff" />
                <stop offset="1" stop-color="#5e6ad2" />
              </linearGradient>
            </defs>
            <path d="M16 6.5 23.8 25.5h-4.05l-1.55-4.05h-4.4L12.25 25.5H8.2Z" fill="url(#sidebar-mark)" />
          </svg>
        </span>
        <span class="brand__text">
          <span class="brand__name">AXmiPic</span>
          <span class="brand__sub">{{ variant === 'admin' ? '管理控制台' : '用户中心' }}</span>
        </span>
      </router-link>
      <button class="sidebar__close" type="button" aria-label="关闭导航菜单" @click="emit('close')">
        <el-icon :size="14"><Close /></el-icon>
      </button>
    </div>

    <nav class="sidebar__nav">
      <div v-for="group in groups" :key="group.label" class="sidebar__group">
        <p class="sidebar__section">{{ group.label }}</p>
        <ul class="sidebar__list">
          <li v-for="item in group.items" :key="item.to">
            <router-link
              :to="item.to"
              class="nav-item"
              :class="{ 'is-active': isActive(item) }"
              :aria-current="isActive(item) ? 'page' : undefined"
              @click="emit('close')"
            >
              <span class="nav-item__rail" aria-hidden="true" />
              <el-icon :size="16" class="nav-item__icon"><component :is="item.icon" /></el-icon>
              <span class="nav-item__label">{{ item.label }}</span>
            </router-link>
          </li>
        </ul>
      </div>
    </nav>

    <div class="sidebar__footer">
      <QuotaMeter
        v-if="auth.user && auth.user.quota_bytes > 0"
        compact
        :used="auth.user.used_bytes"
        :quota="auth.user.quota_bytes"
      />
      <router-link
        :to="variant === 'admin' ? '/admin/settings' : '/user/settings'"
        class="nav-item nav-item--footer"
        :class="{ 'is-active': route.path.endsWith('/settings') }"
        :aria-current="route.path.endsWith('/settings') ? 'page' : undefined"
        @click="emit('close')"
      >
        <span class="nav-item__rail" aria-hidden="true" />
        <el-icon :size="16" class="nav-item__icon"><Setting /></el-icon>
        <span class="nav-item__label">账号设置</span>
        <el-icon :size="12" class="nav-item__arrow"><ArrowRight /></el-icon>
      </router-link>
    </div>
  </aside>
</template>

<style scoped>
.sidebar {
  display: grid;
  grid-template-rows: auto minmax(0, 1fr) auto;
  height: 100dvh;
  background: var(--ax-panel);
  border-right: 1px solid var(--ax-border-subtle);
}

.sidebar__brand {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: var(--ax-topbar-h);
  padding-inline: var(--ax-space-4) var(--ax-space-3);
  border-bottom: 1px solid var(--ax-border-subtle);
}

.brand {
  display: flex;
  align-items: center;
  gap: var(--ax-space-3);
  min-width: 0;
  color: inherit;
}

.brand__mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  background: var(--ax-accent-soft);
  border: 1px solid var(--ax-accent-ring);
  border-radius: var(--ax-radius-md);
  box-shadow: var(--ax-ring-inset);
}

.brand__text {
  display: flex;
  flex-direction: column;
  line-height: 1.15;
}

.brand__name {
  color: var(--ax-text);
  font-size: var(--ax-text-base);
  font-weight: var(--ax-weight-semibold);
  letter-spacing: var(--ax-tracking-display);
}

.brand__sub {
  color: var(--ax-text-4);
  font-size: 11px;
  letter-spacing: 0.02em;
}

.sidebar__close {
  display: none;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  padding: 0;
  color: var(--ax-text-3);
  background: transparent;
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-sm);
  cursor: pointer;
}

.sidebar__nav {
  min-block-size: 0;
  padding: var(--ax-space-4) var(--ax-space-3);
  overflow-y: auto;
}

.sidebar__group + .sidebar__group {
  margin-top: var(--ax-space-5);
}

.sidebar__section {
  margin: 0 0 var(--ax-space-2);
  padding-inline: var(--ax-space-2);
  color: var(--ax-text-4);
  font-size: 11px;
  font-weight: var(--ax-weight-medium);
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.sidebar__list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.nav-item {
  position: relative;
  display: flex;
  align-items: center;
  gap: var(--ax-space-3);
  padding: 7px var(--ax-space-2);
  color: var(--ax-text-3);
  border-radius: var(--ax-radius-sm);
  transition:
    color var(--ax-duration-fast) var(--ax-ease),
    background-color var(--ax-duration-fast) var(--ax-ease);
}

.nav-item:hover {
  color: var(--ax-text);
  background: var(--ax-tint);
}

.nav-item.is-active {
  color: var(--ax-text);
  background: var(--ax-accent-soft);
}

.nav-item__rail {
  position: absolute;
  inset-block: 6px;
  inset-inline-start: -1px;
  width: 2px;
  background: transparent;
  border-radius: var(--ax-radius-full);
}

.nav-item.is-active .nav-item__rail {
  background: var(--ax-accent-bright);
}

.nav-item__icon {
  flex: none;
}

.nav-item__label {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  font-size: var(--ax-text-sm);
  font-weight: var(--ax-weight-medium);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.nav-item__arrow {
  color: var(--ax-text-4);
}

.sidebar__footer {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-3);
  padding: var(--ax-space-4) var(--ax-space-3);
  border-top: 1px solid var(--ax-border-subtle);
}

.nav-item--footer {
  padding-block: 8px;
}

@media (max-width: 960px) {
  .sidebar {
    position: fixed;
    inset-block: 0;
    inset-inline-start: 0;
    z-index: var(--ax-z-drawer);
    width: min(280px, 84vw);
    transform: translateX(-100%);
    transition: transform var(--ax-duration) var(--ax-ease);
    box-shadow: var(--ax-shadow-lg);
  }

  .sidebar.is-open {
    transform: none;
  }

  .sidebar__close {
    display: inline-flex;
  }
}
</style>

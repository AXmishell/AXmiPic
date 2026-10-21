<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowDown, Expand, Fold, Management, Moon, Search, Setting, Sunny, SwitchButton, User } from '@element-plus/icons-vue'

import UserAvatar from '@/components/UserAvatar.vue'
import { useAuthStore } from '@/stores/auth'
import { useThemeStore } from '@/stores/theme'

withDefaults(defineProps<{ sidebarCollapsed?: boolean }>(), { sidebarCollapsed: false })

const emit = defineEmits<{
  (event: 'toggle-menu'): void
  (event: 'toggle-sidebar'): void
  (event: 'open-search'): void
}>()

const auth = useAuthStore()
const theme = useThemeStore()
const route = useRoute()
const router = useRouter()

const pageTitle = computed(() => route.meta.title ?? '')
const inAdminArea = computed(() => route.path.startsWith('/admin'))
const breadcrumbRoot = computed(() => (inAdminArea.value ? '/admin' : '/user'))
const breadcrumbGroup = computed(() => route.meta.group ?? '')
const shortcutLabel = navigator.userAgent.includes('Mac') ? '⌘K' : 'Ctrl K'

async function handleCommand(command: string | number | object): Promise<void> {
  if (command === 'settings') {
    await router.push(inAdminArea.value ? '/admin/settings' : '/user/settings')
    return
  }
  if (command === 'home') {
    await router.push('/')
    return
  }
  if (command === 'user-center') {
    await router.push('/user')
    return
  }
  if (command === 'admin-console') {
    await router.push('/admin')
    return
  }
  if (command === 'logout') {
    try {
      await ElMessageBox.confirm('退出后需要重新登录才能继续管理图片。', '确定退出登录吗？', {
        confirmButtonText: '退出登录',
        cancelButtonText: '取消',
        type: 'warning',
      })
    } catch {
      return
    }
    auth.logout()
    ElMessage.success('已退出登录')
    await router.replace('/login')
  }
}
</script>

<template>
  <header class="topbar">
    <button
      class="topbar__menu"
      type="button"
      aria-label="打开导航菜单"
      @click="emit('toggle-menu')"
    >
      <svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true">
        <path
          d="M2 4.25h12M2 8h12M2 11.75h12"
          fill="none"
          stroke="currentColor"
          stroke-width="1.5"
          stroke-linecap="round"
        />
      </svg>
    </button>

    <button
      class="topbar__collapse"
      type="button"
      :aria-label="sidebarCollapsed ? '展开侧栏' : '折叠侧栏'"
      :title="sidebarCollapsed ? '展开侧栏' : '折叠侧栏'"
      @click="emit('toggle-sidebar')"
    >
      <el-icon :size="16">
        <component :is="sidebarCollapsed ? Expand : Fold" />
      </el-icon>
    </button>

    <nav class="topbar__crumbs" aria-label="面包屑">
      <router-link class="topbar__crumb-root" :to="breadcrumbRoot">AXmiPic</router-link>
      <span class="topbar__crumb-sep" aria-hidden="true">/</span>
      <template v-if="breadcrumbGroup">
        <span class="topbar__crumb-group">{{ breadcrumbGroup }}</span>
        <span class="topbar__crumb-sep" aria-hidden="true">/</span>
      </template>
      <span class="topbar__crumb-current">{{ pageTitle }}</span>
    </nav>

    <div class="topbar__spacer" />

    <button
      class="topbar__search"
      type="button"
      aria-label="全局搜索"
      :title="`全局搜索（${shortcutLabel}）`"
      @click="emit('open-search')"
    >
      <el-icon :size="15"><Search /></el-icon>
      <span class="topbar__search-label">搜索</span>
      <kbd class="topbar__search-kbd">{{ shortcutLabel }}</kbd>
    </button>

    <button
      class="topbar__theme"
      type="button"
      :aria-label="theme.isDark ? '切换到浅色主题' : '切换到深色主题'"
      :title="theme.isDark ? '切换到浅色主题' : '切换到深色主题'"
      @click="theme.toggle()"
    >
      <el-icon :size="16">
        <component :is="theme.isDark ? Sunny : Moon" />
      </el-icon>
    </button>

    <el-dropdown trigger="click" placement="bottom-end" @command="handleCommand">
      <button class="topbar__user" type="button" aria-label="账号菜单">
        <UserAvatar :name="auth.username" size="sm" />
        <span class="topbar__user-meta">
          <span class="topbar__username">{{ auth.username }}</span>
          <span class="topbar__role">{{ auth.isAdmin ? '管理员' : '用户' }}</span>
        </span>
        <el-icon :size="12" class="topbar__caret"><ArrowDown /></el-icon>
      </button>
      <template #dropdown>
        <el-dropdown-menu>
          <el-dropdown-item command="home">
            <el-icon><Setting /></el-icon>返回首页
          </el-dropdown-item>
          <el-dropdown-item v-if="auth.isAdmin && inAdminArea" command="user-center">
            <el-icon><User /></el-icon>用户中心
          </el-dropdown-item>
          <el-dropdown-item v-if="auth.isAdmin && !inAdminArea" command="admin-console">
            <el-icon><Management /></el-icon>管理控制台
          </el-dropdown-item>
          <el-dropdown-item command="settings">
            <el-icon><Setting /></el-icon>账号设置
          </el-dropdown-item>
          <el-dropdown-item command="logout" divided>
            <el-icon><SwitchButton /></el-icon>退出登录
          </el-dropdown-item>
        </el-dropdown-menu>
      </template>
    </el-dropdown>
  </header>
</template>

<style scoped>
.topbar {
  display: flex;
  align-items: center;
  gap: var(--ax-space-3);
  min-width: 0;
  height: var(--ax-topbar-h);
  padding-inline: var(--ax-space-5);
  background: var(--ax-topbar-bg);
  border-bottom: 1px solid var(--ax-border-subtle);
  backdrop-filter: blur(12px);
}

.topbar__menu {
  display: none;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  padding: 0;
  color: var(--ax-text-2);
  background: var(--ax-tint);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-sm);
  cursor: pointer;
  transition:
    color var(--ax-duration-fast) var(--ax-ease),
    background-color var(--ax-duration-fast) var(--ax-ease);
}

.topbar__menu:hover {
  color: var(--ax-text);
  background: var(--ax-tint-strong);
}

.topbar__search {
  display: inline-flex;
  flex: none;
  align-items: center;
  gap: var(--ax-space-2);
  height: 32px;
  padding: 0 var(--ax-space-3);
  color: var(--ax-text-3);
  background: var(--ax-tint-weak);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-full);
  cursor: pointer;
  transition:
    color var(--ax-duration-fast) var(--ax-ease),
    background-color var(--ax-duration-fast) var(--ax-ease),
    border-color var(--ax-duration-fast) var(--ax-ease);
}

.topbar__search:hover {
  color: var(--ax-text);
  background: var(--ax-tint);
  border-color: var(--ax-border);
}

.topbar__search-label {
  font-size: var(--ax-text-sm);
  font-weight: var(--ax-weight-medium);
}

.topbar__search-kbd {
  padding: 1px 5px;
  color: var(--ax-text-4);
  background: var(--ax-tint);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-xs);
  font-family: var(--ax-font-mono);
  font-size: 10px;
  line-height: 1.4;
}

.topbar__theme {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  padding: 0;
  color: var(--ax-text-3);
  background: var(--ax-tint);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-sm);
  cursor: pointer;
  transition:
    color var(--ax-duration-fast) var(--ax-ease),
    background-color var(--ax-duration-fast) var(--ax-ease);
}

.topbar__collapse {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  padding: 0;
  color: var(--ax-text-3);
  background: transparent;
  border: 1px solid transparent;
  border-radius: var(--ax-radius-sm);
  cursor: pointer;
  transition:
    color var(--ax-duration-fast) var(--ax-ease),
    background-color var(--ax-duration-fast) var(--ax-ease);
}

.topbar__collapse:hover {
  color: var(--ax-text);
  background: var(--ax-tint);
}

.topbar__theme:hover {
  color: var(--ax-text);
  background: var(--ax-tint-strong);
}

.topbar__crumbs {
  display: flex;
  align-items: baseline;
  gap: var(--ax-space-2);
  min-width: 0;
  font-size: var(--ax-text-sm);
}

.topbar__crumb-root {
  color: var(--ax-text-4);
  font-weight: var(--ax-weight-medium);
  transition: color var(--ax-duration-fast) var(--ax-ease);
}

.topbar__crumb-root:hover {
  color: var(--ax-text-2);
}

.topbar__crumb-group {
  color: var(--ax-text-4);
  font-weight: var(--ax-weight-medium);
}

.topbar__crumb-sep {
  color: var(--ax-text-4);
}

.topbar__crumb-current {
  overflow: hidden;
  color: var(--ax-text-2);
  font-weight: var(--ax-weight-medium);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.topbar__spacer {
  flex: 1;
}

.topbar__user {
  display: flex;
  align-items: center;
  gap: var(--ax-space-2);
  max-width: 240px;
  padding: 4px 10px 4px 6px;
  color: var(--ax-text-2);
  background: var(--ax-tint-weak);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-full);
  cursor: pointer;
  transition:
    background-color var(--ax-duration-fast) var(--ax-ease),
    border-color var(--ax-duration-fast) var(--ax-ease);
}

.topbar__user:hover {
  background: var(--ax-tint-strong);
  border-color: var(--ax-border);
}

.topbar__user-meta {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  min-width: 0;
  line-height: 1.25;
}

.topbar__username {
  max-width: 130px;
  overflow: hidden;
  color: var(--ax-text);
  font-size: var(--ax-text-sm);
  font-weight: var(--ax-weight-medium);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.topbar__role {
  color: var(--ax-text-4);
  font-size: var(--ax-text-2xs);
}

.topbar__caret {
  color: var(--ax-text-4);
}

@media (max-width: 960px) {
  .topbar {
    padding-inline: var(--ax-space-4);
  }

  .topbar__menu {
    display: inline-flex;
  }

  .topbar__collapse {
    display: none;
  }

  .topbar__user-meta {
    display: none;
  }

  .topbar__user {
    padding: 4px;
  }

  .topbar__search {
    justify-content: center;
    width: 32px;
    padding: 0;
  }

  .topbar__search-label,
  .topbar__search-kbd {
    display: none;
  }

  .topbar__caret {
    display: none;
  }
}
</style>

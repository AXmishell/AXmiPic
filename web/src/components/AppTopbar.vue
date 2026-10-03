<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowDown, Setting, SwitchButton } from '@element-plus/icons-vue'

import UserAvatar from '@/components/UserAvatar.vue'
import { useAuthStore } from '@/stores/auth'

const emit = defineEmits<{ (event: 'toggle-menu'): void }>()

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()

const pageTitle = computed(() => route.meta.title ?? '')

async function handleCommand(command: string | number | object): Promise<void> {
  if (command === 'settings') {
    await router.push('/settings')
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

    <nav class="topbar__crumbs" aria-label="面包屑">
      <span class="topbar__crumb-root">AXmiPic</span>
      <span class="topbar__crumb-sep" aria-hidden="true">/</span>
      <span class="topbar__crumb-current">{{ pageTitle }}</span>
    </nav>

    <div class="topbar__spacer" />

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
  background: rgba(15, 16, 17, 0.72);
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
  background: rgba(255, 255, 255, 0.03);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-sm);
  cursor: pointer;
  transition:
    color var(--ax-duration-fast) var(--ax-ease),
    background-color var(--ax-duration-fast) var(--ax-ease);
}

.topbar__menu:hover {
  color: var(--ax-text);
  background: rgba(255, 255, 255, 0.06);
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
  background: rgba(255, 255, 255, 0.02);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-full);
  cursor: pointer;
  transition:
    background-color var(--ax-duration-fast) var(--ax-ease),
    border-color var(--ax-duration-fast) var(--ax-ease);
}

.topbar__user:hover {
  background: rgba(255, 255, 255, 0.05);
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
  font-size: 11px;
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

  .topbar__user-meta {
    display: none;
  }

  .topbar__user {
    padding: 4px;
  }

  .topbar__caret {
    display: none;
  }
}
</style>

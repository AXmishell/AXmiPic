<script setup lang="ts">
import { useRouter } from 'vue-router'
import { Moon, Sunny, User } from '@element-plus/icons-vue'

import PlazaMasonry from '@/components/PlazaMasonry.vue'
import { useAuthStore } from '@/stores/auth'
import { useThemeStore } from '@/stores/theme'

const router = useRouter()
const auth = useAuthStore()
const theme = useThemeStore()

function goAuth(): void {
  if (auth.isAuthenticated) {
    void router.push(auth.isAdmin ? '/admin' : '/user')
  } else {
    void router.push('/login')
  }
}
</script>

<template>
  <div class="pp">
    <header class="pp__topbar">
      <router-link to="/" class="pp__brand" aria-label="AXmiPic 首页">
        <span class="pp__mark" aria-hidden="true">
          <svg viewBox="0 0 32 32" width="18" height="18">
            <defs>
              <linearGradient id="pp-mark" x1="0" y1="0" x2="1" y2="1">
                <stop offset="0" stop-color="#828fff" />
                <stop offset="1" stop-color="#5e6ad2" />
              </linearGradient>
            </defs>
            <path d="M16 6.5 23.8 25.5h-4.05l-1.55-4.05h-4.4L12.25 25.5H8.2Z" fill="url(#pp-mark)" />
          </svg>
        </span>
        <span class="pp__brand-name">AXmiPic</span>
      </router-link>

      <div class="pp__actions">
        <button
          class="pp__icon-btn"
          type="button"
          :aria-label="theme.isDark ? '切换到浅色主题' : '切换到深色主题'"
          @click="theme.toggle()"
        >
          <el-icon :size="16"><component :is="theme.isDark ? Sunny : Moon" /></el-icon>
        </button>
        <el-button v-if="auth.isAuthenticated" :icon="User" @click="goAuth">
          {{ auth.isAdmin ? '管理控制台' : '用户中心' }}
        </el-button>
        <template v-else>
          <el-button @click="router.push('/login')">登录</el-button>
          <el-button type="primary" @click="router.push('/login')">注册 / 登录</el-button>
        </template>
      </div>
    </header>

    <main class="pp__main">
      <div class="pp__head">
        <h1 class="pp__title">图片广场</h1>
        <p class="pp__desc">所有用户公开分享的图片，瀑布流浏览。</p>
      </div>
      <PlazaMasonry />
    </main>
  </div>
</template>

<style scoped>
.pp {
  min-height: 100dvh;
  background: var(--ax-bg);
}

.pp__topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  max-width: 1280px;
  margin: 0 auto;
  padding: var(--ax-space-4) var(--ax-space-5);
}

.pp__brand {
  display: flex;
  align-items: center;
  gap: var(--ax-space-3);
  color: inherit;
}

.pp__mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  background: var(--ax-accent-soft);
  border: 1px solid var(--ax-accent-ring);
  border-radius: var(--ax-radius-md);
}

.pp__brand-name {
  color: var(--ax-text);
  font-size: var(--ax-text-lg);
  font-weight: var(--ax-weight-semibold);
  letter-spacing: var(--ax-tracking-display);
}

.pp__actions {
  display: flex;
  align-items: center;
  gap: var(--ax-space-2);
}

.pp__icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  padding: 0;
  color: var(--ax-text-3);
  background: var(--ax-tint);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-sm);
  cursor: pointer;
}

.pp__icon-btn:hover {
  color: var(--ax-text);
  background: var(--ax-tint-strong);
}

.pp__main {
  max-width: 1280px;
  margin: 0 auto;
  padding: var(--ax-space-5) var(--ax-space-5) var(--ax-space-12);
}

.pp__head {
  margin-bottom: var(--ax-space-5);
}

.pp__title {
  margin: 0;
  color: var(--ax-text);
  font-size: clamp(1.4rem, 4vw, 2rem);
  font-weight: var(--ax-weight-semibold);
  letter-spacing: var(--ax-tracking-display);
}

.pp__desc {
  margin: var(--ax-space-2) 0 0;
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
}
</style>

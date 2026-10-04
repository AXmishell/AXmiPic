<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { Box, DataLine, Lock, UploadFilled, User } from '@element-plus/icons-vue'

import { ApiError } from '@/api/client'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

const mode = ref<'login' | 'register' | 'admin'>('login')
const formRef = ref<FormInstance>()
const loading = ref(false)
const form = reactive({ username: '', password: '' })

const rules: FormRules = {
  username: [
    { required: true, message: '请输入用户名', trigger: 'blur' },
    { min: 3, max: 64, message: '用户名长度为 3 到 64 个字符', trigger: 'blur' },
  ],
  password: [
    { required: true, message: '请输入密码', trigger: 'blur' },
    { min: 8, max: 72, message: '密码长度为 8 到 72 个字符', trigger: 'blur' },
  ],
}

const heading = computed(() => {
  if (mode.value === 'admin') return '管理员登录'
  return mode.value === 'login' ? '欢迎回来' : '创建新账号'
})
const subheading = computed(() => {
  if (mode.value === 'admin') return '使用管理员账号进入管理控制台'
  return mode.value === 'login' ? '登录以管理图片、访问令牌与存储配额' : '注册成功后将自动登录并进入控制台'
})
const submitLabel = computed(() => {
  if (mode.value === 'admin') return '管理员登录'
  return mode.value === 'login' ? '登录' : '注册并登录'
})

function switchMode(): void {
  mode.value = mode.value === 'register' ? 'login' : 'register'
  formRef.value?.clearValidate()
}

function toggleAdmin(): void {
  mode.value = mode.value === 'admin' ? 'login' : 'admin'
  formRef.value?.clearValidate()
}

async function handleSubmit(): Promise<void> {
  const instance = formRef.value
  if (!instance || loading.value) return
  const valid = await instance.validate().catch(() => false)
  if (!valid) return

  loading.value = true
  try {
    const credentials = { username: form.username.trim(), password: form.password }
    if (mode.value === 'admin') {
      await auth.adminLogin(credentials)
      ElMessage.success('登录成功')
    } else if (mode.value === 'login') {
      await auth.login(credentials)
      ElMessage.success('登录成功')
    } else {
      await auth.register(credentials)
      ElMessage.success('注册成功，已自动登录')
    }
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/'
    await router.replace(redirect.startsWith('/') ? redirect : '/')
  } catch (error) {
    ElMessage.error(error instanceof ApiError ? error.message : '操作失败，请稍后重试')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="auth">
    <section class="auth__brand">
      <div class="auth__aurora" aria-hidden="true" />
      <div class="auth__grid" aria-hidden="true" />
      <div class="auth__brand-inner">
        <div class="auth__logo">
          <span class="auth__logo-mark" aria-hidden="true">
            <svg viewBox="0 0 32 32" width="20" height="20">
              <defs>
                <linearGradient id="auth-mark" x1="0" y1="0" x2="1" y2="1">
                  <stop offset="0" stop-color="#828fff" />
                  <stop offset="1" stop-color="#5e6ad2" />
                </linearGradient>
              </defs>
              <path d="M16 6.5 23.8 25.5h-4.05l-1.55-4.05h-4.4L12.25 25.5H8.2Z" fill="url(#auth-mark)" />
            </svg>
          </span>
          <span class="auth__logo-name">AXmiPic</span>
        </div>

        <h2 class="auth__headline">轻量、可靠的自托管图床</h2>
        <p class="auth__subline">上传、加工、分发图片，一切尽在掌握。</p>

        <ul class="auth__features">
          <li class="auth__feature">
            <span class="auth__feature-icon"><el-icon :size="15"><UploadFilled /></el-icon></span>
            <div>
              <strong>极速上传</strong>
              <span>一次请求完成存储、记录与处理</span>
            </div>
          </li>
          <li class="auth__feature">
            <span class="auth__feature-icon"><el-icon :size="15"><Box /></el-icon></span>
            <div>
              <strong>多存储驱动</strong>
              <span>本地、S3 与七牛云自由切换</span>
            </div>
          </li>
          <li class="auth__feature">
            <span class="auth__feature-icon"><el-icon :size="15"><DataLine /></el-icon></span>
            <div>
              <strong>清晰用量</strong>
              <span>配额与访问令牌一目了然</span>
            </div>
          </li>
        </ul>

        <p class="auth__footnote">AXmiPic · 图床控制台</p>
      </div>
    </section>

    <section class="auth__panel">
      <div class="auth__card">
        <div class="auth__mobile-logo">
          <span class="auth__logo-mark" aria-hidden="true">
            <svg viewBox="0 0 32 32" width="18" height="18">
              <path d="M16 6.5 23.8 25.5h-4.05l-1.55-4.05h-4.4L12.25 25.5H8.2Z" fill="currentColor" />
            </svg>
          </span>
          <span>AXmiPic</span>
        </div>

        <h1 class="auth__title">{{ heading }}</h1>
        <p class="auth__desc">{{ subheading }}</p>

        <el-form
          ref="formRef"
          :model="form"
          :rules="rules"
          label-position="top"
          size="large"
          @submit.prevent="handleSubmit"
        >
          <el-form-item label="用户名" prop="username">
            <el-input
              v-model="form.username"
              :prefix-icon="User"
              placeholder="请输入用户名"
              autocomplete="username"
              spellcheck="false"
            />
          </el-form-item>
          <el-form-item label="密码" prop="password">
            <el-input
              v-model="form.password"
              type="password"
              show-password
              :prefix-icon="Lock"
              placeholder="请输入密码"
              :autocomplete="mode === 'login' ? 'current-password' : 'new-password'"
              @keyup.enter="handleSubmit"
            />
          </el-form-item>

          <el-button
            type="primary"
            size="large"
            class="auth__submit"
            :loading="loading"
            @click="handleSubmit"
          >
            {{ submitLabel }}
          </el-button>
        </el-form>

        <p class="auth__switch">
          <template v-if="mode === 'register'">
            已有账号？
            <button type="button" class="auth__switch-btn" @click="switchMode">去登录</button>
          </template>
          <template v-else-if="mode === 'admin'">
            <button type="button" class="auth__switch-btn" @click="toggleAdmin">返回普通登录</button>
          </template>
          <template v-else>
            还没有账号？
            <button type="button" class="auth__switch-btn" @click="switchMode">立即注册</button>
            <span class="auth__switch-sep">·</span>
            <button type="button" class="auth__switch-btn" @click="toggleAdmin">管理员登录</button>
          </template>
        </p>
      </div>
    </section>
  </div>
</template>

<style scoped>
.auth {
  display: grid;
  grid-template-columns: minmax(0, 1.05fr) minmax(0, 1fr);
  min-height: 100dvh;
}

/* ---- Brand panel ---- */
.auth__brand {
  position: relative;
  display: flex;
  align-items: center;
  overflow: hidden;
  background: #0b0c0f;
  border-right: 1px solid var(--ax-border-subtle);
}

.auth__aurora {
  position: absolute;
  inset: 0;
  background:
    radial-gradient(620px 420px at 18% 12%, rgba(94, 106, 210, 0.28), transparent 62%),
    radial-gradient(520px 380px at 88% 82%, rgba(64, 150, 255, 0.12), transparent 66%),
    radial-gradient(360px 300px at 70% 18%, rgba(130, 143, 255, 0.1), transparent 60%);
  pointer-events: none;
}

.auth__grid {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(rgba(255, 255, 255, 0.03) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255, 255, 255, 0.03) 1px, transparent 1px);
  background-size: 44px 44px;
  mask-image: radial-gradient(ellipse 90% 70% at 40% 40%, #000 30%, transparent 78%);
  pointer-events: none;
}

.auth__brand-inner {
  position: relative;
  display: flex;
  flex-direction: column;
  width: min(480px, 100%);
  margin-inline: auto;
  padding: var(--ax-space-12) var(--ax-space-10);
}

.auth__logo {
  display: flex;
  align-items: center;
  gap: var(--ax-space-3);
  margin-bottom: var(--ax-space-10);
}

.auth__logo-mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  color: #b9bdff;
  background: var(--ax-accent-soft);
  border: 1px solid rgba(113, 112, 255, 0.32);
  border-radius: var(--ax-radius-md);
  box-shadow: var(--ax-ring-inset);
}

.auth__logo-name {
  color: var(--ax-text);
  font-size: var(--ax-text-lg);
  font-weight: var(--ax-weight-semibold);
  letter-spacing: var(--ax-tracking-display);
}

.auth__headline {
  max-width: 14ch;
  color: var(--ax-text);
  font-size: var(--ax-text-3xl);
  font-weight: var(--ax-weight-semibold);
  line-height: 1.2;
  letter-spacing: var(--ax-tracking-display);
}

.auth__subline {
  margin-top: var(--ax-space-3);
  max-width: 34ch;
  color: var(--ax-text-3);
  font-size: var(--ax-text-md);
}

.auth__features {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-5);
  margin: var(--ax-space-10) 0 0;
  padding: 0;
  list-style: none;
}

.auth__feature {
  display: flex;
  align-items: flex-start;
  gap: var(--ax-space-3);
}

.auth__feature-icon {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  color: #b9bdff;
  background: rgba(255, 255, 255, 0.04);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-sm);
}

.auth__feature strong {
  display: block;
  color: var(--ax-text-2);
  font-size: var(--ax-text-sm);
  font-weight: var(--ax-weight-medium);
}

.auth__feature span {
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.auth__footnote {
  margin-top: auto;
  padding-top: var(--ax-space-12);
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

/* ---- Form panel ---- */
.auth__panel {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--ax-space-8) var(--ax-space-6);
}

.auth__card {
  width: min(400px, 100%);
}

.auth__mobile-logo {
  display: none;
  align-items: center;
  gap: var(--ax-space-3);
  margin-bottom: var(--ax-space-8);
  color: var(--ax-text);
  font-weight: var(--ax-weight-semibold);
}

.auth__title {
  font-size: var(--ax-text-2xl);
  letter-spacing: var(--ax-tracking-display);
}

.auth__desc {
  margin-top: var(--ax-space-2);
  margin-bottom: var(--ax-space-8);
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
}

.auth__submit {
  width: 100%;
  margin-top: var(--ax-space-2);
}

.auth__switch {
  margin-top: var(--ax-space-6);
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
  text-align: center;
}

.auth__switch-btn {
  padding: 0;
  color: var(--ax-accent-hover);
  background: none;
  border: none;
  font-size: inherit;
  font-weight: var(--ax-weight-medium);
  cursor: pointer;
  transition: color var(--ax-duration-fast) var(--ax-ease);
}

.auth__switch-btn:hover {
  color: #a5b0ff;
}

.auth__switch-btn:focus-visible {
  outline: 2px solid var(--ax-focus);
  outline-offset: 2px;
  border-radius: 2px;
}

.auth__switch-sep {
  margin: 0 var(--ax-space-2);
  color: var(--ax-text-4);
}

@media (max-width: 960px) {
  .auth {
    grid-template-columns: minmax(0, 1fr);
  }

  .auth__brand {
    display: none;
  }

  .auth__mobile-logo {
    display: flex;
  }
}
</style>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { Box, DataLine, Lock, Message, Moon, Sunny, UploadFilled, User } from '@element-plus/icons-vue'

import { ApiError } from '@/api/client'
import { resetPassword, sendPasswordResetCode, sendRegisterCode } from '@/api/auth'
import { useAuthStore } from '@/stores/auth'
import { useThemeStore } from '@/stores/theme'
import { confirmPasswordRule, usernameRules } from '@/utils/validate'

const auth = useAuthStore()
const theme = useThemeStore()
const router = useRouter()
const route = useRoute()

// 独立的管理员登录页（/admin/login）只需管理员登录，不显示注册与身份切换。
const props = defineProps<{ adminOnly?: boolean }>()

const mode = ref<'login' | 'register' | 'admin'>(props.adminOnly ? 'admin' : 'login')
const formRef = ref<FormInstance>()
const loading = ref(false)
const form = reactive({ username: '', password: '', confirmPassword: '', email: '', code: '' })
const codeSending = ref(false)

// TOTP 二次验证步骤。
const totpChallenge = ref('')
const totpCode = ref('')
const totpLoading = ref(false)

// 找回密码步骤。
const resetDialog = ref(false)
const resetSending = ref(false)
const resetSubmitting = ref(false)
const resetForm = reactive({ email: '', code: '', password: '', confirm: '' })

// 普通登录页与管理员登录页共用本组件。切换路由时组件实例会被 Vue Router 复用，
// 需监听 adminOnly 变化同步登录模式，否则地址变化但页面停留在旧模式。
watch(
  () => props.adminOnly,
  (adminOnly) => {
    mode.value = adminOnly ? 'admin' : 'login'
    form.username = ''
    form.password = ''
    form.confirmPassword = ''
    form.email = ''
    form.code = ''
    totpChallenge.value = ''
    totpCode.value = ''
    resetDialog.value = false
    formRef.value?.clearValidate()
  },
)

const rules = computed<FormRules>(() => ({
  username:
    mode.value === 'register'
      ? usernameRules('请输入用户名')
      : [
          { required: true, message: '请输入用户名或邮箱', trigger: 'blur' },
          { min: 3, max: 254, message: '请输入有效的用户名或邮箱', trigger: 'blur' },
        ],
  email: [
    { required: true, message: '请输入邮箱', trigger: 'blur' },
    { type: 'email', message: '请输入有效的邮箱地址', trigger: ['blur', 'change'] },
  ],
  code: [
    { required: true, message: '请输入验证码', trigger: 'blur' },
    { pattern: /^\d{6}$/, message: '验证码为 6 位数字', trigger: 'blur' },
  ],
  password: [
    { required: true, message: '请输入密码', trigger: 'blur' },
    { min: 8, max: 72, message: '密码长度为 8 到 72 个字符', trigger: 'blur' },
  ],
  confirmPassword: [confirmPasswordRule(() => form.password)],
}))

const heading = computed(() => {
  if (totpChallenge.value) return '两步验证'
  if (mode.value === 'admin') return '管理员登录'
  return mode.value === 'login' ? '欢迎回来' : '创建新账号'
})
const subheading = computed(() => {
  if (totpChallenge.value) return '请输入身份验证器中的 6 位动态验证码以完成登录'
  if (mode.value === 'admin') return '使用管理员账号进入管理控制台'
  return mode.value === 'login' ? '登录以管理图片、访问令牌与存储配额' : '填写邮箱并完成验证后即可创建账号'
})
const submitLabel = computed(() => {
  if (mode.value === 'admin') return '管理员登录'
  return mode.value === 'login' ? '登录' : '注册并登录'
})

function switchMode(): void {
  mode.value = mode.value === 'register' ? 'login' : 'register'
  formRef.value?.clearValidate()
}

/** 向邮箱发送注册验证码。 */
async function sendRegCode(): Promise<void> {
  const email = form.email.trim()
  if (!email) {
    ElMessage.warning('请先输入邮箱')
    return
  }
  codeSending.value = true
  try {
    await sendRegisterCode(email)
    ElMessage.success('验证码已发送，请查收邮箱')
  } catch (error) {
    ElMessage.error(error instanceof ApiError ? error.message : '发送失败，请稍后重试')
  } finally {
    codeSending.value = false
  }
}

async function handleSubmit(): Promise<void> {
  const instance = formRef.value
  if (!instance || loading.value) return
  const valid = await instance.validate().catch(() => false)
  if (!valid) return

  loading.value = true
  try {
    const credentials = { username: form.username.trim(), password: form.password }
    let outcome
    if (mode.value === 'admin') {
      outcome = await auth.adminLogin(credentials)
    } else if (mode.value === 'login') {
      outcome = await auth.login(credentials)
    } else {
      outcome = await auth.register({
        username: form.username.trim(),
        email: form.email.trim(),
        code: form.code.trim(),
        password: form.password,
      })
    }
    if (outcome.totpRequired) {
      totpChallenge.value = outcome.challengeToken
      totpCode.value = ''
      ElMessage.info('请输入动态验证码完成登录')
      return
    }
    ElMessage.success(mode.value === 'register' ? '注册成功，已自动登录' : '登录成功')
    await redirectAfterLogin()
  } catch (error) {
    ElMessage.error(error instanceof ApiError ? error.message : '操作失败，请稍后重试')
  } finally {
    loading.value = false
  }
}

/** 完成 TOTP 二次验证并进入控制台。 */
async function submitTOTP(): Promise<void> {
  const code = totpCode.value.trim()
  if (!/^\d{6}$/.test(code)) {
    ElMessage.warning('请输入 6 位动态验证码')
    return
  }
  totpLoading.value = true
  try {
    await auth.verifyTOTP(totpChallenge.value, code)
    ElMessage.success('登录成功')
    await redirectAfterLogin()
  } catch (error) {
    ElMessage.error(error instanceof ApiError ? error.message : '验证失败，请重试')
  } finally {
    totpLoading.value = false
  }
}

function cancelTOTP(): void {
  totpChallenge.value = ''
  totpCode.value = ''
}

/** 打开找回密码对话框。 */
function openReset(): void {
  resetForm.email = ''
  resetForm.code = ''
  resetForm.password = ''
  resetForm.confirm = ''
  resetDialog.value = true
}

/** 发送密码重置验证码。 */
async function sendResetCode(): Promise<void> {
  if (!resetForm.email.trim()) {
    ElMessage.warning('请输入邮箱地址')
    return
  }
  resetSending.value = true
  try {
    await sendPasswordResetCode(resetForm.email.trim())
    ElMessage.success('若该邮箱已绑定账号，验证码将发送至邮箱')
  } catch (error) {
    ElMessage.error(error instanceof ApiError ? error.message : '发送失败，请稍后重试')
  } finally {
    resetSending.value = false
  }
}

/** 提交密码重置。 */
async function submitReset(): Promise<void> {
  if (!/^\d{6}$/.test(resetForm.code.trim())) {
    ElMessage.warning('请输入 6 位验证码')
    return
  }
  if (resetForm.password.length < 8) {
    ElMessage.warning('新密码至少 8 位')
    return
  }
  if (resetForm.password !== resetForm.confirm) {
    ElMessage.warning('两次输入的新密码不一致')
    return
  }
  resetSubmitting.value = true
  try {
    await resetPassword(resetForm.email.trim(), resetForm.code.trim(), resetForm.password)
    resetDialog.value = false
    ElMessage.success('密码已重置，请使用新密码登录')
  } catch (error) {
    ElMessage.error(error instanceof ApiError ? error.message : '重置失败，请稍后重试')
  } finally {
    resetSubmitting.value = false
  }
}

async function redirectAfterLogin(): Promise<void> {
  const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : ''
  if (redirect.startsWith('/')) {
    await router.replace(redirect)
    return
  }
  // 管理员登录页默认进入管理控制台；普通登录保持原有首页跳转。
  await router.replace(props.adminOnly ? '/admin' : '/')
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
      <button
        class="auth__theme"
        type="button"
        :aria-label="theme.isDark ? '切换到浅色主题' : '切换到深色主题'"
        :title="theme.isDark ? '切换到浅色主题' : '切换到深色主题'"
        @click="theme.toggle()"
      >
        <el-icon :size="16">
          <component :is="theme.isDark ? Sunny : Moon" />
        </el-icon>
      </button>
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
          v-if="!totpChallenge"
          ref="formRef"
          :model="form"
          :rules="rules"
          label-position="top"
          size="large"
          @submit.prevent="handleSubmit"
        >
          <el-form-item :label="mode === 'register' ? '用户名' : '用户名 / 邮箱'" prop="username">
            <el-input
              v-model="form.username"
              :prefix-icon="User"
              :placeholder="mode === 'register' ? '请输入用户名' : '请输入用户名或邮箱'"
              autocomplete="username"
              spellcheck="false"
            />
          </el-form-item>

          <template v-if="mode === 'register'">
            <el-form-item label="邮箱" prop="email">
              <el-input
                v-model="form.email"
                :prefix-icon="Message"
                placeholder="you@example.com"
                autocomplete="email"
                spellcheck="false"
              />
            </el-form-item>
            <el-form-item label="邮箱验证码" prop="code">
              <div class="auth__code-row">
                <el-input v-model="form.code" maxlength="6" inputmode="numeric" placeholder="6 位验证码" />
                <el-button :loading="codeSending" @click="sendRegCode">发送验证码</el-button>
              </div>
            </el-form-item>
          </template>

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

          <el-form-item v-if="mode === 'register'" label="确认密码" prop="confirmPassword">
            <el-input
              v-model="form.confirmPassword"
              type="password"
              show-password
              :prefix-icon="Lock"
              placeholder="请再次输入密码"
              autocomplete="new-password"
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

          <div v-if="mode === 'login'" class="auth__forgot">
            <button type="button" class="auth__switch-btn" @click="openReset">忘记密码？</button>
          </div>
        </el-form>

        <form v-else class="auth__totp" @submit.prevent="submitTOTP">
          <el-input
            v-model="totpCode"
            size="large"
            maxlength="6"
            placeholder="6 位动态验证码"
            inputmode="numeric"
            autocomplete="one-time-code"
            autofocus
          />
          <el-button
            type="primary"
            size="large"
            class="auth__submit"
            :loading="totpLoading"
            @click="submitTOTP"
          >
            验证并登录
          </el-button>
          <button type="button" class="auth__switch-btn auth__totp-back" @click="cancelTOTP">
            返回重新输入账号
          </button>
        </form>

        <p v-if="!totpChallenge" class="auth__switch">
          <template v-if="mode === 'register'">
            已有账号？
            <button type="button" class="auth__switch-btn" @click="switchMode">去登录</button>
          </template>
          <template v-else-if="mode === 'admin'">
            <router-link to="/login" class="auth__switch-link">普通用户登录</router-link>
          </template>
          <template v-else>
            还没有账号？
            <button type="button" class="auth__switch-btn" @click="switchMode">立即注册</button>
          </template>
        </p>
      </div>
    </section>

    <el-dialog
      v-model="resetDialog"
      title="找回密码"
      width="min(420px, 92vw)"
      append-to-body
      :close-on-click-modal="false"
    >
      <div class="auth__reset">
        <p class="auth__reset-desc">
          请输入已绑定账号的邮箱，获取验证码后设置新密码。未收到验证码说明该邮箱未绑定账号。
        </p>
        <div class="auth__reset-row">
          <el-input v-model="resetForm.email" placeholder="you@example.com" autocomplete="email" />
          <el-button :loading="resetSending" @click="sendResetCode">发送验证码</el-button>
        </div>
        <el-input
          v-model="resetForm.code"
          maxlength="6"
          inputmode="numeric"
          placeholder="6 位验证码"
        />
        <el-input
          v-model="resetForm.password"
          type="password"
          show-password
          placeholder="新密码（至少 8 位）"
          autocomplete="new-password"
        />
        <el-input
          v-model="resetForm.confirm"
          type="password"
          show-password
          placeholder="确认新密码"
          autocomplete="new-password"
        />
      </div>
      <template #footer>
        <el-button @click="resetDialog = false">取消</el-button>
        <el-button type="primary" :loading="resetSubmitting" @click="submitReset">重置密码</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.auth {
  display: grid;
  grid-template-columns: minmax(0, 1.05fr) minmax(0, 1fr);
  min-height: 100dvh;
}

/* ---- 品牌面板 ---- */
.auth__brand {
  position: relative;
  display: flex;
  align-items: center;
  overflow: hidden;
  background: var(--ax-brand-panel);
  border-right: 1px solid var(--ax-border-subtle);
}

.auth__aurora {
  position: absolute;
  inset: 0;
  background: var(--ax-aurora);
  pointer-events: none;
}

.auth__grid {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(var(--ax-brand-grid) 1px, transparent 1px),
    linear-gradient(90deg, var(--ax-brand-grid) 1px, transparent 1px);
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
  color: var(--ax-accent-bright);
  background: var(--ax-accent-soft);
  border: 1px solid var(--ax-accent-ring);
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
  color: var(--ax-accent-bright);
  background: var(--ax-tint);
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

/* ---- 表单面板 ---- */
.auth__panel {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--ax-space-8) var(--ax-space-6);
}

.auth__theme {
  position: absolute;
  top: var(--ax-space-6);
  right: var(--ax-space-6);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
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

.auth__theme:hover {
  color: var(--ax-text);
  background: var(--ax-tint-strong);
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

.auth__forgot {
  margin-top: var(--ax-space-3);
  font-size: var(--ax-text-sm);
  text-align: right;
}

.auth__reset {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-2);
}

.auth__reset-desc {
  margin: 0 0 var(--ax-space-2);
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
  line-height: 1.6;
}

.auth__reset-row {
  display: flex;
  gap: var(--ax-space-2);
}

.auth__code-row {
  display: flex;
  gap: var(--ax-space-2);
}

.auth__totp {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-2);
}

.auth__totp-back {
  margin-top: var(--ax-space-3);
  align-self: center;
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
  color: var(--ax-accent-hover);
}

.auth__switch-btn:focus-visible {
  outline: 2px solid var(--ax-focus);
  outline-offset: 2px;
  border-radius: 2px;
}

.auth__switch-link {
  color: var(--ax-accent-hover);
  font-weight: var(--ax-weight-medium);
  text-decoration: none;
}

.auth__switch-link:hover {
  text-decoration: underline;
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

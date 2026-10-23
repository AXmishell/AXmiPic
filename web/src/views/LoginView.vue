<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import type { FormInstance, FormRules } from 'element-plus'
import { Box, DataLine, Lock, Message, Moon, Sunny, UploadFilled, User } from '@element-plus/icons-vue'

import { ApiError } from '@/api/client'
import { resetPassword, sendPasswordResetCode, sendRegisterCode } from '@/api/auth'
import BrandMark from '@/components/BrandMark.vue'
import { useAuthStore } from '@/stores/auth'
import { useThemeStore } from '@/stores/theme'
import { confirmPasswordRule, usernameRules } from '@/utils/validate'

const { t } = useI18n()
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
      ? usernameRules(t('login.placeholders.username'))
      : [
          { required: true, message: t('login.validation.usernameOrEmailRequired'), trigger: 'blur' },
          { min: 3, max: 254, message: t('login.validation.usernameOrEmailInvalid'), trigger: 'blur' },
        ],
  email: [
    { required: true, message: t('login.validation.emailRequired'), trigger: 'blur' },
    { type: 'email', message: t('login.validation.emailInvalid'), trigger: ['blur', 'change'] },
  ],
  code: [
    { required: true, message: t('login.validation.codeRequired'), trigger: 'blur' },
    { pattern: /^\d{6}$/, message: t('login.validation.codeSixDigits'), trigger: 'blur' },
  ],
  password: [
    { required: true, message: t('login.validation.passwordRequired'), trigger: 'blur' },
    { min: 8, max: 72, message: t('login.validation.passwordLength'), trigger: 'blur' },
  ],
  confirmPassword: [confirmPasswordRule(() => form.password)],
}))

const heading = computed(() => {
  if (totpChallenge.value) return t('login.heading.totp')
  if (mode.value === 'admin') return t('login.heading.admin')
  return mode.value === 'login' ? t('login.heading.login') : t('login.heading.register')
})
const subheading = computed(() => {
  if (totpChallenge.value) return t('login.subheading.totp')
  if (mode.value === 'admin') return t('login.subheading.admin')
  return mode.value === 'login' ? t('login.subheading.login') : t('login.subheading.register')
})
const submitLabel = computed(() => {
  if (mode.value === 'admin') return t('login.submit.admin')
  return mode.value === 'login' ? t('login.submit.login') : t('login.submit.register')
})

function switchMode(): void {
  mode.value = mode.value === 'register' ? 'login' : 'register'
  formRef.value?.clearValidate()
}

/** 向邮箱发送注册验证码。 */
async function sendRegCode(): Promise<void> {
  const email = form.email.trim()
  if (!email) {
    ElMessage.warning(t('login.messages.enterEmail'))
    return
  }
  codeSending.value = true
  try {
    await sendRegisterCode(email)
    ElMessage.success(t('login.messages.codeSent'))
  } catch (error) {
    ElMessage.error(error instanceof ApiError ? error.message : t('login.messages.sendFailed'))
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
      ElMessage.info(t('login.messages.enterTotp'))
      return
    }
    ElMessage.success(mode.value === 'register' ? t('login.messages.registerSuccess') : t('login.messages.loginSuccess'))
    await redirectAfterLogin()
  } catch (error) {
    ElMessage.error(error instanceof ApiError ? error.message : t('login.messages.operationFailed'))
  } finally {
    loading.value = false
  }
}

/** 完成 TOTP 二次验证并进入控制台。 */
async function submitTOTP(): Promise<void> {
  const code = totpCode.value.trim()
  if (!/^\d{6}$/.test(code)) {
    ElMessage.warning(t('login.messages.enterTotp6'))
    return
  }
  totpLoading.value = true
  try {
    await auth.verifyTOTP(totpChallenge.value, code)
    ElMessage.success(t('login.messages.loginSuccess'))
    await redirectAfterLogin()
  } catch (error) {
    ElMessage.error(error instanceof ApiError ? error.message : t('login.messages.verifyFailed'))
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
    ElMessage.warning(t('login.messages.enterEmailAddress'))
    return
  }
  resetSending.value = true
  try {
    await sendPasswordResetCode(resetForm.email.trim())
    ElMessage.success(t('login.messages.resetCodeSent'))
  } catch (error) {
    ElMessage.error(error instanceof ApiError ? error.message : t('login.messages.sendFailed'))
  } finally {
    resetSending.value = false
  }
}

/** 提交密码重置。 */
async function submitReset(): Promise<void> {
  if (!/^\d{6}$/.test(resetForm.code.trim())) {
    ElMessage.warning(t('login.messages.enterTotp6'))
    return
  }
  if (resetForm.password.length < 8) {
    ElMessage.warning(t('login.messages.newPasswordMin'))
    return
  }
  if (resetForm.password !== resetForm.confirm) {
    ElMessage.warning(t('login.messages.newPasswordMismatch'))
    return
  }
  resetSubmitting.value = true
  try {
    await resetPassword(resetForm.email.trim(), resetForm.code.trim(), resetForm.password)
    resetDialog.value = false
    ElMessage.success(t('login.messages.resetSuccess'))
  } catch (error) {
    ElMessage.error(error instanceof ApiError ? error.message : t('login.messages.resetFailed'))
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
            <BrandMark :size="20" />
          </span>
          <span class="auth__logo-name">AXmiPic</span>
        </div>

        <h2 class="auth__headline">{{ t('login.brandHeadline') }}</h2>
        <p class="auth__subline">{{ t('login.brandSubline') }}</p>

        <ul class="auth__features">
          <li class="auth__feature">
            <span class="auth__feature-icon"><el-icon :size="15"><UploadFilled /></el-icon></span>
            <div>
              <strong>{{ t('login.features.fastUpload.title') }}</strong>
              <span>{{ t('login.features.fastUpload.desc') }}</span>
            </div>
          </li>
          <li class="auth__feature">
            <span class="auth__feature-icon"><el-icon :size="15"><Box /></el-icon></span>
            <div>
              <strong>{{ t('login.features.storage.title') }}</strong>
              <span>{{ t('login.features.storage.desc') }}</span>
            </div>
          </li>
          <li class="auth__feature">
            <span class="auth__feature-icon"><el-icon :size="15"><DataLine /></el-icon></span>
            <div>
              <strong>{{ t('login.features.usage.title') }}</strong>
              <span>{{ t('login.features.usage.desc') }}</span>
            </div>
          </li>
        </ul>

        <p class="auth__footnote">{{ t('login.footnote') }}</p>
      </div>
    </section>

    <section class="auth__panel">
      <button
        class="auth__theme"
        type="button"
        :aria-label="theme.isDark ? t('topbar.switchToLight') : t('topbar.switchToDark')"
        :title="theme.isDark ? t('topbar.switchToLight') : t('topbar.switchToDark')"
        @click="theme.toggle()"
      >
        <el-icon :size="16">
          <component :is="theme.isDark ? Sunny : Moon" />
        </el-icon>
      </button>
      <div class="auth__card">
        <div class="auth__mobile-logo">
          <span class="auth__logo-mark" aria-hidden="true">
            <BrandMark :size="18" />
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
          :validate-on-rule-change="false"
          label-position="top"
          size="large"
          @submit.prevent="handleSubmit"
        >
          <el-form-item :label="mode === 'register' ? t('login.labels.username') : t('login.labels.usernameOrEmail')" prop="username">
            <el-input
              v-model="form.username"
              :prefix-icon="User"
              :placeholder="mode === 'register' ? t('login.placeholders.username') : t('login.placeholders.usernameOrEmail')"
              autocomplete="username"
              spellcheck="false"
            />
          </el-form-item>

          <template v-if="mode === 'register'">
            <el-form-item :label="t('login.labels.email')" prop="email">
              <el-input
                v-model="form.email"
                :prefix-icon="Message"
                :placeholder="t('login.placeholders.email')"
                autocomplete="email"
                spellcheck="false"
              />
            </el-form-item>
            <el-form-item :label="t('login.labels.emailCode')" prop="code">
              <div class="auth__code-row">
                <el-input v-model="form.code" maxlength="6" inputmode="numeric" :placeholder="t('login.placeholders.code')" />
                <el-button :loading="codeSending" @click="sendRegCode">{{ t('login.actions.sendCode') }}</el-button>
              </div>
            </el-form-item>
          </template>

          <el-form-item :label="t('login.labels.password')" prop="password">
            <el-input
              v-model="form.password"
              type="password"
              show-password
              :prefix-icon="Lock"
              :placeholder="t('login.placeholders.password')"
              :autocomplete="mode === 'login' ? 'current-password' : 'new-password'"
              @keyup.enter="handleSubmit"
            />
          </el-form-item>

          <el-form-item v-if="mode === 'register'" :label="t('login.labels.confirmPassword')" prop="confirmPassword">
            <el-input
              v-model="form.confirmPassword"
              type="password"
              show-password
              :prefix-icon="Lock"
              :placeholder="t('login.placeholders.confirmPassword')"
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
            <button type="button" class="auth__switch-btn" @click="openReset">{{ t('login.actions.forgotPassword') }}</button>
          </div>
        </el-form>

        <form v-else class="auth__totp" @submit.prevent="submitTOTP">
          <el-input
            v-model="totpCode"
            size="large"
            maxlength="6"
            :placeholder="t('login.placeholders.dynamicCode')"
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
            {{ t('login.actions.verifyAndSignIn') }}
          </el-button>
          <button type="button" class="auth__switch-btn auth__totp-back" @click="cancelTOTP">
            {{ t('login.actions.backToAccount') }}
          </button>
        </form>

        <p v-if="!totpChallenge" class="auth__switch">
          <template v-if="mode === 'register'">
            {{ t('login.switch.hasAccount') }}
            <button type="button" class="auth__switch-btn" @click="switchMode">{{ t('login.actions.goSignIn') }}</button>
          </template>
          <template v-else-if="mode === 'admin'">
            <router-link to="/login" class="auth__switch-link">{{ t('login.actions.normalLogin') }}</router-link>
          </template>
          <template v-else>
            {{ t('login.switch.noAccount') }}
            <button type="button" class="auth__switch-btn" @click="switchMode">{{ t('login.actions.registerNow') }}</button>
          </template>
        </p>
      </div>
    </section>

    <el-dialog
      v-model="resetDialog"
      :title="t('login.reset.title')"
      width="min(420px, 92vw)"
      append-to-body
      :close-on-click-modal="false"
    >
      <div class="auth__reset">
        <p class="auth__reset-desc">
          {{ t('login.reset.description') }}
        </p>
        <div class="auth__reset-row">
          <el-input v-model="resetForm.email" :placeholder="t('login.placeholders.email')" autocomplete="email" />
          <el-button :loading="resetSending" @click="sendResetCode">{{ t('login.actions.sendCode') }}</el-button>
        </div>
        <el-input
          v-model="resetForm.code"
          maxlength="6"
          inputmode="numeric"
          :placeholder="t('login.placeholders.code')"
        />
        <el-input
          v-model="resetForm.password"
          type="password"
          show-password
          :placeholder="t('login.placeholders.newPassword')"
          autocomplete="new-password"
        />
        <el-input
          v-model="resetForm.confirm"
          type="password"
          show-password
          :placeholder="t('login.placeholders.confirmNewPassword')"
          autocomplete="new-password"
        />
      </div>
      <template #footer>
        <el-button @click="resetDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="resetSubmitting" @click="submitReset">{{ t('login.reset.submit') }}</el-button>
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

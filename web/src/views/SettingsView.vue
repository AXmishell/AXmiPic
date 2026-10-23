<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { EditPen, Key, Refresh, SwitchButton } from '@element-plus/icons-vue'
import QRCode from 'qrcode'

import {
  changePassword,
  changeUsername,
  disableTOTP,
  enableTOTP,
  fetchSecurity,
  sendEmailCode,
  setupTOTP,
  unbindEmail,
  verifyEmail,
  type SecurityInfo,
} from '@/api/auth'
import { toApiError } from '@/api/client'
import ErrorState from '@/components/ErrorState.vue'
import PageHeader from '@/components/PageHeader.vue'
import QuotaMeter from '@/components/QuotaMeter.vue'
import UserAvatar from '@/components/UserAvatar.vue'
import { useAuthStore } from '@/stores/auth'
import { useThemeStore, type ThemeMode } from '@/stores/theme'
import { formatDateTime } from '@/utils/format'
import { USERNAME_PATTERN, usernameRuleMessage } from '@/utils/validate'

const { t } = useI18n()
const auth = useAuthStore()
const theme = useThemeStore()
const router = useRouter()

const loading = ref(false)
const errorMessage = ref('')

const me = computed(() => auth.user)
const isAdmin = computed(() => auth.isAdmin)

const themeOptions = computed<{ value: ThemeMode; label: string }[]>(() => [
  { value: 'dark', label: t('settings.theme.dark') },
  { value: 'light', label: t('settings.theme.light') },
])

function setTheme(mode: ThemeMode): void {
  theme.setMode(mode)
}

// ---- 安全设置：TOTP 与邮箱绑定 ----
const security = ref<SecurityInfo | null>(null)

// TOTP 开启流程
const totpDialog = ref(false)
const totpBusy = ref(false)
const totpSecret = ref('')
const totpQr = ref('')
const totpCode = ref('')

// TOTP 关闭流程
const disableDialog = ref(false)
const disableBusy = ref(false)
const disableCode = ref('')
const disablePassword = ref('')

// 邮箱绑定
const emailEditing = ref(false)
const emailForm = reactive({ email: '', code: '', password: '' })
const emailSending = ref(false)
const emailBinding = ref(false)
const unbindDialog = ref(false)
const unbindBusy = ref(false)
const unbindPassword = ref('')

// 修改密码
const passwordDialog = ref(false)
const passwordBusy = ref(false)
const passwordForm = reactive({ current: '', next: '', confirm: '' })

// 修改用户名
const usernameDialog = ref(false)
const usernameBusy = ref(false)
const usernameForm = reactive({ username: '', password: '', totpCode: '' })

const emailBound = computed(() => Boolean(security.value?.email && security.value.email_verified))
const showEmailForm = computed(() => !emailBound.value || emailEditing.value)

async function loadSecurity(): Promise<void> {
  try {
    security.value = await fetchSecurity()
  } catch {
    // 安全信息拉取失败时保留上一次的状态。
  }
}

async function refreshAccount(): Promise<void> {
  await auth.refreshUser()
  await loadSecurity()
}

async function openSetupTOTP(): Promise<void> {
  totpBusy.value = true
  try {
    const setup = await setupTOTP()
    totpSecret.value = setup.secret
    totpQr.value = await QRCode.toDataURL(setup.uri, { margin: 1, width: 220 })
    totpCode.value = ''
    totpDialog.value = true
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    totpBusy.value = false
  }
}

async function confirmEnableTOTP(): Promise<void> {
  if (!/^\d{6}$/.test(totpCode.value.trim())) {
    ElMessage.warning(t('settings.messages.enterTotp6'))
    return
  }
  totpBusy.value = true
  try {
    await enableTOTP(totpCode.value.trim())
    totpDialog.value = false
    await refreshAccount()
    ElMessage.success(t('settings.messages.totpEnabled'))
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    totpBusy.value = false
  }
}

function openDisableTOTP(): void {
  disableCode.value = ''
  disablePassword.value = ''
  disableDialog.value = true
}

async function confirmDisableTOTP(): Promise<void> {
  if (!disableCode.value.trim() && !disablePassword.value) {
    ElMessage.warning(t('settings.messages.enterTotpOrPassword'))
    return
  }
  disableBusy.value = true
  try {
    await disableTOTP({
      code: disableCode.value.trim() || undefined,
      password: disablePassword.value || undefined,
    })
    disableDialog.value = false
    await refreshAccount()
    ElMessage.success(t('settings.messages.totpDisabled'))
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    disableBusy.value = false
  }
}

function startChangeEmail(): void {
  emailEditing.value = true
  emailForm.email = ''
  emailForm.code = ''
  emailForm.password = ''
}

async function sendCode(): Promise<void> {
  if (!emailForm.email.trim()) {
    ElMessage.warning(t('settings.messages.enterEmailAddress'))
    return
  }
  emailSending.value = true
  try {
    await sendEmailCode(emailForm.email.trim())
    ElMessage.success(t('settings.messages.codeSent'))
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    emailSending.value = false
  }
}

async function bindEmail(): Promise<void> {
  if (!/^\d{6}$/.test(emailForm.code.trim())) {
    ElMessage.warning(t('settings.messages.enterCode6'))
    return
  }
  if (emailBound.value && !emailForm.password) {
    ElMessage.warning(t('settings.messages.changeEmailPasswordRequired'))
    return
  }
  emailBinding.value = true
  try {
    await verifyEmail(emailForm.email.trim(), emailForm.code.trim(), emailForm.password || undefined)
    emailEditing.value = false
    emailForm.code = ''
    emailForm.password = ''
    await refreshAccount()
    ElMessage.success(t('settings.messages.emailBound'))
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    emailBinding.value = false
  }
}

async function confirmUnbindEmail(): Promise<void> {
  if (!unbindPassword.value) {
    ElMessage.warning(t('settings.messages.enterCurrentPassword'))
    return
  }
  unbindBusy.value = true
  try {
    await unbindEmail(unbindPassword.value)
    unbindDialog.value = false
    unbindPassword.value = ''
    emailEditing.value = false
    await refreshAccount()
    ElMessage.success(t('settings.messages.emailUnbound'))
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    unbindBusy.value = false
  }
}

function openChangeUsername(): void {
  usernameForm.username = me.value?.username ?? ''
  usernameForm.password = ''
  usernameForm.totpCode = ''
  usernameDialog.value = true
}

async function confirmChangeUsername(): Promise<void> {
  const username = usernameForm.username.trim()
  if (!USERNAME_PATTERN.test(username)) {
    ElMessage.warning(usernameRuleMessage())
    return
  }
  if (!usernameForm.password) {
    ElMessage.warning(t('settings.messages.enterCurrentPassword'))
    return
  }
  if (security.value?.totp_enabled && !/^\d{6}$/.test(usernameForm.totpCode.trim())) {
    ElMessage.warning(t('settings.messages.enterTotp6'))
    return
  }
  usernameBusy.value = true
  try {
    await changeUsername({
      username,
      current_password: usernameForm.password,
      totp_code: security.value?.totp_enabled ? usernameForm.totpCode.trim() : undefined,
    })
    usernameDialog.value = false
    await refreshAccount()
    ElMessage.success(t('settings.messages.usernameUpdated'))
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    usernameBusy.value = false
  }
}

function openChangePassword(): void {
  passwordForm.current = ''
  passwordForm.next = ''
  passwordForm.confirm = ''
  passwordDialog.value = true
}

async function confirmChangePassword(): Promise<void> {
  if (!passwordForm.current) {
    ElMessage.warning(t('settings.messages.enterCurrentPassword'))
    return
  }
  if (passwordForm.next.length < 8) {
    ElMessage.warning(t('settings.messages.newPasswordMin'))
    return
  }
  if (passwordForm.next !== passwordForm.confirm) {
    ElMessage.warning(t('settings.messages.newPasswordMismatch'))
    return
  }
  passwordBusy.value = true
  try {
    await changePassword(passwordForm.current, passwordForm.next)
    passwordDialog.value = false
    ElMessage.success(t('settings.messages.passwordChanged'))
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    passwordBusy.value = false
  }
}

async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    await auth.refreshUser()
    await loadSecurity()
  } catch (error) {
    errorMessage.value = toApiError(error).message
  } finally {
    loading.value = false
  }
}

async function handleLogout(): Promise<void> {
  try {
    await ElMessageBox.confirm(t('settings.logout.message'), t('topbar.logoutConfirmTitle'), {
      confirmButtonText: t('topbar.logout'),
      cancelButtonText: t('common.cancel'),
      type: 'warning',
    })
  } catch {
    return
  }
  auth.logout()
  ElMessage.success(t('topbar.loggedOut'))
  await router.replace('/login')
}

onMounted(load)
</script>

<template>
  <div class="ax-page">
    <PageHeader :title="t('settings.title')" :description="t('settings.description')">
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="load" />

    <div v-else-if="loading && !me" class="ax-card" aria-busy="true">
      <div class="ax-card__body"><el-skeleton :rows="6" animated /></div>
    </div>

    <template v-else-if="me">
      <section class="settings-grid">
        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">{{ t('settings.profile.title') }}</h2>
          </header>
          <div class="ax-card__body settings-profile">
            <div class="settings-identity">
              <UserAvatar :name="me.username" size="lg" />
              <div class="settings-identity__meta">
                <p class="settings-identity__name">
                  <span class="settings-identity__text">{{ me.username }}</span>
                  <el-button
                    link
                    type="primary"
                    :icon="EditPen"
                    :title="t('settings.profile.changeUsername')"
                    :aria-label="t('settings.profile.changeUsername')"
                    @click="openChangeUsername"
                  />
                </p>
                <el-tag size="small" :type="isAdmin ? 'primary' : 'info'" effect="plain">
                  {{ isAdmin ? t('topbar.roleAdmin') : t('topbar.roleUser') }}
                </el-tag>
              </div>
            </div>
            <dl class="info-list">
              <div class="info-list__row">
                <dt>{{ t('settings.profile.userId') }}</dt>
                <dd class="ax-mono">{{ me.id }}</dd>
              </div>
              <div class="info-list__row">
                <dt>{{ t('settings.profile.role') }}</dt>
                <dd>{{ isAdmin ? t('topbar.roleAdmin') : t('topbar.roleUser') }}</dd>
              </div>
              <div class="info-list__row">
                <dt>{{ t('settings.profile.createdAt') }}</dt>
                <dd>{{ formatDateTime(me.created_at) }}</dd>
              </div>
              <div class="info-list__row">
                <dt>{{ t('common.status') }}</dt>
                <dd>{{ me.disabled ? t('common.disabled') : t('settings.profile.statusActive') }}</dd>
              </div>
            </dl>
          </div>
        </article>

        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">{{ t('settings.quota.title') }}</h2>
          </header>
          <div class="ax-card__body settings-quota">
            <QuotaMeter :used="me.used_bytes" :quota="me.quota_bytes" />
            <el-button
              v-if="!isAdmin"
              size="small"
              :icon="Key"
              @click="router.push('/user/tokens')"
            >
              {{ t('settings.quota.manageTokens') }}
            </el-button>
          </div>
        </article>
      </section>

      <section class="settings-grid settings-block">
        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">{{ t('settings.totp.title') }}</h2>
            <el-tag size="small" :type="security?.totp_enabled ? 'success' : 'info'" effect="plain">
              {{ security?.totp_enabled ? t('common.enabled') : t('settings.totp.notEnabled') }}
            </el-tag>
          </header>
          <div class="ax-card__body security-card">
            <p class="security-card__desc">
              {{ t('settings.totp.description') }}
            </p>
            <el-alert
              v-if="security && !security.totp_available"
              type="warning"
              :closable="false"
              :title="t('settings.totp.unavailable')"
            />
            <div class="security-card__actions">
              <el-button
                v-if="!security?.totp_enabled"
                type="primary"
                :loading="totpBusy"
                :disabled="security ? !security.totp_available : true"
                @click="openSetupTOTP"
              >
                {{ t('settings.totp.enable') }}
              </el-button>
              <el-button v-else type="danger" plain @click="openDisableTOTP">{{ t('settings.totp.disable') }}</el-button>
            </div>
          </div>
        </article>

        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">{{ t('settings.email.title') }}</h2>
            <el-tag v-if="emailBound" size="small" type="success" effect="plain">{{ t('settings.email.verified') }}</el-tag>
            <el-tag v-else size="small" type="info" effect="plain">{{ t('settings.email.unbound') }}</el-tag>
          </header>
          <div class="ax-card__body security-card">
            <template v-if="emailBound && !showEmailForm">
              <dl class="info-list">
                <div class="info-list__row">
                  <dt>{{ t('settings.email.boundEmail') }}</dt>
                  <dd>{{ security?.email }}</dd>
                </div>
              </dl>
              <div class="security-card__actions">
                <el-button size="small" @click="startChangeEmail">{{ t('settings.email.change') }}</el-button>
                <el-button size="small" type="danger" plain @click="unbindDialog = true">{{ t('settings.email.unbind') }}</el-button>
              </div>
            </template>
            <template v-else>
              <el-alert
                v-if="security && !security.email_available"
                type="warning"
                :closable="false"
                :title="t('settings.email.unavailable')"
              />
              <div class="email-form">
                <el-input v-model="emailForm.email" :placeholder="t('settings.email.placeholder')" autocomplete="email" />
                <div class="email-form__row">
                  <el-input
                    v-model="emailForm.code"
                    maxlength="6"
                    inputmode="numeric"
                    :placeholder="t('settings.email.codePlaceholder')"
                  />
                  <el-button
                    :loading="emailSending"
                    :disabled="security ? !security.email_available : false"
                    @click="sendCode"
                  >
                    {{ t('settings.email.sendCode') }}
                  </el-button>
                </div>
                <el-input
                  v-if="emailBound"
                  v-model="emailForm.password"
                  type="password"
                  show-password
                  :placeholder="t('settings.email.currentPasswordForChange')"
                  autocomplete="current-password"
                />
                <p class="security-card__hint">{{ t('settings.email.rateHint') }}</p>
                <div class="security-card__actions">
                  <el-button type="primary" :loading="emailBinding" @click="bindEmail">
                    {{ emailBound ? t('settings.email.confirmChange') : t('settings.email.bind') }}
                  </el-button>
                  <el-button v-if="emailBound" @click="emailEditing = false">{{ t('common.cancel') }}</el-button>
                </div>
              </div>
            </template>
          </div>
        </article>

        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">{{ t('settings.password.title') }}</h2>
          </header>
          <div class="ax-card__body security-card">
            <p class="security-card__desc">
              {{ t('settings.password.description') }}
            </p>
            <div class="security-card__actions">
              <el-button type="primary" plain :icon="Key" @click="openChangePassword">{{ t('settings.password.change') }}</el-button>
            </div>
          </div>
        </article>
      </section>

      <article class="ax-card settings-block">
        <header class="ax-card__head">
          <h2 class="ax-card__title">{{ t('settings.theme.title') }}</h2>
        </header>
        <div class="ax-card__body settings-theme">
          <div class="settings-theme__text">
            <strong>{{ t('settings.theme.label') }}</strong>
            <span>{{ t('settings.theme.description') }}</span>
          </div>
          <el-radio-group :model-value="theme.mode" @change="setTheme($event as ThemeMode)">
            <el-radio-button v-for="option in themeOptions" :key="option.value" :value="option.value">
              {{ option.label }}
            </el-radio-button>
          </el-radio-group>
        </div>
      </article>

      <article class="ax-card settings-block">
        <header class="ax-card__head">
          <h2 class="ax-card__title">{{ t('settings.session.title') }}</h2>
        </header>
        <div class="ax-card__body settings-session">
          <div class="settings-session__text">
            <strong>{{ t('settings.session.logoutTitle') }}</strong>
            <span>{{ t('settings.session.logoutDesc') }}</span>
          </div>
          <el-button type="danger" plain :icon="SwitchButton" @click="handleLogout">
            {{ t('topbar.logout') }}
          </el-button>
        </div>
      </article>
    </template>

    <!-- TOTP 开启 -->
    <el-dialog
      v-model="totpDialog"
      :title="t('settings.totp.enable')"
      width="min(420px, 92vw)"
      append-to-body
      :close-on-click-modal="false"
    >
      <div class="totp-setup">
        <p class="security-card__desc">
          {{ t('settings.totp.setupDesc') }}
        </p>
        <div class="totp-setup__qr">
          <img v-if="totpQr" :src="totpQr" :alt="t('settings.totp.qrAlt')" width="220" height="220" />
        </div>
        <el-input :model-value="totpSecret" readonly>
          <template #prepend>{{ t('settings.totp.secret') }}</template>
        </el-input>
        <el-input
          v-model="totpCode"
          maxlength="6"
          inputmode="numeric"
          :placeholder="t('settings.totp.setupCodePlaceholder')"
        />
      </div>
      <template #footer>
        <el-button @click="totpDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="totpBusy" @click="confirmEnableTOTP">{{ t('settings.totp.confirmEnable') }}</el-button>
      </template>
    </el-dialog>

    <!-- TOTP 关闭 -->
    <el-dialog v-model="disableDialog" :title="t('settings.totp.disable')" width="min(420px, 92vw)" append-to-body>
      <p class="security-card__desc">{{ t('settings.totp.disableDesc') }}</p>
      <el-input
        v-model="disableCode"
        maxlength="6"
        inputmode="numeric"
        :placeholder="t('settings.totp.dynamicCodePlaceholder')"
        class="dialog-field"
      />
      <el-input
        v-model="disablePassword"
        type="password"
        show-password
        :placeholder="t('settings.totp.currentPasswordOptional')"
        class="dialog-field"
      />
      <template #footer>
        <el-button @click="disableDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="danger" :loading="disableBusy" @click="confirmDisableTOTP">{{ t('settings.totp.confirmDisable') }}</el-button>
      </template>
    </el-dialog>

    <!-- 解绑邮箱 -->
    <el-dialog v-model="unbindDialog" :title="t('settings.email.unbindTitle')" width="min(420px, 92vw)" append-to-body>
      <p class="security-card__desc">{{ t('settings.email.unbindDesc') }}</p>
      <el-input v-model="unbindPassword" type="password" show-password :placeholder="t('settings.email.currentPassword')" />
      <template #footer>
        <el-button @click="unbindDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="danger" :loading="unbindBusy" @click="confirmUnbindEmail">{{ t('settings.email.confirmUnbind') }}</el-button>
      </template>
    </el-dialog>

    <!-- 修改用户名 -->
    <el-dialog
      v-model="usernameDialog"
      :title="t('settings.profile.changeUsername')"
      width="min(420px, 92vw)"
      append-to-body
      :close-on-click-modal="false"
    >
      <div class="email-form">
        <el-input
          v-model="usernameForm.username"
          maxlength="64"
          :placeholder="t('settings.profile.newUsernamePlaceholder')"
          autocomplete="username"
        />
        <el-input
          v-model="usernameForm.password"
          type="password"
          show-password
          :placeholder="t('settings.profile.currentPassword')"
          autocomplete="current-password"
        />
        <el-input
          v-if="security?.totp_enabled"
          v-model="usernameForm.totpCode"
          maxlength="6"
          inputmode="numeric"
          :placeholder="t('settings.totp.dynamicCodePlaceholder')"
          autocomplete="one-time-code"
        />
        <p class="security-card__hint">
          {{ t('settings.profile.usernameHint') }}
        </p>
      </div>
      <template #footer>
        <el-button @click="usernameDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="usernameBusy" @click="confirmChangeUsername">
          {{ t('settings.profile.confirmChange') }}
        </el-button>
      </template>
    </el-dialog>

    <!-- 修改密码 -->
    <el-dialog
      v-model="passwordDialog"
      :title="t('settings.password.change')"
      width="min(420px, 92vw)"
      append-to-body
      :close-on-click-modal="false"
    >
      <div class="email-form">
        <el-input
          v-model="passwordForm.current"
          type="password"
          show-password
          :placeholder="t('settings.password.currentPlaceholder')"
          autocomplete="current-password"
        />
        <el-input
          v-model="passwordForm.next"
          type="password"
          show-password
          :placeholder="t('settings.password.newPlaceholder')"
          autocomplete="new-password"
        />
        <el-input
          v-model="passwordForm.confirm"
          type="password"
          show-password
          :placeholder="t('settings.password.confirmPlaceholder')"
          autocomplete="new-password"
        />
      </div>
      <template #footer>
        <el-button @click="passwordDialog = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="passwordBusy" @click="confirmChangePassword">{{ t('settings.password.confirmChange') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.settings-grid {
  display: grid;
  gap: var(--ax-space-4);
  grid-template-columns: repeat(auto-fit, minmax(min(320px, 100%), 1fr));
  align-items: start;
}

.settings-block {
  margin-top: var(--ax-space-4);
}

.settings-profile {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-5);
}

.settings-identity {
  display: flex;
  align-items: center;
  gap: var(--ax-space-3);
}

.settings-identity__meta {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--ax-space-1);
  min-width: 0;
}

.settings-identity__name {
  display: flex;
  align-items: center;
  gap: 2px;
  max-width: 100%;
  color: var(--ax-text);
  font-size: var(--ax-text-md);
  font-weight: var(--ax-weight-semibold);
}

.settings-identity__text {
  overflow: hidden;
  min-width: 0;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.settings-quota {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-5);
  align-items: flex-start;
}

.info-list {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-3);
  margin: 0;
  width: 100%;
}

.info-list__row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--ax-space-4);
  min-width: 0;
}

.info-list__row dt {
  flex: none;
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
}

.info-list__row dd {
  margin: 0;
  min-width: 0;
  color: var(--ax-text-2);
  font-size: var(--ax-text-sm);
  text-align: right;
  overflow-wrap: anywhere;
}

.settings-session {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--ax-space-4);
}

.settings-theme {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--ax-space-4);
}

.settings-theme__text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 200px;
}

.settings-theme__text strong {
  color: var(--ax-text-2);
  font-size: var(--ax-text-sm);
  font-weight: var(--ax-weight-medium);
}

.settings-theme__text span {
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.settings-session__text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 200px;
}

.settings-session__text strong {
  color: var(--ax-text-2);
  font-size: var(--ax-text-sm);
  font-weight: var(--ax-weight-medium);
}

.settings-session__text span {
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.security-card {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-4);
}

.security-card__desc {
  margin: 0;
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
  line-height: 1.6;
}

.security-card__hint {
  margin: 0;
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.security-card__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--ax-space-2);
}

.email-form {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-2);
}

.email-form__row {
  display: flex;
  gap: var(--ax-space-2);
}

.totp-setup {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-3);
}

.totp-setup__qr {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--ax-space-3);
  background: #fff;
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-md);
}

.totp-setup__qr img {
  display: block;
}

.dialog-field {
  margin-bottom: var(--ax-space-2);
}
</style>

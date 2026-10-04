<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Key, Refresh, SwitchButton } from '@element-plus/icons-vue'
import QRCode from 'qrcode'

import {
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

const auth = useAuthStore()
const theme = useThemeStore()
const router = useRouter()

const loading = ref(false)
const errorMessage = ref('')

const me = computed(() => auth.user)
const isAdmin = computed(() => auth.isAdmin)

const themeOptions: { value: ThemeMode; label: string }[] = [
  { value: 'dark', label: '深色' },
  { value: 'light', label: '浅色' },
]

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
    ElMessage.warning('请输入 6 位动态验证码')
    return
  }
  totpBusy.value = true
  try {
    await enableTOTP(totpCode.value.trim())
    totpDialog.value = false
    await refreshAccount()
    ElMessage.success('二次验证已启用')
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
    ElMessage.warning('请输入动态验证码或当前密码')
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
    ElMessage.success('二次验证已关闭')
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
    ElMessage.warning('请输入邮箱地址')
    return
  }
  emailSending.value = true
  try {
    await sendEmailCode(emailForm.email.trim())
    ElMessage.success('验证码已发送，请查收邮箱')
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    emailSending.value = false
  }
}

async function bindEmail(): Promise<void> {
  if (!/^\d{6}$/.test(emailForm.code.trim())) {
    ElMessage.warning('请输入 6 位验证码')
    return
  }
  if (emailBound.value && !emailForm.password) {
    ElMessage.warning('换绑邮箱需要输入当前密码')
    return
  }
  emailBinding.value = true
  try {
    await verifyEmail(emailForm.email.trim(), emailForm.code.trim(), emailForm.password || undefined)
    emailEditing.value = false
    emailForm.code = ''
    emailForm.password = ''
    await refreshAccount()
    ElMessage.success('邮箱绑定成功')
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    emailBinding.value = false
  }
}

async function confirmUnbindEmail(): Promise<void> {
  if (!unbindPassword.value) {
    ElMessage.warning('请输入当前密码')
    return
  }
  unbindBusy.value = true
  try {
    await unbindEmail(unbindPassword.value)
    unbindDialog.value = false
    unbindPassword.value = ''
    emailEditing.value = false
    await refreshAccount()
    ElMessage.success('邮箱已解绑')
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    unbindBusy.value = false
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
    await ElMessageBox.confirm('退出后需要重新登录才能继续使用控制台。', '确定退出登录吗？', {
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

onMounted(load)
</script>

<template>
  <div class="ax-page">
    <PageHeader title="账号设置" description="查看账户信息、配额与登录状态。">
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
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
            <h2 class="ax-card__title">账户信息</h2>
          </header>
          <div class="ax-card__body settings-profile">
            <div class="settings-identity">
              <UserAvatar :name="me.username" size="lg" />
              <div class="settings-identity__meta">
                <p class="settings-identity__name">{{ me.username }}</p>
                <el-tag size="small" :type="isAdmin ? 'primary' : 'info'" effect="plain">
                  {{ isAdmin ? '管理员' : '用户' }}
                </el-tag>
              </div>
            </div>
            <dl class="info-list">
              <div class="info-list__row">
                <dt>用户 ID</dt>
                <dd class="ax-mono">{{ me.id }}</dd>
              </div>
              <div class="info-list__row">
                <dt>角色</dt>
                <dd>{{ isAdmin ? '管理员' : '用户' }}</dd>
              </div>
              <div class="info-list__row">
                <dt>注册时间</dt>
                <dd>{{ formatDateTime(me.created_at) }}</dd>
              </div>
              <div class="info-list__row">
                <dt>状态</dt>
                <dd>{{ me.disabled ? '已禁用' : '正常' }}</dd>
              </div>
            </dl>
          </div>
        </article>

        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">存储配额</h2>
          </header>
          <div class="ax-card__body settings-quota">
            <QuotaMeter :used="me.used_bytes" :quota="me.quota_bytes" />
            <el-button
              v-if="!isAdmin"
              size="small"
              :icon="Key"
              @click="router.push('/user/tokens')"
            >
              管理访问令牌
            </el-button>
          </div>
        </article>
      </section>

      <section class="settings-grid settings-block">
        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">二次验证（TOTP）</h2>
            <el-tag size="small" :type="security?.totp_enabled ? 'success' : 'info'" effect="plain">
              {{ security?.totp_enabled ? '已启用' : '未启用' }}
            </el-tag>
          </header>
          <div class="ax-card__body security-card">
            <p class="security-card__desc">
              开启后，登录时除密码外还需输入身份验证器（如 Google Authenticator、1Password）生成的动态验证码。
            </p>
            <el-alert
              v-if="security && !security.totp_available"
              type="warning"
              :closable="false"
              title="服务端未配置加密主密钥，暂不可启用二次验证。"
            />
            <div class="security-card__actions">
              <el-button
                v-if="!security?.totp_enabled"
                type="primary"
                :loading="totpBusy"
                :disabled="security ? !security.totp_available : true"
                @click="openSetupTOTP"
              >
                开启二次验证
              </el-button>
              <el-button v-else type="danger" plain @click="openDisableTOTP">关闭二次验证</el-button>
            </div>
          </div>
        </article>

        <article class="ax-card">
          <header class="ax-card__head">
            <h2 class="ax-card__title">邮箱绑定</h2>
            <el-tag v-if="emailBound" size="small" type="success" effect="plain">已验证</el-tag>
            <el-tag v-else size="small" type="info" effect="plain">未绑定</el-tag>
          </header>
          <div class="ax-card__body security-card">
            <template v-if="emailBound && !showEmailForm">
              <dl class="info-list">
                <div class="info-list__row">
                  <dt>已绑定邮箱</dt>
                  <dd>{{ security?.email }}</dd>
                </div>
              </dl>
              <div class="security-card__actions">
                <el-button size="small" @click="startChangeEmail">更换邮箱</el-button>
                <el-button size="small" type="danger" plain @click="unbindDialog = true">解绑</el-button>
              </div>
            </template>
            <template v-else>
              <el-alert
                v-if="security && !security.email_available"
                type="warning"
                :closable="false"
                title="服务端未配置邮件渠道，无法发送验证码。"
              />
              <div class="email-form">
                <el-input v-model="emailForm.email" placeholder="you@example.com" autocomplete="email" />
                <div class="email-form__row">
                  <el-input
                    v-model="emailForm.code"
                    maxlength="6"
                    inputmode="numeric"
                    placeholder="6 位验证码"
                  />
                  <el-button
                    :loading="emailSending"
                    :disabled="security ? !security.email_available : false"
                    @click="sendCode"
                  >
                    发送验证码
                  </el-button>
                </div>
                <el-input
                  v-if="emailBound"
                  v-model="emailForm.password"
                  type="password"
                  show-password
                  placeholder="当前密码（换绑需要）"
                  autocomplete="current-password"
                />
                <p class="security-card__hint">每个账号每分钟最多发送 1 条验证码，每天上限 10 条。</p>
                <div class="security-card__actions">
                  <el-button type="primary" :loading="emailBinding" @click="bindEmail">
                    {{ emailBound ? '确认换绑' : '绑定邮箱' }}
                  </el-button>
                  <el-button v-if="emailBound" @click="emailEditing = false">取消</el-button>
                </div>
              </div>
            </template>
          </div>
        </article>
      </section>

      <article class="ax-card settings-block">
        <header class="ax-card__head">
          <h2 class="ax-card__title">外观主题</h2>
        </header>
        <div class="ax-card__body settings-theme">
          <div class="settings-theme__text">
            <strong>界面主题</strong>
            <span>选择深色或浅色外观，设置会保存在本机浏览器。</span>
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
          <h2 class="ax-card__title">登录状态</h2>
        </header>
        <div class="ax-card__body settings-session">
          <div class="settings-session__text">
            <strong>退出登录</strong>
            <span>退出当前账号并清除本地会话，需要重新登录。</span>
          </div>
          <el-button type="danger" plain :icon="SwitchButton" @click="handleLogout">
            退出登录
          </el-button>
        </div>
      </article>
    </template>

    <!-- TOTP 开启 -->
    <el-dialog
      v-model="totpDialog"
      title="开启二次验证"
      width="min(420px, 92vw)"
      append-to-body
      :close-on-click-modal="false"
    >
      <div class="totp-setup">
        <p class="security-card__desc">
          使用身份验证器扫描下方二维码，或手动输入密钥，然后填写生成的 6 位动态验证码。
        </p>
        <div class="totp-setup__qr">
          <img v-if="totpQr" :src="totpQr" alt="TOTP 二维码" width="220" height="220" />
        </div>
        <el-input :model-value="totpSecret" readonly>
          <template #prepend>密钥</template>
        </el-input>
        <el-input
          v-model="totpCode"
          maxlength="6"
          inputmode="numeric"
          placeholder="输入验证器上的 6 位验证码"
        />
      </div>
      <template #footer>
        <el-button @click="totpDialog = false">取消</el-button>
        <el-button type="primary" :loading="totpBusy" @click="confirmEnableTOTP">确认开启</el-button>
      </template>
    </el-dialog>

    <!-- TOTP 关闭 -->
    <el-dialog v-model="disableDialog" title="关闭二次验证" width="min(420px, 92vw)" append-to-body>
      <p class="security-card__desc">请输入动态验证码，或输入当前密码以关闭二次验证。</p>
      <el-input
        v-model="disableCode"
        maxlength="6"
        inputmode="numeric"
        placeholder="6 位动态验证码"
        class="dialog-field"
      />
      <el-input
        v-model="disablePassword"
        type="password"
        show-password
        placeholder="当前密码（可选）"
        class="dialog-field"
      />
      <template #footer>
        <el-button @click="disableDialog = false">取消</el-button>
        <el-button type="danger" :loading="disableBusy" @click="confirmDisableTOTP">确认关闭</el-button>
      </template>
    </el-dialog>

    <!-- 解绑邮箱 -->
    <el-dialog v-model="unbindDialog" title="解绑邮箱" width="min(420px, 92vw)" append-to-body>
      <p class="security-card__desc">解绑后需重新验证才能绑定新邮箱。请输入当前密码。</p>
      <el-input v-model="unbindPassword" type="password" show-password placeholder="当前密码" />
      <template #footer>
        <el-button @click="unbindDialog = false">取消</el-button>
        <el-button type="danger" :loading="unbindBusy" @click="confirmUnbindEmail">确认解绑</el-button>
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
  overflow: hidden;
  max-width: 100%;
  color: var(--ax-text);
  font-size: var(--ax-text-md);
  font-weight: var(--ax-weight-semibold);
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

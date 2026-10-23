<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { Check, Connection, Key, Lock, Refresh, Setting, User } from '@element-plus/icons-vue'

import { ApiError } from '@/api/client'
import { fetchInstallStatus, runInstall, type InstallInput } from '@/api/auth'
import BrandMark from '@/components/BrandMark.vue'
import { useAppStore } from '@/stores/app'
import { confirmPasswordRule, usernameRules } from '@/utils/validate'

const { t } = useI18n()
const router = useRouter()
const app = useAppStore()

const step = ref(0)
const submitting = ref(false)
const formRef = ref<FormInstance>()
const done = ref(false)

const form = reactive<InstallForm>({
  site_name: 'AXmiPic',
  base_url: window.location.origin,
  database_driver: 'sqlite',
  database_dsn: './data/axmipic.db',
  admin_username: '',
  admin_password: '',
  admin_password_confirm: '',
  storage_driver: 'local',
  storage_root: './data/uploads',
  allow_registration: true,
  allow_guest_upload: true,
})

/** 仅用于前端校验「确认密码」，不直接提交给后端。 */
interface InstallForm extends InstallInput {
  admin_password_confirm: string
}

const rules = computed<FormRules>(() => ({
  base_url: [{ required: true, message: t('install.validation.siteUrlRequired'), trigger: 'blur' }],
  database_dsn: [{ required: true, message: t('install.validation.dsnRequired'), trigger: 'blur' }],
  admin_username: usernameRules(t('install.validation.adminUsernameRequired')),
  admin_password: [
    { required: true, message: t('install.validation.adminPasswordRequired'), trigger: 'blur' },
    { min: 8, max: 72, message: t('install.validation.adminPasswordLength'), trigger: 'blur' },
  ],
  admin_password_confirm: [
    confirmPasswordRule(() => form.admin_password, t('install.validation.adminPasswordConfirmRequired')),
  ],
}))

/** 去除仅用于前端校验的字段后，构造提交给后端的输入。 */
function installPayload(): InstallInput {
  return {
    site_name: form.site_name,
    base_url: form.base_url,
    database_driver: form.database_driver,
    database_dsn: form.database_dsn,
    admin_username: form.admin_username,
    admin_password: form.admin_password,
    storage_driver: form.storage_driver,
    storage_root: form.storage_root,
    allow_registration: form.allow_registration,
    allow_guest_upload: form.allow_guest_upload,
  }
}

const steps = computed(() => [
  { title: t('install.steps.database.title'), description: t('install.steps.database.desc'), icon: Connection },
  { title: t('install.steps.admin.title'), description: t('install.steps.admin.desc'), icon: User },
  { title: t('install.steps.site.title'), description: t('install.steps.site.desc'), icon: Setting },
  { title: t('install.steps.done.title'), description: t('install.steps.done.desc'), icon: Check },
])

// ---- PostgreSQL 简化配置：由字段拼装 libpq 连接串 ----
const pg = reactive({
  host: '127.0.0.1',
  port: 5432,
  database: 'axmipic',
  user: 'axmipic',
  password: '',
  sslmode: 'disable',
})

// ---- MySQL 简化配置：由字段拼装 go-sql-driver 连接串 ----
const my = reactive({
  host: '127.0.0.1',
  port: 3306,
  database: 'axmipic',
  user: 'axmipic',
  password: '',
})

const useAdvancedDsn = ref(false)

const sslModes = [
  { label: 'disable', value: 'disable' },
  { label: 'allow', value: 'allow' },
  { label: 'prefer', value: 'prefer' },
  { label: 'require', value: 'require' },
  { label: 'verify-ca', value: 'verify-ca' },
  { label: 'verify-full', value: 'verify-full' },
]

/** 按 libpq 关键字格式对值做必要转义。 */
function quotePgValue(value: string): string {
  if (value === '') {
    return "''"
  }
  if (/[\s'\\]/.test(value)) {
    return `'${value.replace(/\\/g, '\\\\').replace(/'/g, "\\'")}'`
  }
  return value
}

/** 由简化字段生成 libpq 连接串。 */
function buildPgDsn(): string {
  return [
    `host=${quotePgValue(pg.host.trim())}`,
    `port=${pg.port || 5432}`,
    `user=${quotePgValue(pg.user.trim())}`,
    `password=${quotePgValue(pg.password)}`,
    `dbname=${quotePgValue(pg.database.trim())}`,
    `sslmode=${pg.sslmode || 'disable'}`,
  ].join(' ')
}

/** 由简化字段生成 go-sql-driver 连接串（服务端会再补齐 parseTime/charset/loc）。 */
function buildMySQLDsn(): string {
  const user = my.user.trim() || 'root'
  const auth = my.password ? `${user}:${my.password}` : user
  const host = my.host.trim() || '127.0.0.1'
  const port = my.port || 3306
  const database = my.database.trim() || 'axmipic'
  return `${auth}@tcp(${host}:${port})/${database}?charset=utf8mb4&parseTime=true&loc=Local`
}

const isPostgres = computed(() => form.database_driver === 'postgres')
const isMySQL = computed(() => form.database_driver === 'mysql')
/** PostgreSQL / MySQL 都使用连接串，SQLite 使用文件路径。 */
const isNetworkDB = computed(() => isPostgres.value || isMySQL.value)
const pgDsnPreview = computed(() => buildPgDsn())
const mysqlDsnPreview = computed(() => buildMySQLDsn())
const dsnPreview = computed(() => (isMySQL.value ? mysqlDsnPreview.value : pgDsnPreview.value))

watch(
  pg,
  () => {
    if (isPostgres.value && !useAdvancedDsn.value) {
      form.database_dsn = buildPgDsn()
    }
  },
  { deep: true },
)

watch(
  my,
  () => {
    if (isMySQL.value && !useAdvancedDsn.value) {
      form.database_dsn = buildMySQLDsn()
    }
  },
  { deep: true },
)

watch(
  () => form.database_driver,
  (driver) => {
    // 切换类型后旧连接串不再适用，统一回到简化模式。
    useAdvancedDsn.value = false
    if (driver === 'postgres') {
      form.database_dsn = buildPgDsn()
    } else if (driver === 'mysql') {
      form.database_dsn = buildMySQLDsn()
    } else {
      form.database_dsn = './data/axmipic.db'
    }
  },
)

watch(useAdvancedDsn, (advanced) => {
  if (advanced) return
  if (isPostgres.value) {
    form.database_dsn = buildPgDsn()
  } else if (isMySQL.value) {
    form.database_dsn = buildMySQLDsn()
  }
})

function generatePassword(): void {
  const charset = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789!@#$%^&*'
  const bytes = new Uint32Array(18)
  crypto.getRandomValues(bytes)
  const password = Array.from(bytes, (n) => charset[n % charset.length]).join('')
  form.admin_password = password
  form.admin_password_confirm = password
}

async function next(): Promise<void> {
  const instance = formRef.value
  if (step.value === 0) {
    if (instance) {
      const ok = await instance.validateField(['base_url']).catch(() => false)
      if (!ok) return
    }
    if (isPostgres.value && !useAdvancedDsn.value) {
      if (!pg.host.trim() || !pg.database.trim() || !pg.user.trim()) {
        ElMessage.warning(t('install.messages.postgresRequired'))
        return
      }
    } else if (isMySQL.value && !useAdvancedDsn.value) {
      if (!my.host.trim() || !my.database.trim() || !my.user.trim()) {
        ElMessage.warning(t('install.messages.mysqlRequired'))
        return
      }
    } else if (!form.database_dsn.trim()) {
      ElMessage.warning(isNetworkDB.value ? t('install.messages.dsnRequired') : t('install.messages.filePathRequired'))
      return
    }
  }
  if (step.value === 1 && instance) {
    const ok = await instance.validateField(['admin_username', 'admin_password', 'admin_password_confirm']).catch(() => false)
    if (!ok) return
  }
  step.value = Math.min(step.value + 1, steps.value.length - 1)
}

function prev(): void {
  step.value = Math.max(step.value - 1, 0)
}

async function submit(): Promise<void> {
  if (submitting.value) return
  submitting.value = true
  try {
    await runInstall(installPayload())
    app.markInstalled()
    done.value = true
    step.value = steps.value.length - 1
    ElMessage.success(t('install.messages.success'))
  } catch (error) {
    ElMessage.error(error instanceof ApiError ? error.message : t('install.messages.failed'))
  } finally {
    submitting.value = false
  }
}

async function goLogin(): Promise<void> {
  // 进入管理控制台；未登录时路由守卫会转到 /admin/login，并带上回跳地址。
  await router.replace('/admin')
}

onMounted(async () => {
  // 已初始化时初始化入口关闭，再次访问 /install 显示 404。
  try {
    const status = await fetchInstallStatus()
    if (status.installed) {
      app.markInstalled()
      await router.replace({
        name: 'not-found',
        params: { pathMatch: ['install'] },
        replace: true,
      })
    }
  } catch {
    // 无法读取状态时允许继续尝试初始化。
  }
})
</script>

<template>
  <div class="install">
    <section class="install__panel">
      <header class="install__brand">
        <span class="install__mark" aria-hidden="true">
          <BrandMark :size="20" />
        </span>
        <div>
          <h1 class="install__title">{{ t('install.title') }}</h1>
          <p class="install__subtitle">{{ t('install.subtitle') }}</p>
        </div>
      </header>

      <el-steps :active="step" align-center finish-status="success" class="install__steps">
        <el-step v-for="s in steps" :key="s.title" :title="s.title" :description="s.description" />
      </el-steps>

      <el-form
        v-if="!done"
        ref="formRef"
        :model="form"
        :rules="rules"
        label-position="top"
        class="install__form"
        @submit.prevent="submit"
      >
        <template v-if="step === 0">
          <div class="install__section">
            <div class="install__section-head">
              <span class="install__section-icon"><el-icon><Connection /></el-icon></span>
              <div>
                <h3 class="install__section-title">{{ t('install.database.title') }}</h3>
                <p class="install__section-desc">{{ t('install.database.desc') }}</p>
              </div>
            </div>

            <el-form-item :label="t('install.database.siteUrl')" prop="base_url">
              <el-input v-model="form.base_url" placeholder="https://pic.example.com" />
              <div class="install__hint">{{ t('install.database.siteUrlHint') }}</div>
            </el-form-item>

            <el-form-item :label="t('install.database.type')">
              <el-radio-group v-model="form.database_driver">
                <el-radio-button value="sqlite">SQLite</el-radio-button>
                <el-radio-button value="postgres">PostgreSQL</el-radio-button>
                <el-radio-button value="mysql">MySQL</el-radio-button>
              </el-radio-group>
            </el-form-item>

            <template v-if="isNetworkDB">
              <div v-if="!useAdvancedDsn && isPostgres" class="install__grid">
                <el-form-item :label="t('install.database.host')">
                  <el-input v-model="pg.host" placeholder="127.0.0.1" />
                </el-form-item>
                <el-form-item :label="t('install.database.port')">
                  <el-input-number v-model="pg.port" :min="1" :max="65535" controls-position="right" class="install__number" />
                </el-form-item>
                <el-form-item :label="t('install.database.name')">
                  <el-input v-model="pg.database" placeholder="axmipic" />
                </el-form-item>
                <el-form-item :label="t('install.database.username')">
                  <el-input v-model="pg.user" placeholder="axmipic" />
                </el-form-item>
                <el-form-item :label="t('install.database.password')">
                  <el-input v-model="pg.password" type="password" show-password autocomplete="new-password" :placeholder="t('install.database.passwordPlaceholder')" />
                </el-form-item>
                <el-form-item :label="t('install.database.sslMode')">
                  <el-select v-model="pg.sslmode" class="install__select">
                    <el-option v-for="m in sslModes" :key="m.value" :label="m.label" :value="m.value" />
                  </el-select>
                </el-form-item>
              </div>
              <div v-if="!useAdvancedDsn && isMySQL" class="install__grid">
                <el-form-item :label="t('install.database.host')">
                  <el-input v-model="my.host" placeholder="127.0.0.1" />
                </el-form-item>
                <el-form-item :label="t('install.database.port')">
                  <el-input-number v-model="my.port" :min="1" :max="65535" controls-position="right" class="install__number" />
                </el-form-item>
                <el-form-item :label="t('install.database.name')">
                  <el-input v-model="my.database" placeholder="axmipic" />
                </el-form-item>
                <el-form-item :label="t('install.database.username')">
                  <el-input v-model="my.user" placeholder="axmipic" />
                </el-form-item>
                <el-form-item :label="t('install.database.password')">
                  <el-input v-model="my.password" type="password" show-password autocomplete="new-password" :placeholder="t('install.database.passwordPlaceholder')" />
                </el-form-item>
              </div>
              <div v-if="useAdvancedDsn" class="install__grid install__grid--single">
                <el-form-item :label="t('install.database.dsn')" prop="database_dsn">
                  <el-input v-model="form.database_dsn" type="textarea" :rows="3" />
                </el-form-item>
              </div>
              <div v-if="!useAdvancedDsn" class="install__dsn">
                <span class="install__dsn-label">{{ t('install.database.dsn') }}</span>
                <code class="install__dsn-value">{{ dsnPreview }}</code>
              </div>
              <el-checkbox v-model="useAdvancedDsn" class="install__advanced">{{ t('install.database.advanced') }}</el-checkbox>
              <div v-if="isPostgres" class="install__hint">
                {{ t('install.database.postgresHintPrefix') }}<code>host.docker.internal</code>{{ t('install.database.postgresHintSuffix') }}
              </div>
              <div v-else class="install__hint">
                {{ t('install.database.mysqlHintPrefix') }}<code>CHARACTER SET utf8mb4</code>{{ t('install.database.mysqlHintMiddle') }}<code>host.docker.internal</code>{{ t('install.database.mysqlHintSuffix') }}
              </div>
            </template>

            <el-form-item v-else :label="t('install.database.file')" prop="database_dsn">
              <el-input v-model="form.database_dsn" placeholder="./data/axmipic.db" />
              <div class="install__hint">{{ t('install.database.fileHint') }}</div>
            </el-form-item>
          </div>
        </template>

        <template v-else-if="step === 1">
          <div class="install__section">
            <div class="install__section-head">
              <span class="install__section-icon"><el-icon><User /></el-icon></span>
              <div>
                <h3 class="install__section-title">{{ t('install.admin.title') }}</h3>
                <p class="install__section-desc">{{ t('install.admin.desc') }}</p>
              </div>
            </div>

            <el-form-item :label="t('install.admin.username')" prop="admin_username">
              <el-input v-model="form.admin_username" :prefix-icon="User" placeholder="admin" />
              <div class="install__hint">{{ t('install.admin.usernameHint') }}</div>
            </el-form-item>
            <el-form-item :label="t('install.admin.password')" prop="admin_password">
              <el-input
                v-model="form.admin_password"
                type="password"
                show-password
                :prefix-icon="Lock"
                :placeholder="t('install.admin.passwordPlaceholder')"
              >
                <template #append>
                  <el-button :icon="Refresh" @click="generatePassword">{{ t('install.admin.random') }}</el-button>
                </template>
              </el-input>
            </el-form-item>
            <el-form-item :label="t('install.admin.confirmPassword')" prop="admin_password_confirm">
              <el-input
                v-model="form.admin_password_confirm"
                type="password"
                show-password
                :prefix-icon="Lock"
                :placeholder="t('install.admin.confirmPlaceholder')"
              />
            </el-form-item>
            <p class="install__note">
              <el-icon><Key /></el-icon>
              {{ t('install.admin.note') }}
            </p>
          </div>
        </template>

        <template v-else-if="step === 2">
          <div class="install__section">
            <div class="install__section-head">
              <span class="install__section-icon"><el-icon><Setting /></el-icon></span>
              <div>
                <h3 class="install__section-title">{{ t('install.site.title') }}</h3>
                <p class="install__section-desc">{{ t('install.site.desc') }}</p>
              </div>
            </div>

            <el-form-item :label="t('install.site.name')">
              <el-input v-model="form.site_name" placeholder="AXmiPic" />
            </el-form-item>
            <el-form-item :label="t('install.site.storage')">
              <el-radio-group v-model="form.storage_driver">
                <el-radio-button value="local">{{ t('install.site.storageLocal') }}</el-radio-button>
                <el-radio-button value="s3">S3</el-radio-button>
                <el-radio-button value="qiniu">{{ t('install.site.storageQiniu') }}</el-radio-button>
              </el-radio-group>
              <div class="install__hint">{{ t('install.site.storageHint') }}</div>
            </el-form-item>
            <el-form-item v-if="form.storage_driver === 'local'" :label="t('install.site.localRoot')">
              <el-input v-model="form.storage_root" placeholder="./data/uploads" />
            </el-form-item>
            <el-form-item :label="t('install.site.permissions')">
              <div class="install__switch">
                <div class="install__switch-row">
                  <span>{{ t('install.site.allowRegistration') }}</span>
                  <el-switch v-model="form.allow_registration" />
                </div>
                <div class="install__switch-row">
                  <span>{{ t('install.site.allowGuestUpload') }}</span>
                  <el-switch v-model="form.allow_guest_upload" />
                </div>
              </div>
            </el-form-item>
          </div>
        </template>
      </el-form>

      <div v-else class="install__done">
        <el-icon class="install__done-icon" :size="48"><Check /></el-icon>
        <h2>{{ t('install.done.title') }}</h2>
        <p>{{ t('install.done.desc') }}</p>
        <el-button type="primary" @click="goLogin">{{ t('install.done.goLogin') }}</el-button>
      </div>

      <footer v-if="!done" class="install__footer">
        <el-button :disabled="step === 0" @click="prev">{{ t('install.footer.previous') }}</el-button>
        <el-button v-if="step < steps.length - 1" type="primary" @click="next">{{ t('install.footer.next') }}</el-button>
        <el-button v-else type="primary" :loading="submitting" @click="submit">{{ t('install.footer.submit') }}</el-button>
      </footer>
    </section>
  </div>
</template>

<style scoped>
.install {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100dvh;
  padding: var(--ax-space-6);
  background-color: var(--ax-canvas);
  background-image: var(--ax-atmosphere);
}

.install__panel {
  width: min(640px, 100%);
  padding: var(--ax-space-6) var(--ax-space-6) var(--ax-space-5);
  background: var(--ax-panel);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-lg);
  box-shadow: var(--ax-shadow-lg);
}

.install__brand {
  display: flex;
  align-items: center;
  gap: var(--ax-space-3);
  margin-bottom: var(--ax-space-5);
}

.install__mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 42px;
  height: 42px;
  color: var(--ax-accent-bright);
  background: var(--ax-accent-soft);
  border: 1px solid var(--ax-accent-ring);
  border-radius: var(--ax-radius-md);
}

.install__title {
  margin: 0;
  color: var(--ax-text);
  font-size: var(--ax-text-xl);
}

.install__subtitle {
  margin: 2px 0 0;
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
}

.install__steps {
  margin-bottom: var(--ax-space-5);
}

.install__form {
  min-height: 260px;
}

/* 步骤内容区块 */
.install__section {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-3);
}

.install__section-head {
  display: flex;
  align-items: center;
  gap: var(--ax-space-3);
  padding-bottom: var(--ax-space-3);
  border-bottom: 1px solid var(--ax-border-subtle);
}

.install__section-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  flex: none;
  color: var(--ax-accent-bright);
  background: var(--ax-accent-soft);
  border-radius: var(--ax-radius-md);
  font-size: 18px;
}

.install__section-title {
  margin: 0;
  color: var(--ax-text);
  font-size: var(--ax-text-base);
}

.install__section-desc {
  margin: 2px 0 0;
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

/* PostgreSQL 简化字段的两列网格 */
.install__grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(200px, 100%), 1fr));
  gap: 0 var(--ax-space-4);
}

.install__grid--single {
  grid-template-columns: 1fr;
}

.install__number,
.install__select {
  width: 100%;
}

.install__dsn {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: var(--ax-space-2) var(--ax-space-3);
  background: var(--ax-tint-weak);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-sm);
}

.install__dsn-label {
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.install__dsn-value {
  color: var(--ax-text-2);
  font-family: var(--ax-font-mono);
  font-size: var(--ax-text-xs);
  line-height: 1.5;
  overflow-wrap: anywhere;
}

.install__advanced {
  align-self: flex-start;
}

.install__hint {
  margin-top: 4px;
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
  line-height: 1.5;
}

.install__hint code {
  padding: 1px 4px;
  border-radius: var(--ax-radius-xs);
  background: var(--ax-tint);
  font-family: var(--ax-font-mono);
}

.install__note {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0;
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
}

.install__switch {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-3);
  width: 100%;
}

.install__switch-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  color: var(--ax-text-2);
  font-size: var(--ax-text-sm);
}

.install__footer {
  display: flex;
  justify-content: flex-end;
  gap: var(--ax-space-2);
  margin-top: var(--ax-space-4);
  padding-top: var(--ax-space-4);
  border-top: 1px solid var(--ax-border-subtle);
}

.install__done {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--ax-space-3);
  padding: var(--ax-space-6) 0;
  text-align: center;
}

.install__done-icon {
  color: var(--ax-success);
}

.install__done h2 {
  margin: 0;
  color: var(--ax-text);
}

.install__done p {
  margin: 0;
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
}
</style>

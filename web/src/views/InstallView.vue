<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { Check, Connection, Key, Lock, Refresh, Setting, User } from '@element-plus/icons-vue'

import { ApiError } from '@/api/client'
import { fetchInstallStatus, runInstall, type InstallInput } from '@/api/auth'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { confirmPasswordRule, usernameRules } from '@/utils/validate'

const router = useRouter()
const app = useAppStore()
const auth = useAuthStore()

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
  install_token: '',
})

/** 仅用于前端校验「确认密码」与安装令牌，不直接提交给后端。 */
interface InstallForm extends InstallInput {
  admin_password_confirm: string
  install_token: string
}

const rules: FormRules = {
  base_url: [{ required: true, message: '请输入站点地址', trigger: 'blur' }],
  install_token: [{ required: true, message: '请输入安装令牌', trigger: 'blur' }],
  database_dsn: [{ required: true, message: '请输入数据库连接串', trigger: 'blur' }],
  admin_username: usernameRules('请输入管理员用户名'),
  admin_password: [
    { required: true, message: '请输入管理员密码', trigger: 'blur' },
    { min: 8, max: 72, message: '密码长度为 8 到 72 个字符', trigger: 'blur' },
  ],
  admin_password_confirm: [confirmPasswordRule(() => form.admin_password, '请再次输入管理员密码')],
}

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

const steps = [
  { title: '数据库', description: '连接配置', icon: Connection },
  { title: '管理员', description: '初始账户', icon: User },
  { title: '站点', description: '存储与权限', icon: Setting },
  { title: '完成', description: '开始使用', icon: Check },
]

// ---- PostgreSQL 简化配置：由字段拼装 libpq 连接串 ----
const pg = reactive({
  host: '127.0.0.1',
  port: 5432,
  database: 'axmipic',
  user: 'axmipic',
  password: '',
  sslmode: 'disable',
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

const isPostgres = computed(() => form.database_driver === 'postgres')
const pgDsnPreview = computed(() => buildPgDsn())

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
  () => form.database_driver,
  (driver) => {
    if (driver === 'postgres') {
      if (!useAdvancedDsn.value) {
        form.database_dsn = buildPgDsn()
      }
    } else {
      form.database_dsn = './data/axmipic.db'
    }
  },
)

watch(useAdvancedDsn, (advanced) => {
  if (!advanced && isPostgres.value) {
    form.database_dsn = buildPgDsn()
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
      const ok = await instance.validateField(['base_url', 'install_token']).catch(() => false)
      if (!ok) return
    }
    if (isPostgres.value && !useAdvancedDsn.value) {
      if (!pg.host.trim() || !pg.database.trim() || !pg.user.trim()) {
        ElMessage.warning('请填写 PostgreSQL 主机、数据库名与用户名')
        return
      }
    } else if (!form.database_dsn.trim()) {
      ElMessage.warning(isPostgres.value ? '请填写数据库连接串' : '请填写数据库文件路径')
      return
    }
  }
  if (step.value === 1 && instance) {
    const ok = await instance.validateField(['admin_username', 'admin_password', 'admin_password_confirm']).catch(() => false)
    if (!ok) return
  }
  step.value = Math.min(step.value + 1, steps.length - 1)
}

function prev(): void {
  step.value = Math.max(step.value - 1, 0)
}

async function submit(): Promise<void> {
  if (submitting.value) return
  submitting.value = true
  try {
    await runInstall(installPayload(), form.install_token.trim())
    app.markInstalled()
    done.value = true
    step.value = steps.length - 1
    ElMessage.success('安装完成')
  } catch (error) {
    ElMessage.error(error instanceof ApiError ? error.message : '安装失败，请检查配置')
  } finally {
    submitting.value = false
  }
}

async function goLogin(): Promise<void> {
  await router.replace('/login')
}

onMounted(async () => {
  // 已安装则直接离开安装向导。
  try {
    const status = await fetchInstallStatus()
    if (status.installed) {
      app.markInstalled()
      await router.replace(auth.isAuthenticated ? '/admin' : '/login')
    }
  } catch {
    // 无法读取状态时允许继续尝试安装。
  }
})
</script>

<template>
  <div class="install">
    <section class="install__panel">
      <header class="install__brand">
        <span class="install__mark" aria-hidden="true">
          <svg viewBox="0 0 32 32" width="20" height="20">
            <path d="M16 6.5 23.8 25.5h-4.05l-1.55-4.05h-4.4L12.25 25.5H8.2Z" fill="currentColor" />
          </svg>
        </span>
        <div>
          <h1 class="install__title">AXmiPic 安装向导</h1>
          <p class="install__subtitle">只需几步即可完成初始化</p>
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
                <h3 class="install__section-title">数据库连接</h3>
                <p class="install__section-desc">选择数据库类型并填写连接信息</p>
              </div>
            </div>

            <el-form-item label="安装令牌" prop="install_token">
              <el-input v-model="form.install_token" placeholder="启动日志中的 install_token" autocomplete="off" />
              <div class="install__hint">首次部署时服务启动日志会输出 install_token，请在此填写以完成初始化。</div>
            </el-form-item>

            <el-form-item label="站点地址" prop="base_url">
              <el-input v-model="form.base_url" placeholder="https://pic.example.com" />
              <div class="install__hint">用于生成图片与分享链接，请填写对外可访问的地址。</div>
            </el-form-item>

            <el-form-item label="数据库类型">
              <el-radio-group v-model="form.database_driver">
                <el-radio-button value="sqlite">SQLite</el-radio-button>
                <el-radio-button value="postgres">PostgreSQL</el-radio-button>
              </el-radio-group>
            </el-form-item>

            <template v-if="isPostgres">
              <div v-if="!useAdvancedDsn" class="install__grid">
                <el-form-item label="主机">
                  <el-input v-model="pg.host" placeholder="127.0.0.1" />
                </el-form-item>
                <el-form-item label="端口">
                  <el-input-number v-model="pg.port" :min="1" :max="65535" controls-position="right" class="install__number" />
                </el-form-item>
                <el-form-item label="数据库名">
                  <el-input v-model="pg.database" placeholder="axmipic" />
                </el-form-item>
                <el-form-item label="用户名">
                  <el-input v-model="pg.user" placeholder="axmipic" />
                </el-form-item>
                <el-form-item label="密码">
                  <el-input v-model="pg.password" type="password" show-password autocomplete="new-password" placeholder="数据库密码" />
                </el-form-item>
                <el-form-item label="SSL 模式">
                  <el-select v-model="pg.sslmode" class="install__select">
                    <el-option v-for="m in sslModes" :key="m.value" :label="m.label" :value="m.value" />
                  </el-select>
                </el-form-item>
              </div>
              <div v-else class="install__grid install__grid--single">
                <el-form-item label="连接串" prop="database_dsn">
                  <el-input v-model="form.database_dsn" type="textarea" :rows="3" />
                </el-form-item>
              </div>
              <div v-if="!useAdvancedDsn" class="install__dsn">
                <span class="install__dsn-label">连接串</span>
                <code class="install__dsn-value">{{ pgDsnPreview }}</code>
              </div>
              <el-checkbox v-model="useAdvancedDsn" class="install__advanced">高级：直接填写连接串</el-checkbox>
              <div class="install__hint">
                需先在数据库中创建对应的库与用户；容器部署时主机请填写宿主地址（如 <code>host.docker.internal</code> 或宿主机 IP）。
              </div>
            </template>

            <el-form-item v-else label="数据库文件" prop="database_dsn">
              <el-input v-model="form.database_dsn" placeholder="./data/axmipic.db" />
              <div class="install__hint">SQLite 数据库文件路径，目录需可写。</div>
            </el-form-item>
          </div>
        </template>

        <template v-else-if="step === 1">
          <div class="install__section">
            <div class="install__section-head">
              <span class="install__section-icon"><el-icon><User /></el-icon></span>
              <div>
                <h3 class="install__section-title">管理员账户</h3>
                <p class="install__section-desc">用于登录管理控制台</p>
              </div>
            </div>

            <el-form-item label="管理员用户名" prop="admin_username">
              <el-input v-model="form.admin_username" :prefix-icon="User" placeholder="admin" />
              <div class="install__hint">可包含字母、数字与 "."、"_"、"-"、"+"、"@"（3-64 位），支持邮箱形式。</div>
            </el-form-item>
            <el-form-item label="管理员密码" prop="admin_password">
              <el-input
                v-model="form.admin_password"
                type="password"
                show-password
                :prefix-icon="Lock"
                placeholder="至少 8 位"
              >
                <template #append>
                  <el-button :icon="Refresh" @click="generatePassword">随机</el-button>
                </template>
              </el-input>
            </el-form-item>
            <el-form-item label="确认密码" prop="admin_password_confirm">
              <el-input
                v-model="form.admin_password_confirm"
                type="password"
                show-password
                :prefix-icon="Lock"
                placeholder="再次输入管理员密码"
              />
            </el-form-item>
            <p class="install__note">
              <el-icon><Key /></el-icon>
              该账号仅用于管理后台，与普通用户分表管理；请妥善保存密码。
            </p>
          </div>
        </template>

        <template v-else-if="step === 2">
          <div class="install__section">
            <div class="install__section-head">
              <span class="install__section-icon"><el-icon><Setting /></el-icon></span>
              <div>
                <h3 class="install__section-title">站点与存储</h3>
                <p class="install__section-desc">站点名称、存储方式与权限</p>
              </div>
            </div>

            <el-form-item label="站点名称">
              <el-input v-model="form.site_name" placeholder="AXmiPic" />
            </el-form-item>
            <el-form-item label="存储方式">
              <el-radio-group v-model="form.storage_driver">
                <el-radio-button value="local">本地</el-radio-button>
                <el-radio-button value="s3">S3</el-radio-button>
                <el-radio-button value="qiniu">七牛</el-radio-button>
              </el-radio-group>
              <div class="install__hint">S3 / 七牛可在安装后于「存储配置」中补充密钥。</div>
            </el-form-item>
            <el-form-item v-if="form.storage_driver === 'local'" label="本地存储目录">
              <el-input v-model="form.storage_root" placeholder="./data/uploads" />
            </el-form-item>
            <el-form-item label="权限选项">
              <div class="install__switch">
                <div class="install__switch-row">
                  <span>允许开放注册</span>
                  <el-switch v-model="form.allow_registration" />
                </div>
                <div class="install__switch-row">
                  <span>允许未登录访客上传（Guest 低权角色）</span>
                  <el-switch v-model="form.allow_guest_upload" />
                </div>
              </div>
            </el-form-item>
          </div>
        </template>
      </el-form>

      <div v-else class="install__done">
        <el-icon class="install__done-icon" :size="48"><Check /></el-icon>
        <h2>安装完成</h2>
        <p>已创建管理员账户与 Guest 访客角色，现在可以登录管理控制台。</p>
        <el-button type="primary" @click="goLogin">前往登录</el-button>
      </div>

      <footer v-if="!done" class="install__footer">
        <el-button :disabled="step === 0" @click="prev">上一步</el-button>
        <el-button v-if="step < steps.length - 1" type="primary" @click="next">下一步</el-button>
        <el-button v-else type="primary" :loading="submitting" @click="submit">开始安装</el-button>
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

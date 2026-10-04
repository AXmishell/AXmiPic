<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { Check, Connection, Lock, Setting, User } from '@element-plus/icons-vue'

import { ApiError } from '@/api/client'
import { fetchInstallStatus, runInstall, type InstallInput } from '@/api/auth'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const app = useAppStore()
const auth = useAuthStore()

const step = ref(0)
const submitting = ref(false)
const formRef = ref<FormInstance>()
const done = ref(false)

const form = reactive<InstallInput>({
  site_name: 'AXmiPic',
  base_url: window.location.origin,
  database_driver: 'sqlite',
  database_dsn: './data/axmipic.db',
  admin_username: '',
  admin_password: '',
  storage_driver: 'local',
  storage_root: './data/uploads',
  allow_registration: true,
  allow_guest_upload: true,
})

const rules: FormRules = {
  base_url: [{ required: true, message: '请输入站点地址', trigger: 'blur' }],
  database_dsn: [{ required: true, message: '请输入数据库连接串', trigger: 'blur' }],
  admin_username: [
    { required: true, message: '请输入管理员用户名', trigger: 'blur' },
    { min: 3, max: 64, message: '用户名长度为 3 到 64 个字符', trigger: 'blur' },
  ],
  admin_password: [
    { required: true, message: '请输入管理员密码', trigger: 'blur' },
    { min: 8, max: 72, message: '密码长度为 8 到 72 个字符', trigger: 'blur' },
  ],
}

const steps = [
  { title: '数据库', icon: Connection },
  { title: '管理员', icon: User },
  { title: '站点', icon: Setting },
  { title: '完成', icon: Check },
]

const driverHint = computed(() =>
  form.database_driver === 'postgres'
    ? 'libpq 连接串或 URL，例如 host=127.0.0.1 port=5432 user=axmipic password=xxx dbname=axmipic sslmode=disable'
    : 'SQLite 数据库文件路径，例如 ./data/axmipic.db',
)

async function next(): Promise<void> {
  if (step.value === 0) {
    const instance = formRef.value
    if (instance) {
      const ok = await instance.validateField(['base_url', 'database_dsn']).catch(() => false)
      if (!ok) return
    }
  }
  if (step.value === 1) {
    const instance = formRef.value
    if (instance) {
      const ok = await instance.validateField(['admin_username', 'admin_password']).catch(() => false)
      if (!ok) return
    }
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
    await runInstall({ ...form })
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
        <el-step v-for="s in steps" :key="s.title" :title="s.title" />
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
          <el-form-item label="站点地址" prop="base_url">
            <el-input v-model="form.base_url" placeholder="https://pic.example.com" />
          </el-form-item>
          <el-form-item label="数据库类型">
            <el-radio-group v-model="form.database_driver">
              <el-radio-button value="sqlite">SQLite</el-radio-button>
              <el-radio-button value="postgres">PostgreSQL</el-radio-button>
            </el-radio-group>
          </el-form-item>
          <el-form-item label="数据库连接" prop="database_dsn">
            <el-input v-model="form.database_dsn" type="textarea" :rows="3" />
            <div class="install__hint">{{ driverHint }}</div>
          </el-form-item>
        </template>

        <template v-else-if="step === 1">
          <el-form-item label="管理员用户名" prop="admin_username">
            <el-input v-model="form.admin_username" :prefix-icon="User" placeholder="admin" />
          </el-form-item>
          <el-form-item label="管理员密码" prop="admin_password">
            <el-input v-model="form.admin_password" type="password" show-password :prefix-icon="Lock" placeholder="至少 8 位" />
          </el-form-item>
          <p class="install__note">该账号仅用于管理后台，与普通用户分表管理。</p>
        </template>

        <template v-else-if="step === 2">
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
  background: var(--ax-bg);
}

.install__panel {
  width: min(560px, 100%);
  padding: var(--ax-space-7) var(--ax-space-6);
  background: var(--ax-panel);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-lg);
  box-shadow: var(--ax-shadow-md);
}

.install__brand {
  display: flex;
  align-items: center;
  gap: var(--ax-space-3);
  margin-bottom: var(--ax-space-6);
}

.install__mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
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
  margin-bottom: var(--ax-space-6);
}

.install__form {
  min-height: 240px;
}

.install__hint {
  margin-top: 4px;
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
  line-height: 1.5;
}

.install__note {
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

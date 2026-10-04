<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { Delete, EditPen, FolderOpened, Plus, Refresh, Star } from '@element-plus/icons-vue'

import {
  activateStorage,
  createStorage,
  deleteStorage,
  listStorage,
  updateStorage,
} from '@/api/admin'
import { toApiError } from '@/api/client'
import type { StorageBackend, StorageBackendInput, StorageDriver } from '@/api/types'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'
import PageHeader from '@/components/PageHeader.vue'
import { formatDateTime, formatNumber } from '@/utils/format'

const backends = ref<StorageBackend[]>([])
const loading = ref(false)
const errorMessage = ref('')
const pendingId = ref<string | null>(null)

const dialogOpen = ref(false)
const editingId = ref<string | null>(null)
const submitting = ref(false)
const formRef = ref<FormInstance>()

interface StorageForm {
  name: string
  driver: StorageDriver
  activate: boolean
  // local
  root: string
  // s3 / qiniu 公共
  bucket: string
  // s3
  endpoint: string
  region: string
  secure: boolean
  use_path_style: boolean
  public_base_url: string
  presign_expiry_sec: number
  access_key_id: string
  secret_access_key: string
  // qiniu
  domain: string
  upload_host: string
  zone: string
  private: boolean
  use_https: boolean
  qiniu_access_key: string
  qiniu_secret_key: string
}

function emptyForm(): StorageForm {
  return {
    name: '',
    driver: 'local',
    activate: false,
    root: '',
    bucket: '',
    endpoint: '',
    region: 'us-east-1',
    secure: true,
    use_path_style: false,
    public_base_url: '',
    presign_expiry_sec: 900,
    access_key_id: '',
    secret_access_key: '',
    domain: '',
    upload_host: '',
    zone: '',
    private: false,
    use_https: true,
    qiniu_access_key: '',
    qiniu_secret_key: '',
  }
}

const form = reactive<StorageForm>(emptyForm())

const isEdit = computed(() => editingId.value !== null)
const driverLabel: Record<StorageDriver, string> = {
  local: '本地存储',
  s3: '对象存储 (S3 兼容)',
  qiniu: '七牛云 Kodo',
}

const rules: FormRules = {
  name: [{ required: true, message: '请输入配置名称', trigger: 'blur' }],
}

async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    backends.value = (await listStorage()) ?? []
  } catch (error) {
    errorMessage.value = toApiError(error).message
  } finally {
    loading.value = false
  }
}

function openCreate(): void {
  editingId.value = null
  Object.assign(form, emptyForm())
  dialogOpen.value = true
}

function openEdit(item: StorageBackend): void {
  editingId.value = item.id
  Object.assign(form, emptyForm())
  form.name = item.name
  form.driver = item.driver
  const s = item.settings ?? {}
  if (item.driver === 'local') {
    form.root = String(s.root ?? '')
  } else if (item.driver === 's3') {
    form.endpoint = String(s.endpoint ?? '')
    form.region = String(s.region ?? 'us-east-1')
    form.bucket = String(s.bucket ?? '')
    form.secure = Boolean(s.secure)
    form.use_path_style = Boolean(s.use_path_style)
    form.public_base_url = String(s.public_base_url ?? '')
    form.presign_expiry_sec = Number(s.presign_expiry_sec ?? 900)
  } else {
    form.bucket = String(s.bucket ?? '')
    form.domain = String(s.domain ?? '')
    form.upload_host = String(s.upload_host ?? '')
    form.zone = String(s.zone ?? '')
    form.private = Boolean(s.private)
    form.use_https = Boolean(s.use_https)
    form.presign_expiry_sec = Number(s.presign_expiry_sec ?? 3600)
  }
  dialogOpen.value = true
}

function buildPayload(): StorageBackendInput {
  const settings: Record<string, unknown> = {}
  const secrets: Record<string, string> = {}
  if (form.driver === 'local') {
    settings.root = form.root.trim()
  } else if (form.driver === 's3') {
    settings.endpoint = form.endpoint.trim()
    settings.region = form.region.trim()
    settings.bucket = form.bucket.trim()
    settings.secure = form.secure
    settings.use_path_style = form.use_path_style
    settings.public_base_url = form.public_base_url.trim()
    settings.presign_expiry_sec = Number(form.presign_expiry_sec) || 900
    secrets.access_key_id = form.access_key_id.trim()
    secrets.secret_access_key = form.secret_access_key.trim()
  } else {
    settings.bucket = form.bucket.trim()
    settings.domain = form.domain.trim()
    settings.upload_host = form.upload_host.trim()
    settings.zone = form.zone.trim()
    settings.private = form.private
    settings.use_https = form.use_https
    settings.presign_expiry_sec = Number(form.presign_expiry_sec) || 3600
    secrets.access_key = form.qiniu_access_key.trim()
    secrets.secret_key = form.qiniu_secret_key.trim()
  }
  return {
    name: form.name.trim(),
    driver: form.driver,
    settings,
    secrets,
    activate: form.activate,
  }
}

async function submit(): Promise<void> {
  const instance = formRef.value
  if (!instance || submitting.value) return
  const valid = await instance.validate().catch(() => false)
  if (!valid) return

  submitting.value = true
  try {
    if (isEdit.value && editingId.value) {
      await updateStorage(editingId.value, buildPayload())
      ElMessage.success('存储配置已更新')
    } else {
      await createStorage(buildPayload())
      ElMessage.success('存储配置已创建')
    }
    dialogOpen.value = false
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    submitting.value = false
  }
}

async function activate(item: StorageBackend): Promise<void> {
  try {
    await ElMessageBox.confirm(
      `切换后新上传的图片将存入「${item.name}」，已有图片仍从原存储读取。`,
      '切换默认存储',
      { confirmButtonText: '切换', cancelButtonText: '取消', type: 'warning' },
    )
  } catch {
    return
  }
  pendingId.value = item.id
  try {
    await activateStorage(item.id)
    ElMessage.success('已切换默认存储')
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    pendingId.value = null
  }
}

async function remove(item: StorageBackend): Promise<void> {
  try {
    await ElMessageBox.confirm(
      `确定删除存储配置「${item.name}」吗？仅当其上没有图片时可删除。`,
      '删除存储配置',
      {
        confirmButtonText: '删除',
        cancelButtonText: '取消',
        type: 'warning',
        confirmButtonClass: 'el-button--danger',
      },
    )
  } catch {
    return
  }
  pendingId.value = item.id
  try {
    await deleteStorage(item.id)
    ElMessage.success('存储配置已删除')
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    pendingId.value = null
  }
}

onMounted(load)
</script>

<template>
  <div class="ax-page">
    <PageHeader
      title="存储配置"
      description="配置并切换图片存储后端：本地文件系统、S3 兼容对象存储或七牛云。切换即时生效，无需重启。"
    >
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
        <el-button type="primary" :icon="Plus" @click="openCreate">新建存储</el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="load" />

    <div v-else-if="loading && backends.length === 0" class="ax-card" aria-busy="true">
      <div class="ax-card__body"><el-skeleton :rows="6" animated /></div>
    </div>

    <EmptyState
      v-else-if="backends.length === 0"
      title="还没有存储配置"
      description="创建本地或对象存储配置后，即可在后台随时切换默认存储。"
    >
      <el-button type="primary" :icon="Plus" @click="openCreate">新建存储</el-button>
    </EmptyState>

    <div v-else class="ax-card table-card">
      <div class="ax-table-scroll">
        <el-table :data="backends" style="width: 100%">
          <el-table-column label="名称" min-width="180">
            <template #default="{ row }">
              <span class="storage-name">
                <el-icon :size="14"><FolderOpened /></el-icon>
                {{ row.name }}
                <el-tag v-if="row.is_current" size="small" type="success" effect="plain">
                  当前默认
                </el-tag>
              </span>
            </template>
          </el-table-column>
          <el-table-column label="类型" width="180">
            <template #default="{ row }">{{ driverLabel[row.driver as StorageDriver] }}</template>
          </el-table-column>
          <el-table-column label="图片数" width="120">
            <template #default="{ row }">{{ formatNumber(row.image_count) }}</template>
          </el-table-column>
          <el-table-column label="创建时间" min-width="170">
            <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column label="操作" width="290" align="right">
            <template #default="{ row }">
              <div class="ax-row-actions">
                <el-button
                  link
                  type="primary"
                  :icon="Star"
                  :disabled="row.is_current"
                  :loading="pendingId === row.id"
                  @click="activate(row)"
                >
                  设为默认
                </el-button>
                <el-button link type="primary" :icon="EditPen" @click="openEdit(row)">编辑</el-button>
                <el-button
                  link
                  type="danger"
                  :icon="Delete"
                  :disabled="row.is_current"
                  :loading="pendingId === row.id"
                  @click="remove(row)"
                >
                  删除
                </el-button>
              </div>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </div>

    <el-dialog
      v-model="dialogOpen"
      :title="isEdit ? '编辑存储配置' : '新建存储配置'"
      width="min(560px, 94vw)"
      append-to-body
      :close-on-click-modal="false"
      @closed="formRef?.clearValidate()"
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" placeholder="例如：本地、阿里云 OSS" maxlength="60" />
        </el-form-item>

        <el-form-item label="存储类型">
          <el-radio-group v-model="form.driver" :disabled="isEdit">
            <el-radio-button value="local">本地</el-radio-button>
            <el-radio-button value="s3">对象存储 (S3)</el-radio-button>
            <el-radio-button value="qiniu">七牛云</el-radio-button>
          </el-radio-group>
        </el-form-item>

        <!-- 本地 -->
        <template v-if="form.driver === 'local'">
          <el-form-item label="根目录">
            <el-input v-model="form.root" placeholder="留空则使用配置文件中的默认目录" />
          </el-form-item>
          <p class="storage-hint">
            相对路径相对于服务启动目录；留空时回退到 config.yaml 的 storage.local.root。
          </p>
        </template>

        <!-- S3 -->
        <template v-else-if="form.driver === 's3'">
          <el-form-item label="Endpoint">
            <el-input v-model="form.endpoint" placeholder="https://s3.amazonaws.com" />
          </el-form-item>
          <el-form-item label="Region">
            <el-input v-model="form.region" placeholder="us-east-1" />
          </el-form-item>
          <el-form-item label="Bucket">
            <el-input v-model="form.bucket" placeholder="axmipic" />
          </el-form-item>
          <el-form-item label="Access Key ID">
            <el-input
              v-model="form.access_key_id"
              :placeholder="isEdit ? '留空表示保持不变' : ''"
            />
          </el-form-item>
          <el-form-item label="Secret Access Key">
            <el-input
              v-model="form.secret_access_key"
              type="password"
              show-password
              :placeholder="isEdit ? '留空表示保持不变' : ''"
            />
          </el-form-item>
          <el-form-item label="公开访问 Base URL（可选）">
            <el-input v-model="form.public_base_url" placeholder="https://cdn.example.com" />
          </el-form-item>
          <div class="storage-switches">
            <el-checkbox v-model="form.secure">使用 HTTPS</el-checkbox>
            <el-checkbox v-model="form.use_path_style">Path Style</el-checkbox>
          </div>
        </template>

        <!-- 七牛 -->
        <template v-else>
          <el-form-item label="Bucket">
            <el-input v-model="form.bucket" placeholder="axmipic" />
          </el-form-item>
          <el-form-item label="访问域名">
            <el-input v-model="form.domain" placeholder="https://cdn.example.com" />
          </el-form-item>
          <el-form-item label="Access Key">
            <el-input
              v-model="form.qiniu_access_key"
              :placeholder="isEdit ? '留空表示保持不变' : ''"
            />
          </el-form-item>
          <el-form-item label="Secret Key">
            <el-input
              v-model="form.qiniu_secret_key"
              type="password"
              show-password
              :placeholder="isEdit ? '留空表示保持不变' : ''"
            />
          </el-form-item>
          <el-form-item label="上传域名（可选）">
            <el-input v-model="form.upload_host" placeholder="https://upload.qiniup.com" />
          </el-form-item>
          <div class="storage-switches">
            <el-checkbox v-model="form.use_https">使用 HTTPS</el-checkbox>
            <el-checkbox v-model="form.private">私有空间</el-checkbox>
          </div>
        </template>

        <el-form-item v-if="!isEdit" label="创建后立即设为默认">
          <el-switch v-model="form.activate" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogOpen = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submit">
          {{ isEdit ? '保存' : '创建' }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.table-card {
  overflow: hidden;
}

.storage-name {
  display: inline-flex;
  align-items: center;
  gap: var(--ax-space-2);
  color: var(--ax-text);
  font-weight: var(--ax-weight-medium);
}

.storage-hint {
  margin: -4px 0 0;
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.storage-switches {
  display: flex;
  gap: var(--ax-space-5);
  margin-bottom: var(--ax-space-4);
}
</style>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { Delete, EditPen, FolderOpened, Plus, QuestionFilled, Refresh, Star } from '@element-plus/icons-vue'

import {
  activateStorage,
  activateStoragePool,
  createStorage,
  createStoragePool,
  deleteStorage,
  deleteStoragePool,
  listStorage,
  listStoragePools,
  updateStorage,
  updateStoragePool,
} from '@/api/admin'
import { toApiError } from '@/api/client'
import type {
  StorageBackend,
  StorageBackendInput,
  StorageDriver,
  StoragePool,
  StoragePoolInput,
  StoragePoolPolicy,
} from '@/api/types'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'
import PageHeader from '@/components/PageHeader.vue'
import TableSkeleton from '@/components/TableSkeleton.vue'
import { formatBytes, formatDateTime, formatNumber } from '@/utils/format'

const backends = ref<StorageBackend[]>([])
const pools = ref<StoragePool[]>([])
const loading = ref(false)
const errorMessage = ref('')
const pendingId = ref<string | null>(null)

const storageDocsUrl = 'https://axmipic.gpcn.cc/guide/configuration#存储池'
const currentBackend = computed(() => backends.value.find((backend) => backend.is_current) ?? null)
const canCreatePool = computed(() => backends.value.length >= 2)

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
  // oss
  oss_access_key_id: string
  oss_access_key_secret: string
  // cos
  cos_secret_id: string
  cos_secret_key: string
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
    oss_access_key_id: '',
    oss_access_key_secret: '',
    cos_secret_id: '',
    cos_secret_key: '',
  }
}

const form = reactive<StorageForm>(emptyForm())
const formSnapshot = ref('')

/** 用于对话框脏检查：打开时记录快照，关闭前比较。 */
function formState(): string {
  return JSON.stringify(form)
}

async function closeFormDialog(done: () => void): Promise<void> {
  if (submitting.value) return
  if (formState() === formSnapshot.value) {
    done()
    return
  }
  try {
    await ElMessageBox.confirm('关闭后未保存的修改将丢失。', '放弃修改？', {
      confirmButtonText: '放弃修改',
      cancelButtonText: '继续编辑',
      type: 'warning',
      confirmButtonClass: 'el-button--danger',
    })
    done()
  } catch {
    // 用户选择继续编辑。
  }
}

const isEdit = computed(() => editingId.value !== null)
const driverLabel: Record<StorageDriver, string> = {
  local: '本地存储',
  s3: '对象存储 (S3 兼容)',
  qiniu: '七牛云 Kodo',
  oss: '阿里云 OSS',
  cos: '腾讯云 COS',
}

const rules: FormRules = {
  name: [{ required: true, message: '请输入配置名称', trigger: 'blur' }],
}

async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    const [backendList, poolList] = await Promise.all([
      listStorage(),
      listStoragePools().catch(() => []),
    ])
    backends.value = backendList ?? []
    pools.value = poolList ?? []
  } catch (error) {
    errorMessage.value = toApiError(error).message
  } finally {
    loading.value = false
  }
}

function openCreate(): void {
  editingId.value = null
  Object.assign(form, emptyForm())
  formSnapshot.value = formState()
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
  } else if (item.driver === 'oss') {
    form.endpoint = String(s.endpoint ?? '')
    form.region = String(s.region ?? '')
    form.bucket = String(s.bucket ?? '')
    form.domain = String(s.domain ?? '')
    form.private = Boolean(s.private)
    form.use_https = s.use_https === undefined ? true : Boolean(s.use_https)
  } else if (item.driver === 'cos') {
    form.region = String(s.region ?? '')
    form.bucket = String(s.bucket ?? '')
    form.domain = String(s.domain ?? '')
    form.private = Boolean(s.private)
    form.use_https = s.use_https === undefined ? true : Boolean(s.use_https)
  } else {
    form.bucket = String(s.bucket ?? '')
    form.domain = String(s.domain ?? '')
    form.upload_host = String(s.upload_host ?? '')
    form.zone = String(s.zone ?? '')
    form.private = Boolean(s.private)
    form.use_https = Boolean(s.use_https)
    form.presign_expiry_sec = Number(s.presign_expiry_sec ?? 3600)
  }
  formSnapshot.value = formState()
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
  } else if (form.driver === 'oss') {
    settings.endpoint = form.endpoint.trim()
    settings.region = form.region.trim()
    settings.bucket = form.bucket.trim()
    settings.domain = form.domain.trim()
    settings.private = form.private
    settings.use_https = form.use_https
    settings.presign_expiry_sec = 900
    secrets.access_key_id = form.oss_access_key_id.trim()
    secrets.access_key_secret = form.oss_access_key_secret.trim()
  } else if (form.driver === 'cos') {
    settings.region = form.region.trim()
    settings.bucket = form.bucket.trim()
    settings.domain = form.domain.trim()
    settings.private = form.private
    settings.use_https = form.use_https
    settings.presign_expiry_sec = 900
    secrets.secret_id = form.cos_secret_id.trim()
    secrets.secret_key = form.cos_secret_key.trim()
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
    formSnapshot.value = formState()
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

/** 删除后端的可预判失败原因：前端已有数据，不应等接口报错。 */
function backendDeleteBlockedReason(item: StorageBackend): string {
  if (item.is_current) return '当前默认存储不可删除，请先切换默认存储'
  if (item.image_count > 0) {
    return `该存储上仍有 ${formatNumber(item.image_count)} 张图片，请先迁移或删除后再试`
  }
  return ''
}

/** 删除存储池的可预判失败原因。 */
function poolDeleteBlockedReason(pool: StoragePool): string {
  if (pool.is_current) return '当前写入池不可删除，请先切换到其他存储池'
  return ''
}

async function remove(item: StorageBackend): Promise<void> {  try {
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

// ---- 存储池 ----
const poolDialogOpen = ref(false)
const poolEditingId = ref<string | null>(null)
const poolSubmitting = ref(false)
const poolPendingId = ref<string | null>(null)
const poolFormRef = ref<FormInstance>()

interface PoolMemberForm {
  backend_id: string
  weight: number
  priority: number
  enabled: boolean
}

interface PoolForm {
  name: string
  policy: StoragePoolPolicy
  members: PoolMemberForm[]
}

const poolForm = reactive<PoolForm>({ name: '', policy: 'weighted_rr', members: [] })
const poolFormSnapshot = ref('')

function poolFormState(): string {
  return JSON.stringify(poolForm)
}

async function closePoolDialog(done: () => void): Promise<void> {
  if (poolSubmitting.value) return
  if (poolFormState() === poolFormSnapshot.value) {
    done()
    return
  }
  try {
    await ElMessageBox.confirm('关闭后未保存的修改将丢失。', '放弃修改？', {
      confirmButtonText: '放弃修改',
      cancelButtonText: '继续编辑',
      type: 'warning',
      confirmButtonClass: 'el-button--danger',
    })
    done()
  } catch {
    // 用户选择继续编辑。
  }
}

const poolPolicyLabel: Record<StoragePoolPolicy, string> = {
  weighted_rr: '加权轮询',
  priority: '优先级',
}

const poolRules: FormRules = {
  name: [{ required: true, message: '请输入存储池名称', trigger: 'blur' }],
}

function emptyPoolForm(): PoolForm {
  return {
    name: '',
    policy: 'weighted_rr',
    members: [{ backend_id: '', weight: 100, priority: 0, enabled: true }],
  }
}

function openPoolCreate(): void {
  if (backends.value.length === 0) {
    ElMessage.warning('请先创建至少一个存储后端')
    return
  }
  poolEditingId.value = null
  Object.assign(poolForm, emptyPoolForm())
  poolFormSnapshot.value = poolFormState()
  poolDialogOpen.value = true
}

function openPoolEdit(pool: StoragePool): void {
  poolEditingId.value = pool.id
  poolForm.name = pool.name
  poolForm.policy = pool.policy
  poolForm.members = pool.members.map((m) => ({
    backend_id: m.backend_id,
    weight: m.weight,
    priority: m.priority,
    enabled: m.enabled,
  }))
  if (poolForm.members.length === 0) {
    poolForm.members.push({ backend_id: '', weight: 100, priority: 0, enabled: true })
  }
  poolFormSnapshot.value = poolFormState()
  poolDialogOpen.value = true
}

function addPoolMember(): void {
  poolForm.members.push({ backend_id: '', weight: 100, priority: 0, enabled: true })
}

function removePoolMember(index: number): void {
  poolForm.members.splice(index, 1)
}

async function submitPool(): Promise<void> {
  const instance = poolFormRef.value
  if (!instance || poolSubmitting.value) return
  const valid = await instance.validate().catch(() => false)
  if (!valid) return
  if (poolForm.members.some((m) => !m.backend_id)) {
    ElMessage.warning('请为每个成员选择存储后端')
    return
  }
  if (!poolForm.members.some((m) => m.enabled)) {
    ElMessage.warning('至少需要一个启用的成员')
    return
  }
  const payload: StoragePoolInput = {
    name: poolForm.name.trim(),
    policy: poolForm.policy,
    members: poolForm.members.map((m) => ({
      backend_id: m.backend_id,
      weight: Number(m.weight) || 100,
      priority: Number(m.priority) || 0,
      enabled: m.enabled,
    })),
  }
  poolSubmitting.value = true
  try {
    if (poolEditingId.value) {
      await updateStoragePool(poolEditingId.value, payload)
      ElMessage.success('存储池已更新')
    } else {
      await createStoragePool(payload)
      ElMessage.success('存储池已创建')
    }
    poolFormSnapshot.value = poolFormState()
    poolDialogOpen.value = false
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    poolSubmitting.value = false
  }
}

async function activatePool(pool: StoragePool): Promise<void> {
  try {
    await ElMessageBox.confirm(
      `启用后新上传的图片将按「${poolPolicyLabel[pool.policy]}」策略写入池内后端，已有图片仍从原存储读取。`,
      '启用存储池',
      { confirmButtonText: '启用', cancelButtonText: '取消', type: 'warning' },
    )
  } catch {
    return
  }
  poolPendingId.value = pool.id
  try {
    await activateStoragePool(pool.id)
    ElMessage.success('已启用存储池')
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    poolPendingId.value = null
  }
}

async function removePool(pool: StoragePool): Promise<void> {
  try {
    await ElMessageBox.confirm(
      `确定删除存储池「${pool.name}」吗？底层存储后端和已有图片不会被删除。`,
      '删除存储池',
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
  poolPendingId.value = pool.id
  try {
    await deleteStoragePool(pool.id)
    ElMessage.success('存储池已删除')
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    poolPendingId.value = null
  }
}

onMounted(load)
</script>

<template>
  <div class="ax-page">
    <PageHeader
      title="存储配置"
      description="管理图片的写入位置：先添加存储，再按需组合成存储池。"
    >
      <template #actions>
        <a
          class="storage-help"
          :href="storageDocsUrl"
          target="_blank"
          rel="noopener noreferrer"
        >
          <el-icon :size="14"><QuestionFilled /></el-icon>
          帮助
        </a>
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
        <el-button v-if="backends.length > 0" type="primary" :icon="Plus" @click="openCreate">
          新建存储
        </el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="load" />

    <div v-else-if="loading && backends.length === 0">
      <TableSkeleton :rows="6" :columns="5" title="正在加载存储配置" />
    </div>

    <EmptyState
      v-else-if="backends.length === 0"
      title="还没有存储配置"
      description="先添加一个本地目录或云存储。添加完成后，可按需把多个存储组合成存储池。"
    >
      <el-button type="primary" :icon="Plus" @click="openCreate">新建存储</el-button>
      <a class="storage-empty-help" :href="storageDocsUrl" target="_blank" rel="noopener noreferrer">
        查看存储说明
      </a>
    </EmptyState>

    <template v-else>
      <section class="storage-concepts" aria-label="存储与存储池说明">
        <div class="storage-concept">
          <span class="storage-concept__step">1</span>
          <div>
            <strong>存储</strong>
            <p>连接一个具体位置：本地目录或云存储账号。</p>
          </div>
        </div>
        <span class="storage-concept__arrow" aria-hidden="true">→</span>
        <div class="storage-concept">
          <span class="storage-concept__step">2</span>
          <div>
            <strong>存储池<el-tag size="small" type="info" effect="plain">可选</el-tag></strong>
            <p>把多个存储组合起来，按权重或优先级分配新上传。</p>
          </div>
        </div>
        <a class="storage-concept__link" :href="storageDocsUrl" target="_blank" rel="noopener noreferrer">
          了解详情
        </a>
      </section>

      <div class="ax-card table-card">
        <div class="ax-card__head">
          <div class="storage-table-head">
            <h2 class="ax-card__title">存储后端</h2>
            <p class="storage-table-head__hint">
              <template v-if="currentBackend">
                新上传写入「{{ currentBackend.name }}」；切换即时生效，无需重启。
              </template>
              <template v-else>尚未设置默认存储，请在下方选择一个存储并「设为默认」。</template>
            </p>
          </div>
          <el-tag size="small" type="info" effect="plain">{{ backends.length }} 个</el-tag>
        </div>
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
          <el-table-column label="操作" fixed="right" width="290" align="right">
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
                <el-tooltip
                  :disabled="!backendDeleteBlockedReason(row)"
                  :content="backendDeleteBlockedReason(row)"
                  placement="top"
                >
                  <span class="action-guard">
                    <el-button
                      link
                      type="danger"
                      :icon="Delete"
                      :disabled="Boolean(backendDeleteBlockedReason(row))"
                      :loading="pendingId === row.id"
                      @click="remove(row)"
                    >
                      删除
                    </el-button>
                  </span>
                </el-tooltip>
              </div>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </div>

    <template v-if="pools.length > 0">
      <div class="ax-card table-card pool-card">
        <div class="ax-card__head">
          <div class="storage-table-head">
            <h2 class="ax-card__title">存储池</h2>
            <p class="storage-table-head__hint">
              池只决定新对象写入哪个存储，已有图片仍从原存储读取。
            </p>
          </div>
          <div class="storage-head-actions">
            <el-tag size="small" type="info" effect="plain">{{ pools.length }} 个</el-tag>
            <el-button
              size="small"
              :icon="Plus"
              :disabled="!canCreatePool"
              :title="canCreatePool ? '新建存储池' : '至少需要 2 个存储后端'"
              @click="openPoolCreate"
            >
              新建存储池
            </el-button>
          </div>
        </div>
        <div class="ax-table-scroll">
        <el-table :data="pools" style="width: 100%" empty-text="暂无存储池">
          <el-table-column label="名称" min-width="180">
            <template #default="{ row }">
              <span class="storage-name">
                <el-icon :size="14"><FolderOpened /></el-icon>
                {{ row.name }}
                <el-tag v-if="row.is_current" size="small" type="success" effect="plain">
                  当前写入池
                </el-tag>
              </span>
            </template>
          </el-table-column>
          <el-table-column label="策略" width="120">
            <template #default="{ row }">{{ poolPolicyLabel[row.policy as StoragePoolPolicy] }}</template>
          </el-table-column>
          <el-table-column label="成员" width="90">
            <template #default="{ row }">{{ row.members.length }}</template>
          </el-table-column>
          <el-table-column label="图片数" width="110">
            <template #default="{ row }">{{ formatNumber(row.image_count) }}</template>
          </el-table-column>
          <el-table-column label="占用" width="120">
            <template #default="{ row }">{{ formatBytes(row.size_bytes) }}</template>
          </el-table-column>
          <el-table-column label="操作" fixed="right" width="250" align="right">
            <template #default="{ row }">
              <div class="ax-row-actions">
                <el-button
                  link
                  type="primary"
                  :icon="Star"
                  :disabled="row.is_current"
                  :loading="poolPendingId === row.id"
                  @click="activatePool(row)"
                >
                  启用
                </el-button>
                <el-button link type="primary" :icon="EditPen" @click="openPoolEdit(row)">编辑</el-button>
                <el-tooltip
                  :disabled="!poolDeleteBlockedReason(row)"
                  :content="poolDeleteBlockedReason(row)"
                  placement="top"
                >
                  <span class="action-guard">
                    <el-button
                      link
                      type="danger"
                      :icon="Delete"
                      :disabled="Boolean(poolDeleteBlockedReason(row))"
                      :loading="poolPendingId === row.id"
                      @click="removePool(row)"
                    >
                      删除
                    </el-button>
                  </span>
                </el-tooltip>
              </div>
            </template>
          </el-table-column>
        </el-table>
      </div>
      </div>
    </template>

    <div v-else class="ax-card storage-pool-empty pool-card">
      <span class="storage-pool-empty__badge">可选</span>
      <div class="storage-pool-empty__text">
        <h3 class="storage-pool-empty__title">还没有存储池</h3>
        <p v-if="canCreatePool">
          将多个存储组合成池，可按权重或优先级分流新上传；单个存储无需创建池。
        </p>
        <p v-else>
          存储池用于在多个存储之间分流写入。再添加一个存储后，即可把两个存储组合成池。
        </p>
      </div>
      <el-button
        v-if="canCreatePool"
        :icon="Plus"
        @click="openPoolCreate"
      >
        新建存储池
      </el-button>
      <el-button v-else :icon="Plus" @click="openCreate">再添加一个存储</el-button>
    </div>
    </template>

    <el-dialog
      v-model="poolDialogOpen"
      :title="poolEditingId ? '编辑存储池' : '新建存储池'"
      width="min(680px, 94vw)"
      append-to-body
      :close-on-click-modal="false"
      :before-close="closePoolDialog"
      @closed="poolFormRef?.clearValidate()"
    >
      <el-form ref="poolFormRef" :model="poolForm" :rules="poolRules" label-position="top">
        <el-form-item label="名称" prop="name">
          <el-input v-model="poolForm.name" placeholder="例如：主存储池" maxlength="60" />
        </el-form-item>
        <el-form-item label="放置策略">
          <el-radio-group v-model="poolForm.policy">
            <el-radio-button value="weighted_rr">加权轮询</el-radio-button>
            <el-radio-button value="priority">优先级</el-radio-button>
          </el-radio-group>
          <p class="storage-hint">
            加权轮询按 Weight 分配新对象；优先级先使用 Priority 数值更小的成员，同级内加权轮询。
          </p>
        </el-form-item>

        <el-divider content-position="left">成员</el-divider>
        <div v-for="(member, index) in poolForm.members" :key="index" class="pool-member-row">
          <el-select v-model="member.backend_id" placeholder="选择存储后端" style="min-width: 180px">
            <el-option
              v-for="backend in backends"
              :key="backend.id"
              :label="`${backend.name}（${driverLabel[backend.driver]}）`"
              :value="backend.id"
            />
          </el-select>
          <el-input-number v-model="member.weight" :min="1" :max="10000" controls-position="right" />
          <span class="pool-member-label">权重</span>
          <el-input-number v-model="member.priority" :min="0" :max="1000" controls-position="right" />
          <span class="pool-member-label">优先级</span>
          <el-switch v-model="member.enabled" active-text="启用" />
          <el-button
            link
            type="danger"
            :disabled="poolForm.members.length <= 1"
            @click="removePoolMember(index)"
          >
            移除
          </el-button>
        </div>
        <el-button link type="primary" :icon="Plus" @click="addPoolMember">添加成员</el-button>
      </el-form>
      <template #footer>
        <el-button @click="poolDialogOpen = false">取消</el-button>
        <el-button type="primary" :loading="poolSubmitting" @click="submitPool">
          {{ poolEditingId ? '保存' : '创建' }}
        </el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="dialogOpen"
      :title="isEdit ? '编辑存储配置' : '新建存储配置'"
      width="min(560px, 94vw)"
      append-to-body
      :close-on-click-modal="false"
      :before-close="closeFormDialog"
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
            <el-radio-button value="oss">阿里云 OSS</el-radio-button>
            <el-radio-button value="cos">腾讯云 COS</el-radio-button>
            <el-radio-button value="qiniu">七牛云</el-radio-button>
          </el-radio-group>
        </el-form-item>

        <!-- 本地 -->
        <template v-if="form.driver === 'local'">
          <el-divider content-position="left">本地路径</el-divider>
          <el-form-item label="根目录">
            <el-input v-model="form.root" placeholder="留空则使用配置文件中的默认目录" />
          </el-form-item>
          <p class="storage-hint">
            相对路径相对于服务启动目录；留空时回退到 config.yaml 的 storage.local.root。
          </p>
        </template>

        <!-- S3 -->
        <template v-else-if="form.driver === 's3'">
          <el-divider content-position="left">连接信息</el-divider>
          <el-form-item label="Endpoint">
            <el-input v-model="form.endpoint" placeholder="https://s3.amazonaws.com" />
          </el-form-item>
          <el-form-item label="Region">
            <el-input v-model="form.region" placeholder="us-east-1" />
          </el-form-item>
          <el-form-item label="Bucket">
            <el-input v-model="form.bucket" placeholder="axmipic" />
          </el-form-item>
          <el-divider content-position="left">凭据</el-divider>
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
        <template v-else-if="form.driver === 'qiniu'">
          <el-divider content-position="left">连接信息</el-divider>
          <el-form-item label="Bucket">
            <el-input v-model="form.bucket" placeholder="axmipic" />
          </el-form-item>
          <el-form-item label="访问域名">
            <el-input v-model="form.domain" placeholder="https://cdn.example.com" />
          </el-form-item>
          <el-divider content-position="left">凭据</el-divider>
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

        <!-- 阿里云 OSS -->
        <template v-else-if="form.driver === 'oss'">
          <el-divider content-position="left">连接信息</el-divider>
          <el-form-item label="Region">
            <el-input v-model="form.region" placeholder="cn-hangzhou" />
          </el-form-item>
          <el-form-item label="Endpoint（可选）">
            <el-input
              v-model="form.endpoint"
              placeholder="留空则由 Region 推导，如 https://oss-cn-hangzhou.aliyuncs.com"
            />
          </el-form-item>
          <el-form-item label="Bucket">
            <el-input v-model="form.bucket" placeholder="axmipic" />
          </el-form-item>
          <el-divider content-position="left">凭据</el-divider>
          <el-form-item label="AccessKey ID">
            <el-input
              v-model="form.oss_access_key_id"
              :placeholder="isEdit ? '留空表示保持不变' : ''"
            />
          </el-form-item>
          <el-form-item label="AccessKey Secret">
            <el-input
              v-model="form.oss_access_key_secret"
              type="password"
              show-password
              :placeholder="isEdit ? '留空表示保持不变' : ''"
            />
          </el-form-item>
          <el-form-item label="访问域名（可选）">
            <el-input v-model="form.domain" placeholder="https://cdn.example.com" />
          </el-form-item>
          <div class="storage-switches">
            <el-checkbox v-model="form.use_https">使用 HTTPS</el-checkbox>
            <el-checkbox v-model="form.private">私有 Bucket</el-checkbox>
          </div>
        </template>

        <!-- 腾讯云 COS -->
        <template v-else-if="form.driver === 'cos'">
          <el-divider content-position="left">连接信息</el-divider>
          <el-form-item label="Region">
            <el-input v-model="form.region" placeholder="ap-guangzhou" />
          </el-form-item>
          <el-form-item label="Bucket">
            <el-input v-model="form.bucket" placeholder="axmipic-1250000000" />
            <p class="storage-hint">格式为「存储桶名称-APPID」，例如 axmipic-1250000000。</p>
          </el-form-item>
          <el-divider content-position="left">凭据</el-divider>
          <el-form-item label="SecretId">
            <el-input
              v-model="form.cos_secret_id"
              :placeholder="isEdit ? '留空表示保持不变' : ''"
            />
          </el-form-item>
          <el-form-item label="SecretKey">
            <el-input
              v-model="form.cos_secret_key"
              type="password"
              show-password
              :placeholder="isEdit ? '留空表示保持不变' : ''"
            />
          </el-form-item>
          <el-form-item label="访问域名（可选）">
            <el-input v-model="form.domain" placeholder="https://cdn.example.com" />
          </el-form-item>
          <div class="storage-switches">
            <el-checkbox v-model="form.use_https">使用 HTTPS</el-checkbox>
            <el-checkbox v-model="form.private">私有存储桶</el-checkbox>
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

.storage-help {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 32px;
  padding: 0 var(--ax-space-3);
  color: var(--ax-text-3);
  border: 1px solid transparent;
  border-radius: var(--ax-radius-sm);
  font-size: var(--ax-text-sm);
  font-weight: var(--ax-weight-medium);
  transition:
    color var(--ax-duration-fast) var(--ax-ease),
    background-color var(--ax-duration-fast) var(--ax-ease);
}

.storage-help:hover {
  color: var(--ax-text);
  background: var(--ax-tint);
}

.storage-empty-help {
  align-self: center;
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
}

.storage-empty-help:hover {
  color: var(--ax-accent-hover);
}

.storage-concepts {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--ax-space-3);
  margin-bottom: var(--ax-space-4);
  padding: var(--ax-space-3) var(--ax-space-4);
  background: var(--ax-tint-weak);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-md);
}

.storage-concept {
  display: flex;
  align-items: flex-start;
  gap: var(--ax-space-2);
  min-width: 0;
}

.storage-concept__step {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  color: var(--ax-accent-hover);
  background: var(--ax-accent-soft);
  border-radius: var(--ax-radius-full);
  font-size: var(--ax-text-2xs);
  font-weight: var(--ax-weight-semibold);
}

.storage-concept strong {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--ax-text);
  font-size: var(--ax-text-sm);
  font-weight: var(--ax-weight-medium);
}

.storage-concept p {
  margin-top: 2px;
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.storage-concept__arrow {
  color: var(--ax-text-4);
}

.storage-concept__link {
  margin-left: auto;
  color: var(--ax-accent-hover);
  font-size: var(--ax-text-sm);
  white-space: nowrap;
}

.storage-table-head__hint {
  margin-top: 2px;
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.storage-head-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--ax-space-2);
}

.storage-pool-empty {
  display: flex;
  align-items: center;
  gap: var(--ax-space-4);
  padding: var(--ax-space-5);
}

.storage-pool-empty__badge {
  flex: none;
  padding: 3px 8px;
  color: var(--ax-text-3);
  background: var(--ax-tint);
  border-radius: var(--ax-radius-full);
  font-size: var(--ax-text-2xs);
  font-weight: var(--ax-weight-medium);
}

.storage-pool-empty__text {
  flex: 1;
  min-width: 0;
}

.storage-pool-empty__title {
  color: var(--ax-text);
  font-size: var(--ax-text-base);
  font-weight: var(--ax-weight-medium);
}

.storage-pool-empty__text p {
  margin-top: var(--ax-space-1);
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
}

.storage-pool-empty > .el-button {
  flex: none;
}

@media (max-width: 768px) {
  .storage-concept__arrow {
    display: none;
  }

  .storage-concept__link {
    margin-left: 0;
  }

  .storage-pool-empty {
    flex-wrap: wrap;
  }

  .storage-pool-empty > .el-button {
    margin-left: 0;
  }
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

.pool-card {
  margin-top: var(--ax-space-4);
}

.pool-member-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--ax-space-2);
  margin-bottom: var(--ax-space-3);
}

.pool-member-label {
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

/* Tooltip 需要包裹一个可悬停元素，禁用的按钮本身不触发鼠标事件。 */
.action-guard {
  display: inline-flex;
}
</style>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
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

const { t } = useI18n()

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
    await ElMessageBox.confirm(t('storage.confirm.discardMessage'), t('storage.confirm.discardTitle'), {
      confirmButtonText: t('storage.confirm.discardConfirm'),
      cancelButtonText: t('storage.confirm.continueEditing'),
      type: 'warning',
      confirmButtonClass: 'el-button--danger',
    })
    done()
  } catch {
    // 用户选择继续编辑。
  }
}

const isEdit = computed(() => editingId.value !== null)
const driverLabel = computed<Record<StorageDriver, string>>(() => ({
  local: t('storage.drivers.local'),
  s3: t('storage.drivers.s3'),
  qiniu: t('storage.drivers.qiniu'),
  oss: t('storage.drivers.oss'),
  cos: t('storage.drivers.cos'),
}))

const rules = computed<FormRules>(() => ({
  name: [{ required: true, message: t('storage.messages.nameRequired'), trigger: 'blur' }],
}))

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
      ElMessage.success(t('storage.messages.updated'))
    } else {
      await createStorage(buildPayload())
      ElMessage.success(t('storage.messages.created'))
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
      t('storage.confirm.activateMessage', { name: item.name }),
      t('storage.confirm.activateTitle'),
      { confirmButtonText: t('storage.confirm.activateConfirm'), cancelButtonText: t('common.cancel'), type: 'warning' },
    )
  } catch {
    return
  }
  pendingId.value = item.id
  try {
    await activateStorage(item.id)
    ElMessage.success(t('storage.messages.activated'))
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    pendingId.value = null
  }
}

/** 删除后端的可预判失败原因：前端已有数据，不应等接口报错。 */
function backendDeleteBlockedReason(item: StorageBackend): string {
  if (item.is_current) return t('storage.blocked.currentBackend')
  if (item.image_count > 0) {
    return t('storage.blocked.backendImages', { count: formatNumber(item.image_count) })
  }
  return ''
}

/** 删除存储池的可预判失败原因。 */
function poolDeleteBlockedReason(pool: StoragePool): string {
  if (pool.is_current) return t('storage.blocked.currentPool')
  return ''
}

async function remove(item: StorageBackend): Promise<void> {  try {
    await ElMessageBox.confirm(
      t('storage.confirm.deleteMessage', { name: item.name }),
      t('storage.confirm.deleteTitle'),
      {
        confirmButtonText: t('storage.confirm.deleteConfirm'),
        cancelButtonText: t('common.cancel'),
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
    ElMessage.success(t('storage.messages.deleted'))
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
    await ElMessageBox.confirm(t('storage.confirm.discardMessage'), t('storage.confirm.discardTitle'), {
      confirmButtonText: t('storage.confirm.discardConfirm'),
      cancelButtonText: t('storage.confirm.continueEditing'),
      type: 'warning',
      confirmButtonClass: 'el-button--danger',
    })
    done()
  } catch {
    // 用户选择继续编辑。
  }
}

const poolPolicyLabel = computed<Record<StoragePoolPolicy, string>>(() => ({
  weighted_rr: t('storage.pools.policyWeighted'),
  priority: t('storage.pools.policyPriority'),
}))

const poolRules = computed<FormRules>(() => ({
  name: [{ required: true, message: t('storage.messages.poolNameRequired'), trigger: 'blur' }],
}))

function emptyPoolForm(): PoolForm {
  return {
    name: '',
    policy: 'weighted_rr',
    members: [{ backend_id: '', weight: 100, priority: 0, enabled: true }],
  }
}

function openPoolCreate(): void {
  if (backends.value.length === 0) {
    ElMessage.warning(t('storage.messages.needBackend'))
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
    ElMessage.warning(t('storage.messages.memberBackendRequired'))
    return
  }
  if (!poolForm.members.some((m) => m.enabled)) {
    ElMessage.warning(t('storage.messages.memberEnabledRequired'))
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
      ElMessage.success(t('storage.messages.poolUpdated'))
    } else {
      await createStoragePool(payload)
      ElMessage.success(t('storage.messages.poolCreated'))
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
      t('storage.confirm.activatePoolMessage', { policy: poolPolicyLabel.value[pool.policy] }),
      t('storage.confirm.activatePoolTitle'),
      { confirmButtonText: t('storage.confirm.activatePoolConfirm'), cancelButtonText: t('common.cancel'), type: 'warning' },
    )
  } catch {
    return
  }
  poolPendingId.value = pool.id
  try {
    await activateStoragePool(pool.id)
    ElMessage.success(t('storage.messages.poolActivated'))
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
      t('storage.confirm.deletePoolMessage', { name: pool.name }),
      t('storage.confirm.deletePoolTitle'),
      {
        confirmButtonText: t('storage.confirm.deleteConfirm'),
        cancelButtonText: t('common.cancel'),
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
    ElMessage.success(t('storage.messages.poolDeleted'))
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
      :title="t('storage.page.title')"
      :description="t('storage.page.description')"
    >
      <template #actions>
        <a
          class="storage-help"
          :href="storageDocsUrl"
          target="_blank"
          rel="noopener noreferrer"
        >
          <el-icon :size="14"><QuestionFilled /></el-icon>
          {{ t('storage.actions.help') }}
        </a>
        <el-button :icon="Refresh" :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
        <el-button v-if="backends.length > 0" type="primary" :icon="Plus" @click="openCreate">
          {{ t('storage.actions.create') }}
        </el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="load" />

    <div v-else-if="loading && backends.length === 0">
      <TableSkeleton :rows="6" :columns="5" :title="t('storage.loading')" />
    </div>

    <EmptyState
      v-else-if="backends.length === 0"
      :title="t('storage.empty.title')"
      :description="t('storage.empty.description')"
    >
      <el-button type="primary" :icon="Plus" @click="openCreate">{{ t('storage.actions.create') }}</el-button>
      <a class="storage-empty-help" :href="storageDocsUrl" target="_blank" rel="noopener noreferrer">
        {{ t('storage.actions.viewDocs') }}
      </a>
    </EmptyState>

    <template v-else>
      <section class="storage-concepts" :aria-label="t('storage.concepts.ariaLabel')">
        <div class="storage-concept">
          <span class="storage-concept__step">1</span>
          <div>
            <strong>{{ t('storage.concepts.storageTitle') }}</strong>
            <p>{{ t('storage.concepts.storageDesc') }}</p>
          </div>
        </div>
        <span class="storage-concept__arrow" aria-hidden="true">→</span>
        <div class="storage-concept">
          <span class="storage-concept__step">2</span>
          <div>
            <strong>{{ t('storage.concepts.poolTitle') }}<el-tag size="small" type="info" effect="plain">{{ t('common.optional') }}</el-tag></strong>
            <p>{{ t('storage.concepts.poolDesc') }}</p>
          </div>
        </div>
        <a class="storage-concept__link" :href="storageDocsUrl" target="_blank" rel="noopener noreferrer">
          {{ t('storage.actions.learnMore') }}
        </a>
      </section>

      <div class="ax-card table-card">
        <div class="ax-card__head">
          <div class="storage-table-head">
            <h2 class="ax-card__title">{{ t('storage.backends.title') }}</h2>
            <p class="storage-table-head__hint">
              <template v-if="currentBackend">
                {{ t('storage.backends.hintCurrent', { name: currentBackend.name }) }}
              </template>
              <template v-else>{{ t('storage.backends.hintNone') }}</template>
            </p>
          </div>
          <el-tag size="small" type="info" effect="plain">{{ t('storage.backends.count', { count: backends.length }) }}</el-tag>
        </div>
        <div class="ax-table-scroll">
        <el-table :data="backends" style="width: 100%">
          <el-table-column :label="t('storage.backends.columns.name')" min-width="180">
            <template #default="{ row }">
              <span class="storage-name">
                <el-icon :size="14"><FolderOpened /></el-icon>
                {{ row.name }}
                <el-tag v-if="row.is_current" size="small" type="success" effect="plain">
                  {{ t('storage.backends.current') }}
                </el-tag>
              </span>
            </template>
          </el-table-column>
          <el-table-column :label="t('storage.backends.columns.type')" width="180">
            <template #default="{ row }">{{ driverLabel[row.driver as StorageDriver] }}</template>
          </el-table-column>
          <el-table-column :label="t('storage.backends.columns.images')" width="120">
            <template #default="{ row }">{{ formatNumber(row.image_count) }}</template>
          </el-table-column>
          <el-table-column :label="t('storage.backends.columns.createdAt')" min-width="170">
            <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column :label="t('storage.backends.columns.actions')" fixed="right" width="290" align="right">
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
                  {{ t('storage.actions.setDefault') }}
                </el-button>
                <el-button link type="primary" :icon="EditPen" @click="openEdit(row)">{{ t('common.edit') }}</el-button>
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
                      {{ t('common.delete') }}
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
            <h2 class="ax-card__title">{{ t('storage.pools.title') }}</h2>
            <p class="storage-table-head__hint">
              {{ t('storage.pools.hint') }}
            </p>
          </div>
          <div class="storage-head-actions">
            <el-tag size="small" type="info" effect="plain">{{ t('storage.pools.count', { count: pools.length }) }}</el-tag>
            <el-button
              size="small"
              :icon="Plus"
              :disabled="!canCreatePool"
              :title="canCreatePool ? t('storage.pools.newTitle') : t('storage.pools.minBackendsHint')"
              @click="openPoolCreate"
            >
              {{ t('storage.actions.createPool') }}
            </el-button>
          </div>
        </div>
        <div class="ax-table-scroll">
        <el-table :data="pools" style="width: 100%" :empty-text="t('storage.pools.tableEmpty')">
          <el-table-column :label="t('storage.pools.columns.name')" min-width="180">
            <template #default="{ row }">
              <span class="storage-name">
                <el-icon :size="14"><FolderOpened /></el-icon>
                {{ row.name }}
                <el-tag v-if="row.is_current" size="small" type="success" effect="plain">
                  {{ t('storage.pools.current') }}
                </el-tag>
              </span>
            </template>
          </el-table-column>
          <el-table-column :label="t('storage.pools.columns.policy')" width="120">
            <template #default="{ row }">{{ poolPolicyLabel[row.policy as StoragePoolPolicy] }}</template>
          </el-table-column>
          <el-table-column :label="t('storage.pools.columns.members')" width="90">
            <template #default="{ row }">{{ row.members.length }}</template>
          </el-table-column>
          <el-table-column :label="t('storage.pools.columns.images')" width="110">
            <template #default="{ row }">{{ formatNumber(row.image_count) }}</template>
          </el-table-column>
          <el-table-column :label="t('storage.pools.columns.size')" width="120">
            <template #default="{ row }">{{ formatBytes(row.size_bytes) }}</template>
          </el-table-column>
          <el-table-column :label="t('storage.pools.columns.actions')" fixed="right" width="250" align="right">
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
                  {{ t('storage.actions.enable') }}
                </el-button>
                <el-button link type="primary" :icon="EditPen" @click="openPoolEdit(row)">{{ t('common.edit') }}</el-button>
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
                      {{ t('common.delete') }}
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
      <span class="storage-pool-empty__badge">{{ t('common.optional') }}</span>
      <div class="storage-pool-empty__text">
        <h3 class="storage-pool-empty__title">{{ t('storage.pools.emptyTitle') }}</h3>
        <p v-if="canCreatePool">
          {{ t('storage.pools.emptyHintCanCreate') }}
        </p>
        <p v-else>
          {{ t('storage.pools.emptyHintNeedMore') }}
        </p>
      </div>
      <el-button
        v-if="canCreatePool"
        :icon="Plus"
        @click="openPoolCreate"
      >
        {{ t('storage.actions.createPool') }}
      </el-button>
      <el-button v-else :icon="Plus" @click="openCreate">{{ t('storage.actions.addAnother') }}</el-button>
    </div>
    </template>

    <el-dialog
      v-model="poolDialogOpen"
      :title="poolEditingId ? t('storage.pools.editTitle') : t('storage.pools.newTitle')"
      width="min(680px, 94vw)"
      append-to-body
      :close-on-click-modal="false"
      :before-close="closePoolDialog"
      @closed="poolFormRef?.clearValidate()"
    >
      <el-form ref="poolFormRef" :model="poolForm" :rules="poolRules" label-position="top">
        <el-form-item :label="t('storage.pools.name')" prop="name">
          <el-input v-model="poolForm.name" :placeholder="t('storage.pools.namePlaceholder')" maxlength="60" />
        </el-form-item>
        <el-form-item :label="t('storage.pools.policy')">
          <el-radio-group v-model="poolForm.policy">
            <el-radio-button value="weighted_rr">{{ t('storage.pools.policyWeighted') }}</el-radio-button>
            <el-radio-button value="priority">{{ t('storage.pools.policyPriority') }}</el-radio-button>
          </el-radio-group>
          <p class="storage-hint">
            {{ t('storage.pools.policyHint') }}
          </p>
        </el-form-item>

        <el-divider content-position="left">{{ t('storage.pools.members') }}</el-divider>
        <div v-for="(member, index) in poolForm.members" :key="index" class="pool-member-row">
          <el-select v-model="member.backend_id" :placeholder="t('storage.pools.memberBackend')" style="min-width: 180px">
            <el-option
              v-for="backend in backends"
              :key="backend.id"
              :label="t('storage.pools.backendOption', { name: backend.name, driver: driverLabel[backend.driver] })"
              :value="backend.id"
            />
          </el-select>
          <el-input-number v-model="member.weight" :min="1" :max="10000" controls-position="right" />
          <span class="pool-member-label">{{ t('storage.pools.weight') }}</span>
          <el-input-number v-model="member.priority" :min="0" :max="1000" controls-position="right" />
          <span class="pool-member-label">{{ t('storage.pools.priority') }}</span>
          <el-switch v-model="member.enabled" :active-text="t('storage.pools.enabled')" />
          <el-button
            link
            type="danger"
            :disabled="poolForm.members.length <= 1"
            @click="removePoolMember(index)"
          >
            {{ t('common.remove') }}
          </el-button>
        </div>
        <el-button link type="primary" :icon="Plus" @click="addPoolMember">{{ t('storage.pools.addMember') }}</el-button>
      </el-form>
      <template #footer>
        <el-button @click="poolDialogOpen = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="poolSubmitting" @click="submitPool">
          {{ poolEditingId ? t('common.save') : t('storage.actions.createSubmit') }}
        </el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="dialogOpen"
      :title="isEdit ? t('storage.form.editTitle') : t('storage.form.createTitle')"
      width="min(560px, 94vw)"
      append-to-body
      :close-on-click-modal="false"
      :before-close="closeFormDialog"
      @closed="formRef?.clearValidate()"
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
        <el-form-item :label="t('storage.form.name')" prop="name">
          <el-input v-model="form.name" :placeholder="t('storage.form.namePlaceholder')" maxlength="60" />
        </el-form-item>

        <el-form-item :label="t('storage.form.driver')">
          <el-radio-group v-model="form.driver" :disabled="isEdit">
            <el-radio-button value="local">{{ t('storage.driverOptions.local') }}</el-radio-button>
            <el-radio-button value="s3">{{ t('storage.driverOptions.s3') }}</el-radio-button>
            <el-radio-button value="oss">{{ t('storage.driverOptions.oss') }}</el-radio-button>
            <el-radio-button value="cos">{{ t('storage.driverOptions.cos') }}</el-radio-button>
            <el-radio-button value="qiniu">{{ t('storage.driverOptions.qiniu') }}</el-radio-button>
          </el-radio-group>
        </el-form-item>

        <!-- 本地 -->
        <template v-if="form.driver === 'local'">
          <el-divider content-position="left">{{ t('storage.form.localPath') }}</el-divider>
          <el-form-item :label="t('storage.form.root')">
            <el-input v-model="form.root" :placeholder="t('storage.form.rootPlaceholder')" />
          </el-form-item>
          <p class="storage-hint">
            {{ t('storage.form.rootHint') }}
          </p>
        </template>

        <!-- S3 -->
        <template v-else-if="form.driver === 's3'">
          <el-divider content-position="left">{{ t('storage.form.connection') }}</el-divider>
          <el-form-item :label="t('storage.form.endpoint')">
            <el-input v-model="form.endpoint" :placeholder="t('storage.form.endpointPlaceholder')" />
          </el-form-item>
          <el-form-item :label="t('storage.form.region')">
            <el-input v-model="form.region" :placeholder="t('storage.form.regionPlaceholderS3')" />
          </el-form-item>
          <el-form-item :label="t('storage.form.bucket')">
            <el-input v-model="form.bucket" :placeholder="t('storage.form.bucketPlaceholder')" />
          </el-form-item>
          <el-divider content-position="left">{{ t('storage.form.credentials') }}</el-divider>
          <el-form-item :label="t('storage.form.accessKeyId')">
            <el-input
              v-model="form.access_key_id"
              :placeholder="isEdit ? t('storage.form.keepUnchanged') : ''"
            />
          </el-form-item>
          <el-form-item :label="t('storage.form.secretAccessKey')">
            <el-input
              v-model="form.secret_access_key"
              type="password"
              show-password
              :placeholder="isEdit ? t('storage.form.keepUnchanged') : ''"
            />
          </el-form-item>
          <el-form-item :label="t('storage.form.publicBaseUrl')">
            <el-input v-model="form.public_base_url" :placeholder="t('storage.form.publicBaseUrlPlaceholder')" />
          </el-form-item>
          <div class="storage-switches">
            <el-checkbox v-model="form.secure">{{ t('storage.form.useHttps') }}</el-checkbox>
            <el-checkbox v-model="form.use_path_style">{{ t('storage.form.pathStyle') }}</el-checkbox>
          </div>
        </template>

        <!-- 七牛 -->
        <template v-else-if="form.driver === 'qiniu'">
          <el-divider content-position="left">{{ t('storage.form.connection') }}</el-divider>
          <el-form-item :label="t('storage.form.bucket')">
            <el-input v-model="form.bucket" :placeholder="t('storage.form.bucketPlaceholder')" />
          </el-form-item>
          <el-form-item :label="t('storage.form.domain')">
            <el-input v-model="form.domain" :placeholder="t('storage.form.publicBaseUrlPlaceholder')" />
          </el-form-item>
          <el-divider content-position="left">{{ t('storage.form.credentials') }}</el-divider>
          <el-form-item :label="t('storage.form.accessKey')">
            <el-input
              v-model="form.qiniu_access_key"
              :placeholder="isEdit ? t('storage.form.keepUnchanged') : ''"
            />
          </el-form-item>
          <el-form-item :label="t('storage.form.secretKey')">
            <el-input
              v-model="form.qiniu_secret_key"
              type="password"
              show-password
              :placeholder="isEdit ? t('storage.form.keepUnchanged') : ''"
            />
          </el-form-item>
          <el-form-item :label="t('storage.form.uploadHost')">
            <el-input v-model="form.upload_host" :placeholder="t('storage.form.uploadHostPlaceholder')" />
          </el-form-item>
          <div class="storage-switches">
            <el-checkbox v-model="form.use_https">{{ t('storage.form.useHttps') }}</el-checkbox>
            <el-checkbox v-model="form.private">{{ t('storage.form.privateSpace') }}</el-checkbox>
          </div>
        </template>

        <!-- 阿里云 OSS -->
        <template v-else-if="form.driver === 'oss'">
          <el-divider content-position="left">{{ t('storage.form.connection') }}</el-divider>
          <el-form-item :label="t('storage.form.region')">
            <el-input v-model="form.region" :placeholder="t('storage.form.regionPlaceholderOss')" />
          </el-form-item>
          <el-form-item :label="t('storage.form.endpointOptional')">
            <el-input
              v-model="form.endpoint"
              :placeholder="t('storage.form.endpointOptionalPlaceholder')"
            />
          </el-form-item>
          <el-form-item :label="t('storage.form.bucket')">
            <el-input v-model="form.bucket" :placeholder="t('storage.form.bucketPlaceholder')" />
          </el-form-item>
          <el-divider content-position="left">{{ t('storage.form.credentials') }}</el-divider>
          <el-form-item :label="t('storage.form.ossAccessKeyId')">
            <el-input
              v-model="form.oss_access_key_id"
              :placeholder="isEdit ? t('storage.form.keepUnchanged') : ''"
            />
          </el-form-item>
          <el-form-item :label="t('storage.form.ossAccessKeySecret')">
            <el-input
              v-model="form.oss_access_key_secret"
              type="password"
              show-password
              :placeholder="isEdit ? t('storage.form.keepUnchanged') : ''"
            />
          </el-form-item>
          <el-form-item :label="t('storage.form.domainOptional')">
            <el-input v-model="form.domain" :placeholder="t('storage.form.publicBaseUrlPlaceholder')" />
          </el-form-item>
          <div class="storage-switches">
            <el-checkbox v-model="form.use_https">{{ t('storage.form.useHttps') }}</el-checkbox>
            <el-checkbox v-model="form.private">{{ t('storage.form.privateBucket') }}</el-checkbox>
          </div>
        </template>

        <!-- 腾讯云 COS -->
        <template v-else-if="form.driver === 'cos'">
          <el-divider content-position="left">{{ t('storage.form.connection') }}</el-divider>
          <el-form-item :label="t('storage.form.region')">
            <el-input v-model="form.region" :placeholder="t('storage.form.regionPlaceholderCos')" />
          </el-form-item>
          <el-form-item :label="t('storage.form.bucket')">
            <el-input v-model="form.bucket" :placeholder="t('storage.form.bucketPlaceholderCos')" />
            <p class="storage-hint">{{ t('storage.form.bucketHintCos') }}</p>
          </el-form-item>
          <el-divider content-position="left">{{ t('storage.form.credentials') }}</el-divider>
          <el-form-item :label="t('storage.form.secretId')">
            <el-input
              v-model="form.cos_secret_id"
              :placeholder="isEdit ? t('storage.form.keepUnchanged') : ''"
            />
          </el-form-item>
          <el-form-item :label="t('storage.form.secretKeyCos')">
            <el-input
              v-model="form.cos_secret_key"
              type="password"
              show-password
              :placeholder="isEdit ? t('storage.form.keepUnchanged') : ''"
            />
          </el-form-item>
          <el-form-item :label="t('storage.form.domainOptional')">
            <el-input v-model="form.domain" :placeholder="t('storage.form.publicBaseUrlPlaceholder')" />
          </el-form-item>
          <div class="storage-switches">
            <el-checkbox v-model="form.use_https">{{ t('storage.form.useHttps') }}</el-checkbox>
            <el-checkbox v-model="form.private">{{ t('storage.form.privateBucketCos') }}</el-checkbox>
          </div>
        </template>

        <el-form-item v-if="!isEdit" :label="t('storage.form.activate')">
          <el-switch v-model="form.activate" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogOpen = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="submitting" @click="submit">
          {{ isEdit ? t('common.save') : t('storage.actions.createSubmit') }}
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

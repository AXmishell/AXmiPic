<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import type { FormInstance, FormRules } from 'element-plus'
import { Delete, EditPen, Link, Plus, Refresh } from '@element-plus/icons-vue'

import {
  attachPolicy,
  createPolicy,
  createRoleGroup,
  deletePolicy,
  deleteRoleGroup,
  detachPolicy,
  listPolicies,
  listRoleGroups,
  updatePolicy,
  updateRoleGroup,
} from '@/api/admin'
import { toApiError } from '@/api/client'
import type { Policy, PolicyInput, PolicyType, RoleGroup, RoleGroupInput } from '@/api/types'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'
import PageHeader from '@/components/PageHeader.vue'
import TableSkeleton from '@/components/TableSkeleton.vue'
import { formatDateTime } from '@/utils/format'
import { useTabQuery } from '@/utils/useTabQuery'

type Tab = 'groups' | 'policies'
const POLICY_TABS: readonly Tab[] = ['groups', 'policies']
const activeTab = useTabQuery<Tab>('tab', 'groups', POLICY_TABS)

const { t } = useI18n()

const groups = ref<RoleGroup[]>([])
const policies = ref<Policy[]>([])
const loading = ref(false)
const errorMessage = ref('')

/** 策略类型元数据：展示名、标签色、配置模板与说明。 */
const policyTypes = computed<
  { value: PolicyType; label: string; tag: string; template: Record<string, unknown>; hint: string }[]
>(() => [
  {
    value: 'quota',
    label: t('policies.types.quota.label'),
    tag: 'primary',
    template: { quota_mb: 1024 },
    hint: t('policies.types.quota.hint'),
  },
  {
    value: 'upload',
    label: t('policies.types.upload.label'),
    tag: 'success',
    template: { max_size_mb: 20, allowed_mime_types: ['image/jpeg', 'image/png', 'image/gif', 'image/webp'] },
    hint: t('policies.types.upload.hint'),
  },
  {
    value: 'rate',
    label: t('policies.types.rate.label'),
    tag: 'warning',
    template: { upload_per_minute: 30, upload_burst: 5, image_per_minute: 600, image_burst: 120 },
    hint: t('policies.types.rate.hint'),
  },
  {
    value: 'processing',
    label: t('policies.types.processing.label'),
    tag: 'info',
    template: {
      enabled: true,
      max_width: 4096,
      max_height: 4096,
      default_quality: 82,
      allowed_formats: ['jpeg', 'png', 'gif', 'webp', 'avif'],
    },
    hint: t('policies.types.processing.hint'),
  },
  {
    value: 'feature',
    label: t('policies.types.feature.label'),
    tag: 'danger',
    template: { features: ['plaza', 'albums', 'api_tokens', 'batch_upload', 'paste_upload', 'share'] },
    hint: t('policies.types.feature.hint'),
  },
])

function typeMeta(type: PolicyType) {
  return policyTypes.value.find((item) => item.value === type) ?? policyTypes.value[0]!
}

const featureOptions = computed(() => [
  { value: 'plaza', label: t('policies.features.plaza') },
  { value: 'albums', label: t('policies.features.albums') },
  { value: 'api_tokens', label: t('policies.features.apiTokens') },
  { value: 'batch_upload', label: t('policies.features.batchUpload') },
  { value: 'paste_upload', label: t('policies.features.pasteUpload') },
  { value: 'drag_upload', label: t('policies.features.dragUpload') },
  { value: 'embed_code', label: t('policies.features.embedCode') },
  { value: 'share', label: t('policies.features.share') },
  { value: 'share_password', label: t('policies.features.sharePassword') },
])

const mimeOptions = ['image/jpeg', 'image/png', 'image/gif', 'image/webp', 'image/avif', 'image/bmp', 'image/tiff']
const formatOptions = ['jpeg', 'png', 'gif', 'webp', 'avif', 'bmp', 'tiff']

/** 常用预设：一键填入某类型的整套设置，仍可在表单中继续微调。 */
const policyPresets = computed<Record<PolicyType, { label: string; settings: Record<string, unknown> }[]>>(() => ({
  quota: [
    { label: t('policies.presets.quota.unlimited'), settings: { quota_mb: 0 } },
    { label: t('policies.presets.quota.basic'), settings: { quota_mb: 1024 } },
    { label: t('policies.presets.quota.advanced'), settings: { quota_mb: 10240 } },
  ],
  upload: [
    {
      label: t('policies.presets.upload.strict'),
      settings: { max_size_mb: 2, allowed_mime_types: ['image/jpeg', 'image/png', 'image/gif', 'image/webp'] },
    },
    {
      label: t('policies.presets.upload.standard'),
      settings: { max_size_mb: 20, allowed_mime_types: ['image/jpeg', 'image/png', 'image/gif', 'image/webp'] },
    },
    {
      label: t('policies.presets.upload.relaxed'),
      settings: { max_size_mb: 100, allowed_mime_types: ['image/jpeg', 'image/png', 'image/gif', 'image/webp', 'image/avif'] },
    },
  ],
  rate: [
    {
      label: t('policies.presets.rate.strict'),
      settings: { upload_per_minute: 10, upload_burst: 2, image_per_minute: 120, image_burst: 30 },
    },
    {
      label: t('policies.presets.rate.standard'),
      settings: { upload_per_minute: 30, upload_burst: 5, image_per_minute: 600, image_burst: 120 },
    },
    {
      label: t('policies.presets.rate.relaxed'),
      settings: { upload_per_minute: 120, upload_burst: 20, image_per_minute: 3000, image_burst: 600 },
    },
  ],
  processing: [
    {
      label: t('policies.presets.processing.lowResource'),
      settings: { enabled: true, max_width: 2048, max_height: 2048, default_quality: 75, allowed_formats: ['jpeg', 'png', 'gif', 'webp'] },
    },
    {
      label: t('policies.presets.processing.standard'),
      settings: { enabled: true, max_width: 4096, max_height: 4096, default_quality: 82, allowed_formats: ['jpeg', 'png', 'gif', 'webp', 'avif'] },
    },
    {
      label: t('policies.presets.processing.highQuality'),
      settings: { enabled: true, max_width: 8192, max_height: 8192, default_quality: 92, allowed_formats: ['jpeg', 'png', 'gif', 'webp', 'avif'] },
    },
  ],
  feature: [
    { label: t('policies.presets.feature.minimal'), settings: { features: ['share'] } },
    {
      label: t('policies.presets.feature.basic'),
      settings: { features: ['plaza', 'albums', 'api_tokens', 'batch_upload', 'paste_upload', 'share'] },
    },
    { label: t('policies.presets.feature.full'), settings: { features: featureOptions.value.map((f) => f.value) } },
  ],
}))

// ---- 设置 JSON 与结构化表单的双向绑定 ----

/** 读取当前 JSON 设置为对象；解析失败时返回空对象。 */
function readSettings(): Record<string, unknown> {
  try {
    const parsed: unknown = JSON.parse(policyForm.settingsText || '{}')
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>
    }
  } catch {
    // 忽略解析错误；结构化控件在 JSON 无效时显示回退值。
  }
  return {}
}

/** 写入单个字段；值为空时删除该字段，从而回退到配置默认值。 */
function writeSetting(key: string, value: unknown): void {
  const settings = readSettings()
  if (value === undefined || value === null || value === '') {
    delete settings[key]
  } else {
    settings[key] = value
  }
  policyForm.settingsText = JSON.stringify(settings, null, 2)
}

/** 生成与某个数值/布尔字段双向绑定的计算属性。 */
function settingModel<T>(key: string, fallback: T) {
  return computed<T>({
    get: () => {
      const value = readSettings()[key]
      return value === undefined || value === null ? fallback : (value as T)
    },
    set: (value) => writeSetting(key, value),
  })
}

/** 生成与某个字符串数组字段双向绑定的计算属性。 */
function listModel(key: string, fallback: string[]) {
  return computed<string[]>({
    get: () => {
      const value = readSettings()[key]
      return Array.isArray(value) ? value.map(String) : fallback
    },
    set: (value) => writeSetting(key, value.length > 0 ? value : undefined),
  })
}

function selectedFeature(): Record<string, unknown> {
  return readSettings()
}

function toggleFeature(name: string, enabled: boolean): void {
  const settings = readSettings()
  const current = Array.isArray(settings.features) ? (settings.features as unknown[]).map(String) : []
  const next = enabled ? Array.from(new Set([...current, name])) : current.filter((f) => f !== name)
  settings.features = next
  policyForm.settingsText = JSON.stringify(settings, null, 2)
}

const selectedFeatures = computed<string[]>(() => {
  const settings = selectedFeature()
  return Array.isArray(settings.features) ? (settings.features as unknown[]).map(String) : []
})

// ---- 角色组对话框 ----
const groupOpen = ref(false)
const groupSaving = ref(false)
const groupFormRef = ref<FormInstance>()
const editingGroupId = ref('')
const groupForm = reactive<RoleGroupInput>({ name: '', description: '', is_default: false })
const groupSnapshot = ref('')

function groupState(): string {
  return JSON.stringify(groupForm)
}

async function closeGroupDialog(done: () => void): Promise<void> {
  if (groupSaving.value) return
  if (groupState() === groupSnapshot.value) {
    done()
    return
  }
  try {
    await ElMessageBox.confirm(t('policies.discard.message'), t('policies.discard.title'), {
      confirmButtonText: t('policies.discard.confirm'),
      cancelButtonText: t('policies.discard.cancel'),
      type: 'warning',
      confirmButtonClass: 'el-button--danger',
    })
    done()
  } catch {
    // 用户选择继续编辑。
  }
}

/** 可预判的删除失败原因：仍被用户或绑定关系引用时直接禁用。 */
function groupDeleteBlockedReason(group: RoleGroup): string {
  if (group.customer_count > 0) {
    return t('policies.blocked.groupUsers', { count: group.customer_count })
  }
  return ''
}

function policyBindings(policyId: string): RoleGroup[] {
  return groups.value.filter((group) => group.policies.some((policy) => policy.id === policyId))
}

function policyDeleteBlockedReason(policy: Policy): string {
  const bound = policyBindings(policy.id)
  if (bound.length > 0) {
    return t('policies.blocked.policyBound', {
      groups: bound.map((group) => group.name).join(t('policies.listSeparator')),
    })
  }
  return ''
}
const groupRules = computed<FormRules>(() => ({
  name: [
    { required: true, message: t('policies.validation.groupNameRequired'), trigger: 'blur' },
    { min: 1, max: 64, message: t('policies.validation.nameLength'), trigger: 'blur' },
  ],
}))

// ---- 策略对话框 ----
const policyOpen = ref(false)
const policySaving = ref(false)
const policyFormRef = ref<FormInstance>()
const editingPolicyId = ref('')
const policyForm = reactive<{ name: string; type: PolicyType; description: string; enabled: boolean; settingsText: string }>({
  name: '',
  type: 'quota',
  description: '',
  enabled: true,
  settingsText: '{}',
})
const policySnapshot = ref('')

function policyState(): string {
  return JSON.stringify(policyForm)
}

async function closePolicyDialog(done: () => void): Promise<void> {
  if (policySaving.value) return
  if (policyState() === policySnapshot.value) {
    done()
    return
  }
  try {
    await ElMessageBox.confirm(t('policies.discard.message'), t('policies.discard.title'), {
      confirmButtonText: t('policies.discard.confirm'),
      cancelButtonText: t('policies.discard.cancel'),
      type: 'warning',
      confirmButtonClass: 'el-button--danger',
    })
    done()
  } catch {
    // 用户选择继续编辑。
  }
}

/** 配置编辑模式：结构化表单或原始 JSON。 */
const settingsMode = ref<'form' | 'json'>('form')

// 结构化字段与 settingsText 双向绑定（读取时回退到模板默认值）。
const quotaMB = settingModel<number>('quota_mb', 1024)
const maxSizeMB = settingModel<number>('max_size_mb', 20)
const allowedMimeTypes = listModel('allowed_mime_types', ['image/jpeg', 'image/png', 'image/gif', 'image/webp'])
const uploadPerMinute = settingModel<number>('upload_per_minute', 30)
const uploadBurst = settingModel<number>('upload_burst', 5)
const imagePerMinute = settingModel<number>('image_per_minute', 600)
const imageBurst = settingModel<number>('image_burst', 120)
const processingEnabled = settingModel<boolean>('enabled', true)
const maxWidth = settingModel<number>('max_width', 4096)
const maxHeight = settingModel<number>('max_height', 4096)
const defaultQuality = settingModel<number>('default_quality', 82)
const allowedFormats = listModel('allowed_formats', ['jpeg', 'png', 'gif', 'webp', 'avif'])
const policyRules = computed<FormRules>(() => ({
  name: [
    { required: true, message: t('policies.validation.policyNameRequired'), trigger: 'blur' },
    { min: 1, max: 64, message: t('policies.validation.nameLength'), trigger: 'blur' },
  ],
  type: [{ required: true, message: t('policies.validation.typeRequired'), trigger: 'change' }],
}))

// ---- 策略绑定对话框 ----
const bindOpen = ref(false)
const bindingGroupId = ref('')
const bindingGroupName = ref('')

const filteredPolicies = (type: PolicyType) => policies.value.filter((p) => p.type === type)

const boundGroup = computed(() => groups.value.find((g) => g.id === bindingGroupId.value) ?? null)

function boundPolicyFor(type: PolicyType): string {
  const group = boundGroup.value
  if (!group) return ''
  return group.policies.find((p) => p.type === type)?.id ?? ''
}

async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    const [groupList, policyList] = await Promise.all([listRoleGroups(), listPolicies()])
    groups.value = groupList ?? []
    policies.value = policyList ?? []
  } catch (error) {
    errorMessage.value = toApiError(error).message
  } finally {
    loading.value = false
  }
}

// ---- 角色组操作 ----
function openCreateGroup(): void {
  editingGroupId.value = ''
  Object.assign(groupForm, { name: '', description: '', is_default: false })
  groupSnapshot.value = groupState()
  groupOpen.value = true
}

function openEditGroup(group: RoleGroup): void {
  editingGroupId.value = group.id
  Object.assign(groupForm, { name: group.name, description: group.description, is_default: group.is_default })
  groupSnapshot.value = groupState()
  groupOpen.value = true
}

async function submitGroup(): Promise<void> {
  const instance = groupFormRef.value
  if (!instance || groupSaving.value) return
  const valid = await instance.validate().catch(() => false)
  if (!valid) return
  groupSaving.value = true
  try {
    const payload: RoleGroupInput = {
      name: groupForm.name.trim(),
      description: groupForm.description.trim(),
      is_default: groupForm.is_default,
    }
    if (editingGroupId.value) {
      await updateRoleGroup(editingGroupId.value, payload)
      ElMessage.success(t('policies.groups.updated'))
    } else {
      await createRoleGroup(payload)
      ElMessage.success(t('policies.groups.created'))
    }
    groupSnapshot.value = groupState()
    groupOpen.value = false
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    groupSaving.value = false
  }
}

async function removeGroup(group: RoleGroup): Promise<void> {
  try {
    await ElMessageBox.confirm(t('policies.groups.deleteConfirm', { name: group.name }), t('policies.groups.deleteTitle'), {
      confirmButtonText: t('common.delete'),
      cancelButtonText: t('common.cancel'),
      type: 'warning',
      confirmButtonClass: 'el-button--danger',
    })
  } catch {
    return
  }
  try {
    await deleteRoleGroup(group.id)
    ElMessage.success(t('policies.groups.deleted'))
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  }
}

// ---- 策略操作 ----
function applyTemplate(type: PolicyType): void {
  policyForm.settingsText = JSON.stringify(typeMeta(type).template, null, 2)
}

function applyPreset(settings: Record<string, unknown>): void {
  policyForm.settingsText = JSON.stringify(settings, null, 2)
}

function openCreatePolicy(): void {
  editingPolicyId.value = ''
  policyForm.name = ''
  policyForm.type = 'quota'
  policyForm.description = ''
  policyForm.enabled = true
  settingsMode.value = 'form'
  applyTemplate('quota')
  policySnapshot.value = policyState()
  policyOpen.value = true
}

function openEditPolicy(policy: Policy): void {
  editingPolicyId.value = policy.id
  policyForm.name = policy.name
  policyForm.type = policy.type
  policyForm.description = policy.description
  policyForm.enabled = policy.enabled
  settingsMode.value = 'form'
  policyForm.settingsText = JSON.stringify(policy.settings ?? {}, null, 2)
  policySnapshot.value = policyState()
  policyOpen.value = true
}

function onPolicyTypeChange(type: PolicyType): void {
  if (!editingPolicyId.value) applyTemplate(type)
}

async function submitPolicy(): Promise<void> {
  const instance = policyFormRef.value
  if (!instance || policySaving.value) return
  const valid = await instance.validate().catch(() => false)
  if (!valid) return
  let settings: Record<string, unknown>
  try {
    const parsed: unknown = JSON.parse(policyForm.settingsText || '{}')
    if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
      throw new Error(t('policies.messages.settingsMustBeObject'))
    }
    settings = parsed as Record<string, unknown>
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : t('policies.messages.settingsInvalid'))
    return
  }
  policySaving.value = true
  try {
    const payload: PolicyInput = {
      name: policyForm.name.trim(),
      type: policyForm.type,
      description: policyForm.description.trim(),
      enabled: policyForm.enabled,
      settings,
    }
    if (editingPolicyId.value) {
      await updatePolicy(editingPolicyId.value, payload)
      ElMessage.success(t('policies.policy.updated'))
    } else {
      await createPolicy(payload)
      ElMessage.success(t('policies.policy.created'))
    }
    policySnapshot.value = policyState()
    policyOpen.value = false
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    policySaving.value = false
  }
}

async function removePolicy(policy: Policy): Promise<void> {
  try {
    await ElMessageBox.confirm(t('policies.policy.deleteConfirm', { name: policy.name }), t('policies.policy.deleteTitle'), {
      confirmButtonText: t('common.delete'),
      cancelButtonText: t('common.cancel'),
      type: 'warning',
      confirmButtonClass: 'el-button--danger',
    })
  } catch {
    return
  }
  try {
    await deletePolicy(policy.id)
    ElMessage.success(t('policies.policy.deleted'))
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  }
}

// ---- 绑定操作 ----
function openBind(group: RoleGroup): void {
  bindingGroupId.value = group.id
  bindingGroupName.value = group.name
  bindOpen.value = true
}

async function onBindChange(type: PolicyType, policyId: string): Promise<void> {
  const group = boundGroup.value
  if (!group) return
  const current = boundPolicyFor(type)
  try {
    if (policyId) {
      await attachPolicy(group.id, policyId)
    } else if (current) {
      await detachPolicy(group.id, current)
    }
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  }
}

onMounted(load)
</script>

<template>
  <div class="ax-page">
    <PageHeader :title="t('policies.page.title')" :description="t('policies.page.description')">
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
        <el-button type="primary" :icon="Plus" @click="openCreateGroup">{{ t('policies.actions.createGroup') }}</el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="load" />

    <div v-else-if="loading && groups.length === 0 && policies.length === 0">
      <TableSkeleton :rows="8" :columns="5" :title="t('policies.loading')" />
    </div>

    <el-tabs v-else v-model="activeTab" class="policy-tabs">
      <el-tab-pane :label="t('policies.tabs.groups', { count: groups.length })" name="groups">
        <EmptyState
          v-if="groups.length === 0"
          :title="t('policies.empty.groups.title')"
          :description="t('policies.empty.groups.description')"
        />
        <div v-else class="ax-card table-card">
          <div class="ax-table-scroll">
            <el-table :data="groups" style="width: 100%">
              <el-table-column :label="t('policies.columns.group')" min-width="220">
                <template #default="{ row }">
                  <div class="group-cell">
                    <span class="group-cell__name">
                      {{ row.name }}
                      <el-tag v-if="row.is_default" size="small" type="primary" effect="plain">{{ t('policies.tags.default') }}</el-tag>
                    </span>
                    <span class="group-cell__desc">{{ row.description || '—' }}</span>
                  </div>
                </template>
              </el-table-column>
              <el-table-column :label="t('policies.columns.policies')" min-width="260">
                <template #default="{ row }">
                  <div class="policy-tags">
                    <el-tag
                      v-for="policy in row.policies"
                      :key="policy.id"
                      size="small"
                      :type="(typeMeta(policy.type).tag as any)"
                      effect="plain"
                    >
                      {{ typeMeta(policy.type).label }} · {{ policy.name }}
                    </el-tag>
                    <span v-if="row.policies.length === 0" class="ax-muted">{{ t('policies.tags.noPolicies') }}</span>
                  </div>
                </template>
              </el-table-column>
              <el-table-column :label="t('policies.columns.users')" width="90" align="right">
                <template #default="{ row }">{{ row.customer_count }}</template>
              </el-table-column>
              <el-table-column :label="t('policies.columns.updatedAt')" min-width="170">
                <template #default="{ row }">{{ formatDateTime(row.updated_at) }}</template>
              </el-table-column>
              <el-table-column :label="t('common.actions')" fixed="right" width="280" align="right">
                <template #default="{ row }">
                  <div class="ax-row-actions">
                    <el-button link type="primary" :icon="Link" @click="openBind(row)">{{ t('policies.actions.bind') }}</el-button>
                    <el-button link type="primary" :icon="EditPen" @click="openEditGroup(row)">{{ t('common.edit') }}</el-button>
                    <el-tooltip
                      :disabled="!groupDeleteBlockedReason(row)"
                      :content="groupDeleteBlockedReason(row)"
                      placement="top"
                    >
                      <span class="action-guard">
                        <el-button
                          link
                          type="danger"
                          :icon="Delete"
                          :disabled="Boolean(groupDeleteBlockedReason(row))"
                          @click="removeGroup(row)"
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
      </el-tab-pane>

      <el-tab-pane :label="t('policies.tabs.policies', { count: policies.length })" name="policies">
        <div class="policy-toolbar">
          <el-button type="primary" :icon="Plus" @click="openCreatePolicy">{{ t('policies.actions.createPolicy') }}</el-button>
        </div>
        <EmptyState
          v-if="policies.length === 0"
          :title="t('policies.empty.policies.title')"
          :description="t('policies.empty.policies.description')"
        />
        <div v-else class="ax-card table-card">
          <div class="ax-table-scroll">
            <el-table :data="policies" style="width: 100%">
              <el-table-column :label="t('policies.columns.policies')" min-width="200">
                <template #default="{ row }">
                  <div class="group-cell">
                    <span class="group-cell__name">{{ row.name }}</span>
                    <span class="group-cell__desc">{{ row.description || '—' }}</span>
                  </div>
                </template>
              </el-table-column>
              <el-table-column :label="t('policies.columns.type')" width="120">
                <template #default="{ row }">
                  <el-tag size="small" :type="(typeMeta(row.type).tag as any)" effect="plain">
                    {{ typeMeta(row.type).label }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column :label="t('common.status')" width="100">
                <template #default="{ row }">
                  <el-tag size="small" :type="row.enabled ? 'success' : 'info'" effect="plain">
                    {{ row.enabled ? t('policies.policyStatus.enabled') : t('policies.policyStatus.disabled') }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column :label="t('policies.columns.updatedAt')" min-width="170">
                <template #default="{ row }">{{ formatDateTime(row.updated_at) }}</template>
              </el-table-column>
              <el-table-column :label="t('common.actions')" fixed="right" width="170" align="right">
                <template #default="{ row }">
                  <div class="ax-row-actions">
                    <el-button link type="primary" :icon="EditPen" @click="openEditPolicy(row)">{{ t('common.edit') }}</el-button>
                    <el-tooltip
                      :disabled="!policyDeleteBlockedReason(row)"
                      :content="policyDeleteBlockedReason(row)"
                      placement="top"
                    >
                      <span class="action-guard">
                        <el-button
                          link
                          type="danger"
                          :icon="Delete"
                          :disabled="Boolean(policyDeleteBlockedReason(row))"
                          @click="removePolicy(row)"
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
      </el-tab-pane>
    </el-tabs>

    <!-- 角色组编辑 -->
    <el-dialog
      v-model="groupOpen"
      :title="editingGroupId ? t('policies.groups.editTitle') : t('policies.groups.createTitle')"
      width="min(480px, 92vw)"
      append-to-body
      :close-on-click-modal="false"
      :before-close="closeGroupDialog"
      @closed="groupFormRef?.clearValidate()"
    >
      <el-form ref="groupFormRef" :model="groupForm" :rules="groupRules" label-position="top" @submit.prevent="submitGroup">
        <el-form-item :label="t('policies.groups.name')" prop="name">
          <el-input v-model="groupForm.name" :placeholder="t('policies.groups.namePlaceholder')" maxlength="64" />
        </el-form-item>
        <el-form-item :label="t('policies.groups.description')">
          <el-input v-model="groupForm.description" type="textarea" :rows="2" maxlength="255" :placeholder="t('policies.groups.descriptionPlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('policies.groups.isDefault')">
          <el-switch v-model="groupForm.is_default" />
          <span class="form-hint">{{ t('policies.groups.defaultHint') }}</span>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="groupOpen = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="groupSaving" @click="submitGroup">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- 策略编辑 -->
    <el-dialog
      v-model="policyOpen"
      :title="editingPolicyId ? t('policies.policy.editTitle') : t('policies.policy.createTitle')"
      width="min(560px, 92vw)"
      append-to-body
      :close-on-click-modal="false"
      :before-close="closePolicyDialog"
      @closed="policyFormRef?.clearValidate()"
    >
      <el-form ref="policyFormRef" :model="policyForm" :rules="policyRules" label-position="top" @submit.prevent="submitPolicy">
        <el-form-item :label="t('policies.policy.name')" prop="name">
          <el-input v-model="policyForm.name" :placeholder="t('policies.policy.namePlaceholder')" maxlength="64" />
        </el-form-item>
        <el-form-item :label="t('policies.policy.type')" prop="type">
          <el-select v-model="policyForm.type" style="width: 100%" @change="onPolicyTypeChange">
            <el-option v-for="item in policyTypes" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('policies.policy.description')">
          <el-input v-model="policyForm.description" type="textarea" :rows="2" maxlength="255" :placeholder="t('policies.policy.descriptionPlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('policies.policy.enabled')">
          <el-switch v-model="policyForm.enabled" />
        </el-form-item>

        <el-form-item :label="t('policies.form.quickPreset')">
          <div class="preset-row">
            <el-button
              v-for="preset in policyPresets[policyForm.type]"
              :key="preset.label"
              size="small"
              @click="applyPreset(preset.settings)"
            >
              {{ preset.label }}
            </el-button>
          </div>
          <div class="form-hint">{{ t('policies.form.presetHint') }}</div>
        </el-form-item>

        <el-form-item :label="t('policies.form.settingsMode')">
          <el-radio-group v-model="settingsMode">
            <el-radio-button value="form">{{ t('policies.form.modeForm') }}</el-radio-button>
            <el-radio-button value="json">{{ t('policies.form.modeJson') }}</el-radio-button>
          </el-radio-group>
        </el-form-item>

        <template v-if="settingsMode === 'form'">
          <div class="form-hint settings-hint">{{ t('policies.form.settingsHint') }}</div>

          <!-- 配额 -->
          <el-form-item v-if="policyForm.type === 'quota'" :label="t('policies.form.quota')">
            <el-input-number v-model="quotaMB" :min="0" :step="128" controls-position="right" style="width: 200px" />
            <span class="form-hint form-hint--inline">{{ t('policies.form.unitHint') }}</span>
          </el-form-item>

          <!-- 上传 -->
          <template v-else-if="policyForm.type === 'upload'">
            <el-form-item :label="t('policies.form.maxSize')">
              <el-input-number v-model="maxSizeMB" :min="0" :step="1" controls-position="right" style="width: 200px" />
              <span class="form-hint form-hint--inline">{{ t('policies.form.unitHint') }}</span>
            </el-form-item>
            <el-form-item :label="t('policies.form.allowedMime')">
              <el-select v-model="allowedMimeTypes" multiple filterable allow-create default-first-option style="width: 100%">
                <el-option v-for="mime in mimeOptions" :key="mime" :label="mime" :value="mime" />
              </el-select>
            </el-form-item>
          </template>

          <!-- 速率 -->
          <div v-else-if="policyForm.type === 'rate'" class="field-grid">
            <el-form-item :label="t('policies.form.uploadPerMinute')">
              <el-input-number v-model="uploadPerMinute" :min="0" controls-position="right" style="width: 100%" />
            </el-form-item>
            <el-form-item :label="t('policies.form.uploadBurst')">
              <el-input-number v-model="uploadBurst" :min="0" controls-position="right" style="width: 100%" />
            </el-form-item>
            <el-form-item :label="t('policies.form.imagePerMinute')">
              <el-input-number v-model="imagePerMinute" :min="0" controls-position="right" style="width: 100%" />
            </el-form-item>
            <el-form-item :label="t('policies.form.imageBurst')">
              <el-input-number v-model="imageBurst" :min="0" controls-position="right" style="width: 100%" />
            </el-form-item>
          </div>

          <!-- 图片处理 -->
          <template v-else-if="policyForm.type === 'processing'">
            <el-form-item :label="t('policies.form.processingEnabled')">
              <el-switch v-model="processingEnabled" />
            </el-form-item>
            <div class="field-grid">
              <el-form-item :label="t('policies.form.maxWidth')">
                <el-input-number v-model="maxWidth" :min="1" :step="256" controls-position="right" style="width: 100%" />
              </el-form-item>
              <el-form-item :label="t('policies.form.maxHeight')">
                <el-input-number v-model="maxHeight" :min="1" :step="256" controls-position="right" style="width: 100%" />
              </el-form-item>
              <el-form-item :label="t('policies.form.defaultQuality')">
                <el-input-number v-model="defaultQuality" :min="1" :max="100" controls-position="right" style="width: 100%" />
              </el-form-item>
            </div>
            <el-form-item :label="t('policies.form.allowedFormats')">
              <el-select v-model="allowedFormats" multiple filterable allow-create default-first-option style="width: 100%">
                <el-option v-for="format in formatOptions" :key="format" :label="format" :value="format" />
              </el-select>
            </el-form-item>
          </template>

          <!-- 功能开关 -->
          <el-form-item v-else-if="policyForm.type === 'feature'" :label="t('policies.form.features')">
            <div class="feature-grid">
              <el-checkbox
                v-for="f in featureOptions"
                :key="f.value"
                :model-value="selectedFeatures.includes(f.value)"
                @change="(value: boolean | string | number) => toggleFeature(f.value, value === true)"
              >
                {{ f.label }}
              </el-checkbox>
            </div>
          </el-form-item>
        </template>

        <el-form-item v-else :label="t('policies.form.settingsJson')">
          <el-input v-model="policyForm.settingsText" type="textarea" :rows="10" spellcheck="false" class="settings-input" />
          <div class="form-hint">
            {{ typeMeta(policyForm.type).hint }}
            <el-button link type="primary" @click="applyTemplate(policyForm.type)">{{ t('policies.actions.applyTemplate') }}</el-button>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="policyOpen = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="policySaving" @click="submitPolicy">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- 策略绑定 -->
    <el-dialog v-model="bindOpen" :title="t('policies.bind.title', { name: bindingGroupName })" width="min(560px, 92vw)" append-to-body>
      <p class="bind-hint">{{ t('policies.bind.hint') }}</p>
      <div v-for="item in policyTypes" :key="item.value" class="bind-row">
        <div class="bind-row__label">
          <el-tag size="small" :type="(item.tag as any)" effect="plain">{{ item.label }}</el-tag>
        </div>
        <el-select
          :model-value="boundPolicyFor(item.value)"
          clearable
          :placeholder="filteredPolicies(item.value).length ? t('policies.bind.unbound') : t('policies.bind.noPolicies')"
          :disabled="filteredPolicies(item.value).length === 0"
          style="flex: 1"
          @update:model-value="(value: string) => onBindChange(item.value, value ?? '')"
        >
          <el-option v-for="p in filteredPolicies(item.value)" :key="p.id" :label="p.name" :value="p.id">
            <span>{{ p.name }}</span>
            <span class="ax-muted" style="margin-left: 8px">{{ p.enabled ? '' : t('policies.bind.disabledSuffix') }}</span>
          </el-option>
        </el-select>
      </div>
      <template #footer>
        <el-button type="primary" @click="bindOpen = false">{{ t('policies.actions.complete') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.table-card {
  overflow: hidden;
}

.policy-toolbar {
  display: flex;
  justify-content: flex-end;
  margin-bottom: var(--ax-space-3);
}

.group-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.group-cell__name {
  display: inline-flex;
  align-items: center;
  gap: var(--ax-space-2);
  color: var(--ax-text);
  font-size: var(--ax-text-sm);
  font-weight: var(--ax-weight-medium);
}

.group-cell__desc {
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.policy-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.form-hint {
  margin-top: 4px;
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
  line-height: 1.5;
}

.settings-input :deep(textarea) {
  font-family: var(--ax-font-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
  font-size: 12px;
}

.bind-hint {
  margin: 0 0 var(--ax-space-3);
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
}

.bind-row {
  display: flex;
  align-items: center;
  gap: var(--ax-space-3);
  margin-bottom: var(--ax-space-3);
}

.bind-row__label {
  width: 80px;
  flex: none;
}

.action-guard {
  display: inline-flex;
}

.feature-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: var(--ax-space-2);
  width: 100%;
}

.preset-row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--ax-space-2);
}

.field-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  column-gap: var(--ax-space-4);
}

.form-hint--inline {
  margin-left: var(--ax-space-2);
}

.settings-hint {
  margin-bottom: var(--ax-space-3);
}
</style>

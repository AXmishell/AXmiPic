<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
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
import { formatDateTime } from '@/utils/format'

type Tab = 'groups' | 'policies'
const activeTab = ref<Tab>('groups')

const groups = ref<RoleGroup[]>([])
const policies = ref<Policy[]>([])
const loading = ref(false)
const errorMessage = ref('')

/** 策略类型元数据：展示名、标签色、配置模板与说明。 */
const policyTypes: { value: PolicyType; label: string; tag: string; template: Record<string, unknown>; hint: string }[] = [
  { value: 'quota', label: '配额', tag: 'primary', template: { quota_mb: 1024 }, hint: 'quota_mb：存储空间上限（MiB，0 表示不限）' },
  {
    value: 'upload',
    label: '上传',
    tag: 'success',
    template: { max_size_mb: 20, allowed_mime_types: ['image/jpeg', 'image/png', 'image/gif', 'image/webp'] },
    hint: 'max_size_mb：单文件上限；allowed_mime_types：允许的媒体类型',
  },
  {
    value: 'rate',
    label: '速率',
    tag: 'warning',
    template: { upload_per_minute: 30, upload_burst: 5, image_per_minute: 600, image_burst: 120 },
    hint: 'upload_per_minute/burst、image_per_minute/burst：每分钟限额与突发',
  },
  {
    value: 'processing',
    label: '图片处理',
    tag: 'info',
    template: {
      enabled: true,
      max_width: 4096,
      max_height: 4096,
      default_quality: 82,
      allowed_formats: ['jpeg', 'png', 'gif', 'webp', 'avif'],
    },
    hint: 'max_width/max_height、default_quality、allowed_formats',
  },
  {
    value: 'feature',
    label: '功能开关',
    tag: 'danger',
    template: { features: ['plaza', 'albums', 'api_tokens', 'batch_upload', 'paste_upload', 'share'] },
    hint: 'features：启用 plaza、albums、api_tokens、batch_upload、paste_upload、drag_upload、embed_code、share、share_password',
  },
]

function typeMeta(type: PolicyType) {
  return policyTypes.find((t) => t.value === type) ?? policyTypes[0]!
}

const featureOptions = [
  { value: 'plaza', label: '图片广场' },
  { value: 'albums', label: '相册' },
  { value: 'api_tokens', label: 'API 令牌' },
  { value: 'batch_upload', label: '批量上传' },
  { value: 'paste_upload', label: '粘贴上传' },
  { value: 'drag_upload', label: '拖拽上传' },
  { value: 'embed_code', label: '嵌入代码' },
  { value: 'share', label: '分享' },
  { value: 'share_password', label: '密码分享' },
]

function selectedFeature(): Record<string, unknown> {
  try {
    const parsed: unknown = JSON.parse(policyForm.settingsText || '{}')
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>
    }
  } catch {
    // 忽略解析错误；复选框在 JSON 无效时不显示选中态。
  }
  return {}
}

function toggleFeature(name: string, enabled: boolean): void {
  const settings = { ...selectedFeature() }
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
const groupRules: FormRules = {
  name: [
    { required: true, message: '请输入角色组名称', trigger: 'blur' },
    { min: 1, max: 64, message: '名称长度为 1 到 64 个字符', trigger: 'blur' },
  ],
}

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
const policyRules: FormRules = {
  name: [
    { required: true, message: '请输入策略名称', trigger: 'blur' },
    { min: 1, max: 64, message: '名称长度为 1 到 64 个字符', trigger: 'blur' },
  ],
  type: [{ required: true, message: '请选择策略类型', trigger: 'change' }],
}

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
  groupOpen.value = true
}

function openEditGroup(group: RoleGroup): void {
  editingGroupId.value = group.id
  Object.assign(groupForm, { name: group.name, description: group.description, is_default: group.is_default })
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
      ElMessage.success('角色组已更新')
    } else {
      await createRoleGroup(payload)
      ElMessage.success('角色组已创建')
    }
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
    await ElMessageBox.confirm(`将删除角色组「${group.name}」，该操作不可恢复。`, '删除角色组', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning',
      confirmButtonClass: 'el-button--danger',
    })
  } catch {
    return
  }
  try {
    await deleteRoleGroup(group.id)
    ElMessage.success('角色组已删除')
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  }
}

// ---- 策略操作 ----
function applyTemplate(type: PolicyType): void {
  policyForm.settingsText = JSON.stringify(typeMeta(type).template, null, 2)
}

function openCreatePolicy(): void {
  editingPolicyId.value = ''
  policyForm.name = ''
  policyForm.type = 'quota'
  policyForm.description = ''
  policyForm.enabled = true
  applyTemplate('quota')
  policyOpen.value = true
}

function openEditPolicy(policy: Policy): void {
  editingPolicyId.value = policy.id
  policyForm.name = policy.name
  policyForm.type = policy.type
  policyForm.description = policy.description
  policyForm.enabled = policy.enabled
  policyForm.settingsText = JSON.stringify(policy.settings ?? {}, null, 2)
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
      throw new Error('设置必须是 JSON 对象')
    }
    settings = parsed as Record<string, unknown>
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '设置 JSON 无效')
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
      ElMessage.success('策略已更新')
    } else {
      await createPolicy(payload)
      ElMessage.success('策略已创建')
    }
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
    await ElMessageBox.confirm(`将删除策略「${policy.name}」，仍被角色组引用时无法删除。`, '删除策略', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning',
      confirmButtonClass: 'el-button--danger',
    })
  } catch {
    return
  }
  try {
    await deletePolicy(policy.id)
    ElMessage.success('策略已删除')
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
    <PageHeader title="角色策略" description="用角色组与多类型策略控制用户的配额、上传、速率、处理与功能权限。">
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
        <el-button type="primary" :icon="Plus" @click="openCreateGroup">新建角色组</el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="load" />

    <div v-else-if="loading && groups.length === 0 && policies.length === 0" class="ax-card" aria-busy="true">
      <div class="ax-card__body"><el-skeleton :rows="6" animated /></div>
    </div>

    <el-tabs v-else v-model="activeTab" class="policy-tabs">
      <el-tab-pane :label="`角色组 (${groups.length})`" name="groups">
        <EmptyState
          v-if="groups.length === 0"
          title="暂无角色组"
          description="新建角色组并为它绑定策略，随后可在用户管理中分配。"
        />
        <div v-else class="ax-card table-card">
          <div class="ax-table-scroll">
            <el-table :data="groups" style="width: 100%">
              <el-table-column label="角色组" min-width="220">
                <template #default="{ row }">
                  <div class="group-cell">
                    <span class="group-cell__name">
                      {{ row.name }}
                      <el-tag v-if="row.is_default" size="small" type="primary" effect="plain">默认</el-tag>
                    </span>
                    <span class="group-cell__desc">{{ row.description || '—' }}</span>
                  </div>
                </template>
              </el-table-column>
              <el-table-column label="策略" min-width="260">
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
                    <span v-if="row.policies.length === 0" class="ax-muted">未绑定策略</span>
                  </div>
                </template>
              </el-table-column>
              <el-table-column label="用户数" width="90" align="right">
                <template #default="{ row }">{{ row.customer_count }}</template>
              </el-table-column>
              <el-table-column label="更新时间" min-width="170">
                <template #default="{ row }">{{ formatDateTime(row.updated_at) }}</template>
              </el-table-column>
              <el-table-column label="操作" width="280" align="right">
                <template #default="{ row }">
                  <div class="ax-row-actions">
                    <el-button link type="primary" :icon="Link" @click="openBind(row)">绑定策略</el-button>
                    <el-button link type="primary" :icon="EditPen" @click="openEditGroup(row)">编辑</el-button>
                    <el-button link type="danger" :icon="Delete" @click="removeGroup(row)">删除</el-button>
                  </div>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </div>
      </el-tab-pane>

      <el-tab-pane :label="`策略 (${policies.length})`" name="policies">
        <div class="policy-toolbar">
          <el-button type="primary" :icon="Plus" @click="openCreatePolicy">新建策略</el-button>
        </div>
        <EmptyState
          v-if="policies.length === 0"
          title="暂无策略"
          description="创建配额、上传、速率、图片处理或功能开关策略。"
        />
        <div v-else class="ax-card table-card">
          <div class="ax-table-scroll">
            <el-table :data="policies" style="width: 100%">
              <el-table-column label="策略" min-width="200">
                <template #default="{ row }">
                  <div class="group-cell">
                    <span class="group-cell__name">{{ row.name }}</span>
                    <span class="group-cell__desc">{{ row.description || '—' }}</span>
                  </div>
                </template>
              </el-table-column>
              <el-table-column label="类型" width="120">
                <template #default="{ row }">
                  <el-tag size="small" :type="(typeMeta(row.type).tag as any)" effect="plain">
                    {{ typeMeta(row.type).label }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column label="状态" width="100">
                <template #default="{ row }">
                  <el-tag size="small" :type="row.enabled ? 'success' : 'info'" effect="plain">
                    {{ row.enabled ? '启用' : '停用' }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column label="更新时间" min-width="170">
                <template #default="{ row }">{{ formatDateTime(row.updated_at) }}</template>
              </el-table-column>
              <el-table-column label="操作" width="170" align="right">
                <template #default="{ row }">
                  <div class="ax-row-actions">
                    <el-button link type="primary" :icon="EditPen" @click="openEditPolicy(row)">编辑</el-button>
                    <el-button link type="danger" :icon="Delete" @click="removePolicy(row)">删除</el-button>
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
      :title="editingGroupId ? '编辑角色组' : '新建角色组'"
      width="min(480px, 92vw)"
      append-to-body
      :close-on-click-modal="false"
      @closed="groupFormRef?.clearValidate()"
    >
      <el-form ref="groupFormRef" :model="groupForm" :rules="groupRules" label-position="top" @submit.prevent="submitGroup">
        <el-form-item label="名称" prop="name">
          <el-input v-model="groupForm.name" placeholder="例如：VIP 用户" maxlength="64" />
        </el-form-item>
        <el-form-item label="简介">
          <el-input v-model="groupForm.description" type="textarea" :rows="2" maxlength="255" placeholder="可选" />
        </el-form-item>
        <el-form-item label="设为默认组">
          <el-switch v-model="groupForm.is_default" />
          <span class="form-hint">新注册用户默认加入该角色组（全库至多一个）。</span>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="groupOpen = false">取消</el-button>
        <el-button type="primary" :loading="groupSaving" @click="submitGroup">保存</el-button>
      </template>
    </el-dialog>

    <!-- 策略编辑 -->
    <el-dialog
      v-model="policyOpen"
      :title="editingPolicyId ? '编辑策略' : '新建策略'"
      width="min(560px, 92vw)"
      append-to-body
      :close-on-click-modal="false"
      @closed="policyFormRef?.clearValidate()"
    >
      <el-form ref="policyFormRef" :model="policyForm" :rules="policyRules" label-position="top" @submit.prevent="submitPolicy">
        <el-form-item label="名称" prop="name">
          <el-input v-model="policyForm.name" placeholder="例如：高配额" maxlength="64" />
        </el-form-item>
        <el-form-item label="类型" prop="type">
          <el-select v-model="policyForm.type" style="width: 100%" @change="onPolicyTypeChange">
            <el-option v-for="t in policyTypes" :key="t.value" :label="t.label" :value="t.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="简介">
          <el-input v-model="policyForm.description" type="textarea" :rows="2" maxlength="255" placeholder="可选" />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="policyForm.enabled" />
        </el-form-item>
        <el-form-item label="设置（JSON）">
          <el-input v-model="policyForm.settingsText" type="textarea" :rows="8" spellcheck="false" class="settings-input" />
          <div class="form-hint">
            {{ typeMeta(policyForm.type).hint }}
            <el-button link type="primary" @click="applyTemplate(policyForm.type)">填入默认模板</el-button>
          </div>
        </el-form-item>
        <el-form-item v-if="policyForm.type === 'feature'" label="功能开关">
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
      </el-form>
      <template #footer>
        <el-button @click="policyOpen = false">取消</el-button>
        <el-button type="primary" :loading="policySaving" @click="submitPolicy">保存</el-button>
      </template>
    </el-dialog>

    <!-- 策略绑定 -->
    <el-dialog v-model="bindOpen" :title="`为「${bindingGroupName}」绑定策略`" width="min(560px, 92vw)" append-to-body>
      <p class="bind-hint">每个类型保留一个策略；同一类型选择新策略会替换旧的绑定。</p>
      <div v-for="t in policyTypes" :key="t.value" class="bind-row">
        <div class="bind-row__label">
          <el-tag size="small" :type="(t.tag as any)" effect="plain">{{ t.label }}</el-tag>
        </div>
        <el-select
          :model-value="boundPolicyFor(t.value)"
          clearable
          :placeholder="filteredPolicies(t.value).length ? '未绑定' : '该类型暂无策略'"
          :disabled="filteredPolicies(t.value).length === 0"
          style="flex: 1"
          @update:model-value="(value: string) => onBindChange(t.value, value ?? '')"
        >
          <el-option v-for="p in filteredPolicies(t.value)" :key="p.id" :label="p.name" :value="p.id">
            <span>{{ p.name }}</span>
            <span class="ax-muted" style="margin-left: 8px">{{ p.enabled ? '' : '（停用）' }}</span>
          </el-option>
        </el-select>
      </div>
      <template #footer>
        <el-button type="primary" @click="bindOpen = false">完成</el-button>
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

.feature-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: var(--ax-space-2);
  width: 100%;
}
</style>

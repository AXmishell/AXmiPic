<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import type { FormInstance, FormRules } from 'element-plus'
import { Delete, EditPen, Plus, Refresh, Search } from '@element-plus/icons-vue'

import {
  createAdmin,
  deleteAdmin,
  deleteCustomer,
  listAdmins,
  listCustomers,
  listRoleGroups,
  updateAdmin,
  updateCustomer,
} from '@/api/admin'
import { toApiError } from '@/api/client'
import type { RoleGroup, User as UserModel, UserUpdate } from '@/api/types'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'
import PageHeader from '@/components/PageHeader.vue'
import TableSkeleton from '@/components/TableSkeleton.vue'
import UserAvatar from '@/components/UserAvatar.vue'
import { useAuthStore } from '@/stores/auth'
import { formatBytes, formatDateTime, usagePercent } from '@/utils/format'
import { useTabQuery } from '@/utils/useTabQuery'
import { USERNAME_PATTERN, usernameRuleMessage, usernameRules } from '@/utils/validate'

const { t } = useI18n()
const auth = useAuthStore()

type Tab = 'customers' | 'admins'
const CUSTOMER_TABS: readonly Tab[] = ['customers', 'admins']
const activeTab = useTabQuery<Tab>('tab', 'customers', CUSTOMER_TABS)

const customers = ref<UserModel[]>([])
const admins = ref<UserModel[]>([])
const roleGroups = ref<RoleGroup[]>([])
const loading = ref(false)
const tableLoading = ref(false)
const errorMessage = ref('')
const search = ref('')
const pendingIds = ref<Set<string>>(new Set())
const selectedIds = ref<string[]>([])
const batchBusy = ref(false)
const customerTableRef = ref<{ clearSelection: () => void } | null>(null)

// 服务端分页与筛选（普通用户标签页）。
const page = ref(1)
const pageSize = ref(20)
const customerTotal = ref(0)
const statusFilter = ref<'' | 'enabled' | 'disabled'>('')
const roleGroupFilter = ref('')
const sort = ref<'newest' | 'oldest' | 'username' | 'usage'>('newest')

// 请求序号与「正在批量清空筛选」标记：清空筛选时抑制各 watcher 的重复请求。
let customerSeq = 0
let resettingFilters = false

const hasCustomerFilters = computed(
  () =>
    search.value.trim() !== '' ||
    statusFilter.value !== '' ||
    roleGroupFilter.value !== '' ||
    sort.value !== 'newest',
)

const sortLabels = computed<Record<'newest' | 'oldest' | 'username' | 'usage', string>>(() => ({
  newest: t('users.sort.newest'),
  oldest: t('users.sort.oldest'),
  username: t('users.sort.username'),
  usage: t('users.sort.usage'),
}))
const sortLabel = computed(() => sortLabels.value[sort.value])

// 编辑用户资料
const editOpen = ref(false)
const editing = ref(false)
const editTarget = ref<UserModel | null>(null)
const editIsAdmin = ref(false)
const editForm = reactive({ username: '', email: '', password: '' })

// 新建管理员对话框
const createOpen = ref(false)
const creating = ref(false)
const createFormRef = ref<FormInstance>()
const createForm = ref({ username: '', password: '', confirmPassword: '' })
const createRules = computed<FormRules>(() => ({
  username: usernameRules(t('validation.usernameRequired')),
  password: [
    { required: true, message: t('users.validation.passwordRequired'), trigger: 'blur' },
    { min: 8, max: 72, message: t('users.validation.passwordLength'), trigger: 'blur' },
  ],
  confirmPassword: [
    { required: true, message: t('validation.passwordConfirmRequired'), trigger: 'blur' },
    {
      validator: (_rule, value: string, callback) => {
        if (value !== createForm.value.password) {
          callback(new Error(t('validation.passwordMismatch')))
          return
        }
        callback()
      },
      trigger: 'blur',
    },
  ],
}))

function filter(list: UserModel[]): UserModel[] {
  const keyword = search.value.trim().toLowerCase()
  if (!keyword) return list
  return list.filter(
    (user) =>
      user.username.toLowerCase().includes(keyword) ||
      (user.email ?? '').toLowerCase().includes(keyword),
  )
}

const filteredAdmins = computed(() => filter(admins.value))
const totalCount = computed(() => customers.value.length + admins.value.length)

function isSelf(user: UserModel): boolean {
  return auth.user?.id === user.id
}

function roleGroupLabel(id?: string): string {
  if (!id) return t('users.roleGroup.default')
  return roleGroups.value.find((group) => group.id === id)?.name ?? t('users.roleGroup.unknown')
}

const detailOpen = ref(false)
const detailUser = ref<UserModel | null>(null)

function openDetail(user: UserModel): void {
  detailUser.value = user
  detailOpen.value = true
}

function customerQuery(): Parameters<typeof listCustomers>[0] {
  return {
    page: page.value,
    page_size: pageSize.value,
    keyword: search.value.trim() || undefined,
    status: statusFilter.value || undefined,
    role_group_id: roleGroupFilter.value || undefined,
    sort: sort.value,
  }
}

async function loadCustomers(): Promise<void> {
  // 搜索防抖、翻页与筛选可能同时触发请求；用序号丢弃过期响应，
  // 避免慢请求覆盖新结果。
  const seq = ++customerSeq
  tableLoading.value = true
  try {
    const data = await listCustomers(customerQuery())
    if (seq !== customerSeq) return
    customers.value = data.items ?? []
    customerTotal.value = data.total ?? 0
    if (customers.value.length === 0 && page.value > 1 && customerTotal.value > 0) {
      page.value -= 1
      await loadCustomers()
    }
  } finally {
    if (seq === customerSeq) tableLoading.value = false
  }
}

async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    const [, adminList, groupList] = await Promise.all([
      loadCustomers(),
      listAdmins(),
      listRoleGroups(),
    ])
    admins.value = adminList ?? []
    roleGroups.value = groupList ?? []
  } catch (error) {
    errorMessage.value = toApiError(error).message
  } finally {
    loading.value = false
  }
}

function onPageChange(value: number): void {
  page.value = value
  void loadCustomers().catch((error) => {
    ElMessage.error(toApiError(error).message)
  })
}

/** 表头排序与下拉排序共用同一个服务端 sort 参数。 */
function onCustomerSortChange(payload: { prop: string; order: string | null }): void {
  if (payload.prop !== 'created_at' || !payload.order) {
    sort.value = 'newest'
    return
  }
  sort.value = payload.order === 'ascending' ? 'oldest' : 'newest'
}

function onSizeChange(value: number): void {
  pageSize.value = value
  page.value = 1
  void loadCustomers().catch((error) => {
    ElMessage.error(toApiError(error).message)
  })
}

let searchTimer: number | undefined
watch(search, () => {
  if (searchTimer !== undefined) window.clearTimeout(searchTimer)
  searchTimer = window.setTimeout(() => {
    searchTimer = undefined
    if (resettingFilters) return
    if (activeTab.value !== 'customers') return
    page.value = 1
    void loadCustomers().catch((error) => {
      ElMessage.error(toApiError(error).message)
    })
  }, 300)
})

watch([statusFilter, roleGroupFilter, sort], () => {
  if (resettingFilters) return
  if (activeTab.value !== 'customers') return
  page.value = 1
  void loadCustomers().catch((error) => {
    ElMessage.error(toApiError(error).message)
  })
})

/** 清空全部筛选并只触发一次请求。 */
async function clearFilters(): Promise<void> {
  resettingFilters = true
  search.value = ''
  statusFilter.value = ''
  roleGroupFilter.value = ''
  sort.value = 'newest'
  page.value = 1
  // 等待 watcher 刷新后再取消搜索防抖，避免 300ms 后补发一次请求。
  await nextTick()
  if (searchTimer !== undefined) {
    window.clearTimeout(searchTimer)
    searchTimer = undefined
  }
  resettingFilters = false
  try {
    await loadCustomers()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  }
}

function openCreate(): void {
  createForm.value = { username: '', password: '', confirmPassword: '' }
  createOpen.value = true
}

function openEdit(user: UserModel, isAdmin: boolean): void {
  editTarget.value = user
  editIsAdmin.value = isAdmin
  editForm.username = user.username
  editForm.email = user.email ?? ''
  editForm.password = ''
  editOpen.value = true
}

async function submitEdit(): Promise<void> {
  const target = editTarget.value
  if (!target || editing.value) return
  const username = editForm.username.trim()
  if (!USERNAME_PATTERN.test(username)) {
    ElMessage.warning(usernameRuleMessage())
    return
  }
  if (editForm.password && editForm.password.length < 8) {
    ElMessage.warning(t('users.messages.newPasswordMin'))
    return
  }
  const payload: UserUpdate = {
    username,
    email: editForm.email.trim().toLowerCase(),
  }
  if (editForm.password) payload.password = editForm.password

  editing.value = true
  try {
    const updated = editIsAdmin.value
      ? await updateAdmin(target.id, payload)
      : await updateCustomer(target.id, payload)
    const list = editIsAdmin.value ? admins : customers
    list.value = list.value.map((user) => (user.id === updated.id ? updated : user))
    if (isSelf(target)) await auth.refreshUser()
    editOpen.value = false
    ElMessage.success(t('users.messages.updated'))
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    editing.value = false
  }
}

async function submitCreate(): Promise<void> {
  const instance = createFormRef.value
  if (!instance || creating.value) return
  const valid = await instance.validate().catch(() => false)
  if (!valid) return
  creating.value = true
  try {
    await createAdmin({
      username: createForm.value.username.trim(),
      password: createForm.value.password,
    })
    ElMessage.success(t('users.messages.adminCreated'))
    createOpen.value = false
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    creating.value = false
  }
}

async function patch(
  target: UserModel,
  payload: UserUpdate,
  run: (id: string, body: UserUpdate) => Promise<UserModel>,
  list: typeof customers,
): Promise<void> {
  pendingIds.value.add(target.id)
  try {
    const updated = await run(target.id, payload)
    list.value = list.value.map((user) => (user.id === updated.id ? updated : user))
    ElMessage.success(t('users.messages.updated'))
    // 状态/角色组筛选生效时，变更后需重新取当前页，避免残留不匹配的行。
    if (list === customers && (statusFilter.value || roleGroupFilter.value)) {
      await loadCustomers()
    }
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    pendingIds.value.delete(target.id)
  }
}

function onDisabledChange(target: UserModel, value: unknown, isAdmin: boolean): void {
  // el-switch 的 model-value 是「账号启用」状态：true=正常，false=禁用。
  // 账户字段 disabled 与它相反，必须取反，否则关闭开关会发出 disabled=false。
  const enabled = value === true
  void applyDisabled(target, !enabled, isAdmin)
}

async function applyDisabled(target: UserModel, disabled: boolean, isAdmin: boolean): Promise<void> {
  if (disabled) {
    try {
      await ElMessageBox.confirm(
        t('users.confirm.disableMessage', { name: target.username }),
        t('users.confirm.disableTitle'),
        {
          confirmButtonText: t('users.confirm.disable'),
          cancelButtonText: t('common.cancel'),
          type: 'warning',
          confirmButtonClass: 'el-button--danger',
        },
      )
    } catch {
      return
    }
  }
  const run = isAdmin ? updateAdmin : updateCustomer
  await patch(target, { disabled }, run, isAdmin ? admins : customers)
}

async function onRoleGroupChange(target: UserModel, value: unknown): Promise<void> {
  const roleGroupID = typeof value === 'string' ? value : ''
  if (roleGroupID === (target.role_group_id ?? '')) return
  try {
    await ElMessageBox.confirm(
      t('users.confirm.roleGroupMessage', {
        name: target.username,
        role: roleGroupLabel(roleGroupID),
      }),
      t('users.confirm.roleGroupTitle'),
      {
        confirmButtonText: t('users.confirm.roleGroup'),
        cancelButtonText: t('common.cancel'),
        type: 'warning',
      },
    )
  } catch {
    return
  }
  await patch(target, { role_group_id: roleGroupID }, updateCustomer, customers)
}

function onCustomerSelectionChange(rows: UserModel[]): void {
  selectedIds.value = rows.map((row) => row.id)
}

function clearCustomerSelection(): void {
  customerTableRef.value?.clearSelection()
  selectedIds.value = []
}

/** 逐条执行批量操作；失败不中断，结束后汇总成功与首条失败原因。 */
async function runBatch(
  ids: string[],
  run: (id: string) => Promise<unknown>,
  label: string,
): Promise<void> {
  batchBusy.value = true
  const errors: string[] = []
  for (const id of ids) {
    try {
      await run(id)
    } catch (error) {
      errors.push(toApiError(error).message)
    }
  }
  batchBusy.value = false
  clearCustomerSelection()
  try {
    await loadCustomers()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  }
  if (errors.length === 0) {
    ElMessage.success(t('users.batch.success', { label, count: ids.length }))
  } else {
    ElMessage.error(
      t('users.batch.partial', {
        label,
        success: ids.length - errors.length,
        failed: errors.length,
        reason: errors[0] ?? '',
      }),
    )
  }
}

async function batchDisable(): Promise<void> {
  const ids = [...selectedIds.value]
  if (ids.length === 0 || batchBusy.value) return
  try {
    await ElMessageBox.confirm(
      t('users.confirm.batchDisableMessage', { count: ids.length }),
      t('users.confirm.batchDisableTitle'),
      {
        confirmButtonText: t('users.confirm.batchDisable'),
        cancelButtonText: t('common.cancel'),
        type: 'warning',
      },
    )
  } catch {
    return
  }
  await runBatch(ids, (id) => updateCustomer(id, { disabled: true }), t('users.batch.disableLabel'))
}

async function batchRemove(): Promise<void> {
  const ids = [...selectedIds.value]
  if (ids.length === 0 || batchBusy.value) return
  try {
    await ElMessageBox.prompt(
      t('users.confirm.batchRemoveMessage', { count: ids.length }),
      t('users.confirm.batchRemoveTitle'),
      {
        confirmButtonText: t('users.confirm.batchRemove'),
        cancelButtonText: t('common.cancel'),
        type: 'warning',
        inputPlaceholder: 'DELETE',
        inputValidator: (value) => value === 'DELETE' || t('users.confirm.deleteKeywordHint'),
        confirmButtonClass: 'el-button--danger',
      },
    )
  } catch {
    return
  }
  await runBatch(ids, (id) => deleteCustomer(id), t('users.batch.removeLabel'))
}

async function removeUser(target: UserModel, isAdmin: boolean): Promise<void> {
  try {
    await ElMessageBox.prompt(
      t('users.confirm.deleteMessage', { name: target.username }),
      isAdmin ? t('users.confirm.deleteAdminTitle') : t('users.confirm.deleteUserTitle'),
      {
        confirmButtonText: t('common.delete'),
        cancelButtonText: t('common.cancel'),
        type: 'warning',
        inputPlaceholder: target.username,
        inputValidator: (value) => value === target.username || t('users.confirm.usernameMismatch'),
        confirmButtonClass: 'el-button--danger',
      },
    )
  } catch {
    return
  }
  pendingIds.value.add(target.id)
  try {
    if (isAdmin) {
      await deleteAdmin(target.id)
      admins.value = admins.value.filter((user) => user.id !== target.id)
    } else {
      await deleteCustomer(target.id)
      if (customers.value.length === 1 && page.value > 1) page.value -= 1
      await loadCustomers()
    }
    ElMessage.success(t('users.messages.deleted'))
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    pendingIds.value.delete(target.id)
  }
}

onMounted(load)
</script>

<template>
  <div class="ax-page">
    <PageHeader :title="t('users.title')" :description="t('users.description')">
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
        <el-button
          v-if="activeTab === 'admins'"
          type="primary"
          :icon="Plus"
          @click="openCreate"
        >
          {{ t('users.actions.createAdmin') }}
        </el-button>
      </template>
      <template #toolbar>
        <el-input
          v-model="search"
          class="user-search"
          :placeholder="t('users.searchPlaceholder')"
          clearable
          :prefix-icon="Search"
        />
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="load" />

    <div v-else-if="loading && totalCount === 0">
      <TableSkeleton :rows="8" :columns="6" :title="t('users.loading')" />
    </div>

    <el-tabs v-else v-model="activeTab" class="user-tabs">
      <el-tab-pane :label="t('users.tabs.customers', { count: customerTotal })" name="customers">
        <div class="user-toolbar">
          <el-select
            v-model="statusFilter"
            :placeholder="t('users.filters.statusPlaceholder')"
            clearable
            class="user-toolbar__select"
          >
            <el-option :label="t('users.status.enabled')" value="enabled" />
            <el-option :label="t('users.status.disabled')" value="disabled" />
          </el-select>
          <el-select
            v-model="roleGroupFilter"
            :placeholder="t('users.filters.roleGroupPlaceholder')"
            clearable
            class="user-toolbar__select"
          >
            <el-option
              v-for="group in roleGroups"
              :key="group.id"
              :label="group.name"
              :value="group.id"
            />
          </el-select>
          <el-select v-model="sort" class="user-toolbar__sort">
            <el-option :label="t('users.sort.newest')" value="newest" />
            <el-option :label="t('users.sort.oldest')" value="oldest" />
            <el-option :label="t('users.sort.username')" value="username" />
            <el-option :label="t('users.sort.usage')" value="usage" />
          </el-select>
          <span class="user-toolbar__total">{{ t('users.total', { count: customerTotal }) }}</span>
        </div>

        <div v-if="hasCustomerFilters" class="user-filter-tags">
          <span class="user-filter-tags__label">{{ t('users.filterTags.label') }}</span>
          <el-tag v-if="search.trim()" type="info" closable @close="search = ''">
            {{ t('users.filterTags.keyword', { keyword: search.trim() }) }}
          </el-tag>
          <el-tag v-if="statusFilter" type="info" closable @close="statusFilter = ''">
            {{ statusFilter === 'enabled' ? t('users.status.enabled') : t('users.status.disabled') }}
          </el-tag>
          <el-tag v-if="roleGroupFilter" type="info" closable @close="roleGroupFilter = ''">
            {{ t('users.filterTags.roleGroup', { name: roleGroupLabel(roleGroupFilter) }) }}
          </el-tag>
          <el-tag v-if="sort !== 'newest'" type="info" closable @close="sort = 'newest'">
            {{ t('users.sort.tag', { label: sortLabel }) }}
          </el-tag>
          <el-button link type="primary" @click="clearFilters">{{ t('users.filterTags.clearAll') }}</el-button>
        </div>

        <div v-if="selectedIds.length > 0" class="user-selection-bar">
          <span class="user-selection-bar__text">
            {{ t('users.selection.count', { count: selectedIds.length }) }}
          </span>
          <div class="user-selection-bar__actions">
            <el-button size="small" :loading="batchBusy" @click="batchDisable">
              {{ t('users.selection.disable') }}
            </el-button>
            <el-button size="small" type="danger" plain :loading="batchBusy" @click="batchRemove">
              {{ t('users.selection.remove') }}
            </el-button>
            <el-button size="small" text :disabled="batchBusy" @click="clearCustomerSelection">
              {{ t('users.selection.clear') }}
            </el-button>
          </div>
        </div>

        <EmptyState
          v-if="customers.length === 0"
          :title="
            hasCustomerFilters ? t('users.empty.customers.filteredTitle') : t('users.empty.customers.title')
          "
          :description="
            hasCustomerFilters
              ? t('users.empty.customers.filteredDescription')
              : t('users.empty.customers.description')
          "
        >
          <el-button v-if="hasCustomerFilters" @click="clearFilters">
            {{ t('users.empty.customers.clearFilters') }}
          </el-button>
        </EmptyState>
        <div v-else class="ax-card table-card">
          <div class="ax-table-scroll">
            <el-table
              ref="customerTableRef"
              v-loading="tableLoading"
              :data="customers"
              row-key="id"
              style="width: 100%"
              @selection-change="onCustomerSelectionChange"
              @sort-change="onCustomerSortChange"
            >
              <el-table-column type="selection" width="44" />
              <el-table-column :label="t('users.columns.user')" min-width="200">
                <template #default="{ row }">
                  <div class="user-cell">
                    <UserAvatar :name="row.username" size="sm" />
                    <div class="user-cell__meta">
                      <span class="user-cell__name">
                        {{ row.username }}
                        <el-tag v-if="isSelf(row)" size="small" type="primary" effect="plain">
                          {{ t('users.currentAccount') }}
                        </el-tag>
                      </span>
                      <span class="user-cell__id">ID {{ row.id }}</span>
                    </div>
                  </div>
                </template>
              </el-table-column>
              <el-table-column :label="t('users.columns.usage')" min-width="200">
                <template #default="{ row }">
                  <div class="user-usage">
                    <span class="user-usage__text">
                      {{ formatBytes(row.used_bytes) }}
                      <span class="ax-muted">
                        /
                        {{ row.quota_bytes > 0 ? formatBytes(row.quota_bytes) : t('quotaMeter.unlimited') }}
                      </span>
                    </span>
                    <div
                      v-if="row.quota_bytes > 0"
                      class="user-usage__track"
                      role="progressbar"
                      :aria-valuenow="usagePercent(row.used_bytes, row.quota_bytes)"
                      aria-valuemin="0"
                      aria-valuemax="100"
                    >
                      <div
                        class="user-usage__fill"
                        :style="{
                          transform: `scaleX(${usagePercent(row.used_bytes, row.quota_bytes) / 100})`,
                        }"
                      />
                    </div>
                  </div>
                </template>
              </el-table-column>
              <el-table-column :label="t('users.columns.status')" width="150">
                <template #default="{ row }">
                  <div class="user-status">
                    <el-switch
                      :model-value="!row.disabled"
                      :loading="pendingIds.has(row.id)"
                      :aria-label="t('users.status.aria', { name: row.username })"
                      @change="onDisabledChange(row, $event, false)"
                    />
                    <span :class="row.disabled ? 'user-status__off' : 'user-status__on'">
                      {{ row.disabled ? t('users.status.disabled') : t('users.status.enabled') }}
                    </span>
                  </div>
                </template>
              </el-table-column>
              <el-table-column :label="t('users.columns.roleGroup')" min-width="170">
                <template #default="{ row }">
                  <el-select
                    :model-value="row.role_group_id ?? ''"
                    :loading="pendingIds.has(row.id)"
                    :aria-label="t('users.roleGroup.aria', { name: row.username })"
                    style="width: 100%"
                    @change="onRoleGroupChange(row, $event)"
                  >
                    <el-option :label="t('users.roleGroup.default')" value="" />
                    <el-option
                      v-for="group in roleGroups"
                      :key="group.id"
                      :label="group.name"
                      :value="group.id"
                    />
                  </el-select>
                </template>
              </el-table-column>
              <el-table-column
                prop="created_at"
                :label="t('users.columns.createdAt')"
                min-width="170"
                sortable="custom"
              >
                <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
              </el-table-column>
              <el-table-column
                :label="t('users.columns.actions')"
                fixed="right"
                width="230"
                align="right"
              >
                <template #default="{ row }">
                  <div class="ax-row-actions">
                    <el-button link type="primary" :icon="EditPen" @click="openEdit(row, false)">
                      {{ t('common.edit') }}
                    </el-button>
                    <el-button link type="primary" @click="openDetail(row)">
                      {{ t('users.actions.detail') }}
                    </el-button>
                    <el-button
                      link
                      type="danger"
                      :icon="Delete"
                      :loading="pendingIds.has(row.id)"
                      @click="removeUser(row, false)"
                    >
                      {{ t('common.delete') }}
                    </el-button>
                  </div>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </div>
        <div v-if="customerTotal > 0" class="user-pagination">
          <el-pagination
            background
            layout="sizes, prev, pager, next, jumper"
            :total="customerTotal"
            :current-page="page"
            :page-size="pageSize"
            :page-sizes="[10, 20, 50, 100]"
            @current-change="onPageChange"
            @size-change="onSizeChange"
          />
        </div>
      </el-tab-pane>

      <el-tab-pane :label="t('users.tabs.admins', { count: filteredAdmins.length })" name="admins">
        <EmptyState
          v-if="filteredAdmins.length === 0"
          :title="
            search.trim() ? t('users.empty.admins.filteredTitle') : t('users.empty.admins.title')
          "
          :description="
            search.trim()
              ? t('users.empty.admins.filteredDescription')
              : t('users.empty.admins.description')
          "
        >
          <el-button v-if="search.trim()" @click="search = ''">
            {{ t('users.empty.admins.clearSearch') }}
          </el-button>
        </EmptyState>
        <div v-else class="ax-card table-card">
          <div class="ax-table-scroll">
            <el-table :data="filteredAdmins" style="width: 100%">
              <el-table-column :label="t('users.columns.admin')" min-width="220">
                <template #default="{ row }">
                  <div class="user-cell">
                    <UserAvatar :name="row.username" size="sm" />
                    <div class="user-cell__meta">
                      <span class="user-cell__name">
                        {{ row.username }}
                        <el-tag size="small" type="primary" effect="plain">
                          {{ t('topbar.roleAdmin') }}
                        </el-tag>
                        <el-tag v-if="isSelf(row)" size="small" type="info" effect="plain">
                          {{ t('users.currentAccount') }}
                        </el-tag>
                      </span>
                      <span class="user-cell__id">ID {{ row.id }}</span>
                    </div>
                  </div>
                </template>
              </el-table-column>
              <el-table-column :label="t('users.columns.status')" width="150">
                <template #default="{ row }">
                  <div
                    class="user-status"
                    :title="isSelf(row) ? t('users.disableSelfHint') : undefined"
                  >
                    <el-switch
                      :model-value="!row.disabled"
                      :loading="pendingIds.has(row.id)"
                      :disabled="isSelf(row)"
                      :aria-label="t('users.status.aria', { name: row.username })"
                      @change="onDisabledChange(row, $event, true)"
                    />
                    <span :class="row.disabled ? 'user-status__off' : 'user-status__on'">
                      {{ row.disabled ? t('users.status.disabled') : t('users.status.enabled') }}
                    </span>
                  </div>
                </template>
              </el-table-column>
              <el-table-column :label="t('users.columns.created')" min-width="170">
                <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
              </el-table-column>
              <el-table-column
                :label="t('users.columns.actions')"
                fixed="right"
                width="200"
                align="right"
              >
                <template #default="{ row }">
                  <div class="ax-row-actions">
                    <el-button link type="primary" :icon="EditPen" @click="openEdit(row, true)">
                      {{ t('common.edit') }}
                    </el-button>
                    <el-button
                      link
                      type="danger"
                      :icon="Delete"
                      :disabled="isSelf(row)"
                      :loading="pendingIds.has(row.id)"
                      @click="removeUser(row, true)"
                    >
                      {{ t('common.delete') }}
                    </el-button>
                  </div>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </div>
      </el-tab-pane>
    </el-tabs>

    <el-dialog
      v-model="editOpen"
      :title="editIsAdmin ? t('users.dialogs.edit.titleAdmin') : t('users.dialogs.edit.titleUser')"
      width="min(440px, 92vw)"
      append-to-body
      :close-on-click-modal="false"
    >
      <el-form label-position="top" @submit.prevent="submitEdit">
        <el-form-item :label="t('users.dialogs.edit.username')">
          <el-input
            v-model="editForm.username"
            maxlength="64"
            :placeholder="t('users.dialogs.edit.usernamePlaceholder')"
          />
        </el-form-item>
        <el-form-item :label="t('users.dialogs.edit.email')">
          <el-input
            v-model="editForm.email"
            :placeholder="t('users.dialogs.edit.emailPlaceholder')"
            autocomplete="email"
          />
          <p class="user-edit__hint">{{ t('users.dialogs.edit.emailHint') }}</p>
        </el-form-item>
        <el-form-item :label="t('users.dialogs.edit.newPassword')">
          <el-input
            v-model="editForm.password"
            type="password"
            show-password
            maxlength="72"
            :placeholder="t('users.dialogs.edit.newPasswordPlaceholder')"
            autocomplete="new-password"
          />
          <p class="user-edit__hint">{{ t('users.dialogs.edit.passwordHint') }}</p>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editOpen = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="editing" @click="submitEdit">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="createOpen"
      :title="t('users.dialogs.create.title')"
      width="min(440px, 92vw)"
      append-to-body
      :close-on-click-modal="false"
      @closed="createFormRef?.clearValidate()"
    >
      <el-form
        ref="createFormRef"
        :model="createForm"
        :rules="createRules"
        label-position="top"
        @submit.prevent="submitCreate"
      >
        <el-form-item :label="t('users.dialogs.create.username')" prop="username">
          <el-input
            v-model="createForm.username"
            :placeholder="t('users.dialogs.create.usernamePlaceholder')"
            maxlength="64"
          />
        </el-form-item>
        <el-form-item :label="t('users.dialogs.create.password')" prop="password">
          <el-input
            v-model="createForm.password"
            type="password"
            show-password
            :placeholder="t('users.dialogs.create.passwordPlaceholder')"
            maxlength="72"
          />
        </el-form-item>
        <el-form-item :label="t('users.dialogs.create.confirmPassword')" prop="confirmPassword">
          <el-input
            v-model="createForm.confirmPassword"
            type="password"
            show-password
            :placeholder="t('users.dialogs.create.confirmPasswordPlaceholder')"
            maxlength="72"
            @keyup.enter="submitCreate"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createOpen = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="creating" @click="submitCreate">
          {{ t('users.dialogs.create.submit') }}
        </el-button>
      </template>
    </el-dialog>

    <el-drawer
      v-model="detailOpen"
      :title="t('users.dialogs.detail.title')"
      size="min(420px, 92vw)"
      append-to-body
    >
      <template v-if="detailUser">
        <div class="user-detail__identity">
          <UserAvatar :name="detailUser.username" size="lg" />
          <div class="user-detail__meta">
            <p class="user-detail__name">{{ detailUser.username }}</p>
            <div class="ax-cluster">
              <el-tag size="small" :type="detailUser.disabled ? 'info' : 'success'" effect="plain">
                {{ detailUser.disabled ? t('users.status.disabled') : t('users.status.enabled') }}
              </el-tag>
              <el-tag v-if="isSelf(detailUser)" size="small" type="primary" effect="plain">
                {{ t('users.currentAccount') }}
              </el-tag>
            </div>
          </div>
        </div>

        <dl class="user-detail__list">
          <div class="user-detail__row">
            <dt>{{ t('users.dialogs.detail.userId') }}</dt>
            <dd class="ax-mono">{{ detailUser.id }}</dd>
          </div>
          <div class="user-detail__row">
            <dt>{{ t('users.dialogs.detail.email') }}</dt>
            <dd>{{ detailUser.email || t('users.dialogs.detail.emailUnbound') }}</dd>
          </div>
          <div class="user-detail__row">
            <dt>{{ t('users.dialogs.detail.roleGroup') }}</dt>
            <dd>{{ roleGroupLabel(detailUser.role_group_id) }}</dd>
          </div>
          <div class="user-detail__row">
            <dt>{{ t('users.dialogs.detail.usage') }}</dt>
            <dd>
              {{ formatBytes(detailUser.used_bytes) }} /
              {{
                detailUser.quota_bytes > 0 ? formatBytes(detailUser.quota_bytes) : t('quotaMeter.unlimited')
              }}
            </dd>
          </div>
          <div class="user-detail__row">
            <dt>{{ t('users.dialogs.detail.planExpires') }}</dt>
            <dd>
              {{
                detailUser.plan_expires_at
                  ? formatDateTime(detailUser.plan_expires_at)
                  : t('users.dialogs.detail.planNone')
              }}
            </dd>
          </div>
          <div class="user-detail__row">
            <dt>{{ t('users.dialogs.detail.totp') }}</dt>
            <dd>
              {{
                detailUser.totp_enabled
                  ? t('users.dialogs.detail.totpEnabled')
                  : t('users.dialogs.detail.totpDisabled')
              }}
            </dd>
          </div>
          <div class="user-detail__row">
            <dt>{{ t('users.dialogs.detail.createdAt') }}</dt>
            <dd>{{ formatDateTime(detailUser.created_at) }}</dd>
          </div>
        </dl>

        <div class="user-detail__actions">
          <el-button @click="detailOpen = false">{{ t('common.close') }}</el-button>
        </div>
      </template>
    </el-drawer>
  </div>
</template>

<style scoped>
.table-card {
  overflow: hidden;
}

.user-search {
  width: 220px;
}

.user-edit__hint {
  margin: 6px 0 0;
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.user-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--ax-space-2);
  margin-bottom: var(--ax-space-3);
}

.user-toolbar__select {
  width: 150px;
}

.user-toolbar__sort {
  width: 190px;
}

.user-toolbar__total {
  margin-left: auto;
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.user-pagination {
  display: flex;
  justify-content: flex-end;
  margin-top: var(--ax-space-3);
}

.user-selection-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--ax-space-3);
  margin-bottom: var(--ax-space-3);
  padding: var(--ax-space-2) var(--ax-space-4);
  background: var(--ax-accent-soft);
  border: 1px solid var(--ax-accent-ring);
  border-radius: var(--ax-radius-md);
}

.user-selection-bar__text {
  color: var(--ax-text);
  font-size: var(--ax-text-sm);
  font-weight: var(--ax-weight-medium);
}

.user-filter-tags {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--ax-space-2);
  margin-bottom: var(--ax-space-3);
}

.user-filter-tags__label {
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.user-selection-bar__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--ax-space-2);
}

.user-cell {
  display: flex;
  align-items: center;
  gap: var(--ax-space-3);
  min-width: 0;
}

.user-cell__meta {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.user-cell__name {
  display: inline-flex;
  align-items: center;
  gap: var(--ax-space-2);
  overflow: hidden;
  color: var(--ax-text);
  font-size: var(--ax-text-sm);
  font-weight: var(--ax-weight-medium);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.user-cell__id {
  color: var(--ax-text-4);
  font-size: var(--ax-text-2xs);
  font-variant-numeric: tabular-nums;
}

.user-usage {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 140px;
}

.user-usage__text {
  color: var(--ax-text-2);
  font-size: var(--ax-text-sm);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.user-usage__track {
  height: 3px;
  overflow: hidden;
  background: var(--ax-tint-strong);
  border-radius: var(--ax-radius-full);
}

.user-usage__fill {
  width: 100%;
  height: 100%;
  background: var(--ax-accent);
  border-radius: inherit;
  transform-origin: left center;
}

.user-status {
  display: flex;
  align-items: center;
  gap: var(--ax-space-2);
}

.user-status__on {
  color: var(--ax-success);
  font-size: var(--ax-text-xs);
}

.user-status__off {
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.user-detail__identity {
  display: flex;
  align-items: center;
  gap: var(--ax-space-3);
}

.user-detail__meta {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-2);
  min-width: 0;
}

.user-detail__name {
  color: var(--ax-text);
  font-size: var(--ax-text-md);
  font-weight: var(--ax-weight-semibold);
  overflow-wrap: anywhere;
}

.user-detail__list {
  display: flex;
  flex-direction: column;
  margin: var(--ax-space-5) 0 0;
}

.user-detail__row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--ax-space-4);
  padding: var(--ax-space-3) 0;
  border-bottom: 1px solid var(--ax-border-subtle);
}

.user-detail__row:last-child {
  border-bottom: none;
}

.user-detail__row dt {
  flex: none;
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
}

.user-detail__row dd {
  margin: 0;
  min-width: 0;
  color: var(--ax-text-2);
  font-size: var(--ax-text-sm);
  text-align: right;
  overflow-wrap: anywhere;
}

.user-detail__actions {
  display: flex;
  justify-content: flex-end;
  margin-top: var(--ax-space-5);
}

@media (max-width: 768px) {
  .user-search {
    width: 100%;
  }
}
</style>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { Plus, Refresh, Search } from '@element-plus/icons-vue'

import {
  createAdmin,
  deleteAdmin,
  deleteCustomer,
  listAdmins,
  listCustomers,
  updateAdmin,
  updateCustomer,
} from '@/api/admin'
import { toApiError } from '@/api/client'
import type { User as UserModel, UserUpdate } from '@/api/types'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'
import PageHeader from '@/components/PageHeader.vue'
import UserAvatar from '@/components/UserAvatar.vue'
import { useAuthStore } from '@/stores/auth'
import { formatBytes, formatDateTime, usagePercent } from '@/utils/format'

const auth = useAuthStore()

type Tab = 'customers' | 'admins'
const activeTab = ref<Tab>('customers')

const customers = ref<UserModel[]>([])
const admins = ref<UserModel[]>([])
const loading = ref(false)
const errorMessage = ref('')
const search = ref('')
const pendingIds = ref<Set<string>>(new Set())

// 新建管理员对话框
const createOpen = ref(false)
const creating = ref(false)
const createFormRef = ref<FormInstance>()
const createForm = ref({ username: '', password: '' })
const createRules: FormRules = {
  username: [
    { required: true, message: '请输入用户名', trigger: 'blur' },
    { min: 3, max: 64, message: '用户名长度为 3 到 64 个字符', trigger: 'blur' },
  ],
  password: [
    { required: true, message: '请输入密码', trigger: 'blur' },
    { min: 8, max: 72, message: '密码长度为 8 到 72 个字符', trigger: 'blur' },
  ],
}

function filter(list: UserModel[]): UserModel[] {
  const keyword = search.value.trim().toLowerCase()
  if (!keyword) return list
  return list.filter((user) => user.username.toLowerCase().includes(keyword))
}

const filteredCustomers = computed(() => filter(customers.value))
const filteredAdmins = computed(() => filter(admins.value))
const totalCount = computed(() => customers.value.length + admins.value.length)

function isSelf(user: UserModel): boolean {
  return auth.user?.id === user.id
}

async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    const [customerList, adminList] = await Promise.all([listCustomers(), listAdmins()])
    customers.value = customerList ?? []
    admins.value = adminList ?? []
  } catch (error) {
    errorMessage.value = toApiError(error).message
  } finally {
    loading.value = false
  }
}

function openCreate(): void {
  createForm.value = { username: '', password: '' }
  createOpen.value = true
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
    ElMessage.success('管理员已创建')
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
    ElMessage.success('用户信息已更新')
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    pendingIds.value.delete(target.id)
  }
}

function onDisabledChange(target: UserModel, value: unknown, isAdmin: boolean): void {
  const disabled = value === true
  void applyDisabled(target, disabled, isAdmin)
}

async function applyDisabled(target: UserModel, disabled: boolean, isAdmin: boolean): Promise<void> {
  if (disabled) {
    try {
      await ElMessageBox.confirm(
        `禁用后「${target.username}」将无法登录，确定要禁用该账号吗？`,
        '禁用账号',
        {
          confirmButtonText: '禁用',
          cancelButtonText: '取消',
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

async function removeUser(target: UserModel, isAdmin: boolean): Promise<void> {
  try {
    await ElMessageBox.confirm(
      `将永久删除「${target.username}」，该操作不可恢复。`,
      isAdmin ? '删除管理员' : '删除用户',
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
  pendingIds.value.add(target.id)
  try {
    if (isAdmin) {
      await deleteAdmin(target.id)
      admins.value = admins.value.filter((user) => user.id !== target.id)
    } else {
      await deleteCustomer(target.id)
      customers.value = customers.value.filter((user) => user.id !== target.id)
    }
    ElMessage.success('账号已删除')
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
    <PageHeader title="用户管理" description="管理员与普通用户分表管理：调整状态或删除账号。">
      <template #actions>
        <el-input
          v-model="search"
          class="user-search"
          placeholder="搜索用户名"
          clearable
          :prefix-icon="Search"
        />
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
        <el-button type="primary" :icon="Plus" @click="openCreate">新建管理员</el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="load" />

    <div v-else-if="loading && totalCount === 0" class="ax-card" aria-busy="true">
      <div class="ax-card__body"><el-skeleton :rows="6" animated /></div>
    </div>

    <el-tabs v-else v-model="activeTab" class="user-tabs">
      <el-tab-pane :label="`普通用户 (${customers.length})`" name="customers">
        <EmptyState
          v-if="filteredCustomers.length === 0"
          title="暂无普通用户"
          description="用户注册后会显示在这里。"
        />
        <div v-else class="ax-card table-card">
          <div class="ax-table-scroll">
            <el-table :data="filteredCustomers" style="width: 100%">
              <el-table-column label="用户" min-width="200">
                <template #default="{ row }">
                  <div class="user-cell">
                    <UserAvatar :name="row.username" size="sm" />
                    <div class="user-cell__meta">
                      <span class="user-cell__name">
                        {{ row.username }}
                        <el-tag v-if="isSelf(row)" size="small" type="primary" effect="plain">
                          当前账号
                        </el-tag>
                      </span>
                      <span class="user-cell__id">ID {{ row.id }}</span>
                    </div>
                  </div>
                </template>
              </el-table-column>
              <el-table-column label="用量" min-width="200">
                <template #default="{ row }">
                  <div class="user-usage">
                    <span class="user-usage__text">
                      {{ formatBytes(row.used_bytes) }}
                      <span class="ax-muted">
                        / {{ row.quota_bytes > 0 ? formatBytes(row.quota_bytes) : '不限' }}
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
              <el-table-column label="状态" width="150">
                <template #default="{ row }">
                  <div class="user-status">
                    <el-switch
                      :model-value="!row.disabled"
                      :loading="pendingIds.has(row.id)"
                      @change="onDisabledChange(row, $event, false)"
                    />
                    <span :class="row.disabled ? 'user-status__off' : 'user-status__on'">
                      {{ row.disabled ? '已禁用' : '正常' }}
                    </span>
                  </div>
                </template>
              </el-table-column>
              <el-table-column label="注册时间" min-width="170">
                <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
              </el-table-column>
              <el-table-column label="操作" width="100" align="right">
                <template #default="{ row }">
                  <el-button
                    link
                    type="danger"
                    :loading="pendingIds.has(row.id)"
                    @click="removeUser(row, false)"
                  >
                    删除
                  </el-button>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </div>
      </el-tab-pane>

      <el-tab-pane :label="`管理员 (${admins.length})`" name="admins">
        <EmptyState
          v-if="filteredAdmins.length === 0"
          title="暂无管理员"
          description="点击「新建管理员」创建特权账号。"
        />
        <div v-else class="ax-card table-card">
          <div class="ax-table-scroll">
            <el-table :data="filteredAdmins" style="width: 100%">
              <el-table-column label="管理员" min-width="220">
                <template #default="{ row }">
                  <div class="user-cell">
                    <UserAvatar :name="row.username" size="sm" />
                    <div class="user-cell__meta">
                      <span class="user-cell__name">
                        {{ row.username }}
                        <el-tag size="small" type="primary" effect="plain">管理员</el-tag>
                        <el-tag v-if="isSelf(row)" size="small" type="info" effect="plain">
                          当前账号
                        </el-tag>
                      </span>
                      <span class="user-cell__id">ID {{ row.id }}</span>
                    </div>
                  </div>
                </template>
              </el-table-column>
              <el-table-column label="状态" width="150">
                <template #default="{ row }">
                  <div class="user-status" :title="isSelf(row) ? '不能禁用当前登录账号' : undefined">
                    <el-switch
                      :model-value="!row.disabled"
                      :loading="pendingIds.has(row.id)"
                      :disabled="isSelf(row)"
                      @change="onDisabledChange(row, $event, true)"
                    />
                    <span :class="row.disabled ? 'user-status__off' : 'user-status__on'">
                      {{ row.disabled ? '已禁用' : '正常' }}
                    </span>
                  </div>
                </template>
              </el-table-column>
              <el-table-column label="创建时间" min-width="170">
                <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
              </el-table-column>
              <el-table-column label="操作" width="100" align="right">
                <template #default="{ row }">
                  <el-button
                    link
                    type="danger"
                    :disabled="isSelf(row)"
                    :loading="pendingIds.has(row.id)"
                    @click="removeUser(row, true)"
                  >
                    删除
                  </el-button>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </div>
      </el-tab-pane>
    </el-tabs>

    <el-dialog
      v-model="createOpen"
      title="新建管理员"
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
        <el-form-item label="用户名" prop="username">
          <el-input v-model="createForm.username" placeholder="管理员用户名" maxlength="64" />
        </el-form-item>
        <el-form-item label="密码" prop="password">
          <el-input
            v-model="createForm.password"
            type="password"
            show-password
            placeholder="至少 8 位"
            maxlength="72"
            @keyup.enter="submitCreate"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createOpen = false">取消</el-button>
        <el-button type="primary" :loading="creating" @click="submitCreate">创建</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.table-card {
  overflow: hidden;
}

.user-search {
  width: 220px;
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
  font-size: 11px;
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

@media (max-width: 768px) {
  .user-search {
    width: 100%;
  }
}
</style>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh, Search } from '@element-plus/icons-vue'

import { listUsers, updateUser } from '@/api/admin'
import { toApiError } from '@/api/client'
import type { User as UserModel, UserRole, UserUpdate } from '@/api/types'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'
import PageHeader from '@/components/PageHeader.vue'
import UserAvatar from '@/components/UserAvatar.vue'
import { useAuthStore } from '@/stores/auth'
import { formatBytes, formatDateTime, usagePercent } from '@/utils/format'

const auth = useAuthStore()

const users = ref<UserModel[]>([])
const loading = ref(false)
const errorMessage = ref('')
const search = ref('')
const pendingIds = ref<Set<number>>(new Set())

const filteredUsers = computed(() => {
  const keyword = search.value.trim().toLowerCase()
  if (!keyword) return users.value
  return users.value.filter((user) => user.username.toLowerCase().includes(keyword))
})

function isSelf(user: UserModel): boolean {
  return auth.user?.id === user.id
}

async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    users.value = (await listUsers()) ?? []
  } catch (error) {
    errorMessage.value = toApiError(error).message
  } finally {
    loading.value = false
  }
}

function onRoleChange(target: UserModel, value: unknown): void {
  const role: UserRole = value === 'admin' ? 'admin' : 'user'
  if (role === target.role) return
  void applyRole(target, role)
}

async function applyRole(target: UserModel, role: UserRole): Promise<void> {
  const label = role === 'admin' ? '管理员' : '普通用户'
  try {
    await ElMessageBox.confirm(`将「${target.username}」的角色调整为${label}。`, '修改用户角色', {
      confirmButtonText: '确认修改',
      cancelButtonText: '取消',
      type: 'warning',
    })
  } catch {
    return
  }
  await patchUser(target, { role })
}

function onDisabledChange(target: UserModel, value: unknown): void {
  void applyDisabled(target, value === true)
}

async function applyDisabled(target: UserModel, disabled: boolean): Promise<void> {
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
  await patchUser(target, { disabled })
}

async function patchUser(target: UserModel, payload: UserUpdate): Promise<void> {
  pendingIds.value.add(target.id)
  try {
    const updated = await updateUser(target.id, payload)
    users.value = users.value.map((user) => (user.id === updated.id ? updated : user))
    ElMessage.success('用户信息已更新')
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
    <PageHeader title="用户管理" description="调整用户角色，或禁用、启用账号。">
      <template #actions>
        <el-input
          v-model="search"
          class="user-search"
          placeholder="搜索用户名"
          clearable
          :prefix-icon="Search"
        />
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="load" />

    <div v-else-if="loading && users.length === 0" class="ax-card" aria-busy="true">
      <div class="ax-card__body"><el-skeleton :rows="6" animated /></div>
    </div>

    <EmptyState
      v-else-if="filteredUsers.length === 0"
      :title="users.length === 0 ? '暂无用户' : '没有匹配的用户'"
      :description="
        users.length === 0 ? '用户注册后会显示在这里。' : '尝试更换搜索关键词。'
      "
    />

    <div v-else class="ax-card table-card">
      <div class="ax-table-scroll">
        <el-table :data="filteredUsers" style="width: 100%">
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

          <el-table-column label="角色" width="132">
            <template #default="{ row }">
              <span :title="isSelf(row) ? '不能修改当前登录账号' : undefined">
                <el-select
                  :model-value="row.role"
                  size="small"
                  :disabled="isSelf(row) || pendingIds.has(row.id)"
                  @change="onRoleChange(row, $event)"
                >
                  <el-option label="用户" value="user" />
                  <el-option label="管理员" value="admin" />
                </el-select>
              </span>
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
                  aria-label="存储用量"
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
              <div class="user-status" :title="isSelf(row) ? '不能禁用当前登录账号' : undefined">
                <el-switch
                  :model-value="!row.disabled"
                  :loading="pendingIds.has(row.id)"
                  :disabled="isSelf(row)"
                  @change="onDisabledChange(row, $event)"
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
        </el-table>
      </div>
    </div>
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
  background: rgba(255, 255, 255, 0.06);
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

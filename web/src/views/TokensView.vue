<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { Key, Plus, Refresh } from '@element-plus/icons-vue'

import { toApiError } from '@/api/client'
import { createToken, listTokens, revokeToken } from '@/api/tokens'
import type { CreatedToken, Token } from '@/api/types'
import CopyField from '@/components/CopyField.vue'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'
import PageHeader from '@/components/PageHeader.vue'
import { formatDateTime } from '@/utils/format'

const tokens = ref<Token[]>([])
const loading = ref(false)
const errorMessage = ref('')
const revokingId = ref<number | null>(null)

const createOpen = ref(false)
const creating = ref(false)
const createFormRef = ref<FormInstance>()
const createForm = reactive({ name: '' })
const createdToken = ref<CreatedToken | null>(null)

const plaintextOpen = computed({
  get: () => createdToken.value !== null,
  set: (value: boolean) => {
    if (!value) createdToken.value = null
  },
})

const createRules: FormRules = {
  name: [
    { required: true, message: '请输入令牌名称', trigger: 'blur' },
    { min: 1, max: 40, message: '名称长度为 1 到 40 个字符', trigger: 'blur' },
  ],
}

async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    tokens.value = (await listTokens()) ?? []
  } catch (error) {
    errorMessage.value = toApiError(error).message
  } finally {
    loading.value = false
  }
}

function openCreate(): void {
  createForm.name = ''
  createOpen.value = true
  void nextTick(() => createFormRef.value?.clearValidate())
}

function closePlaintext(): void {
  createdToken.value = null
}

async function submitCreate(): Promise<void> {
  const instance = createFormRef.value
  if (!instance || creating.value) return
  const valid = await instance.validate().catch(() => false)
  if (!valid) return

  creating.value = true
  try {
    createdToken.value = await createToken(createForm.name.trim())
    createOpen.value = false
    ElMessage.success('令牌创建成功')
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    creating.value = false
  }
}

async function revoke(token: Token): Promise<void> {
  try {
    await ElMessageBox.confirm(
      `撤销后，使用「${token.name}」的请求将立即失效，且无法恢复。`,
      '撤销访问令牌',
      {
        confirmButtonText: '撤销',
        cancelButtonText: '取消',
        type: 'warning',
        confirmButtonClass: 'el-button--danger',
      },
    )
  } catch {
    return
  }

  revokingId.value = token.id
  try {
    await revokeToken(token.id)
    ElMessage.success('令牌已撤销')
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    revokingId.value = null
  }
}

onMounted(load)
</script>

<template>
  <div class="ax-page">
    <PageHeader title="访问令牌" description="用于通过 API 上传图片的凭证，创建后仅显示一次明文。">
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
        <el-button type="primary" :icon="Plus" @click="openCreate">新建 Token</el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="load" />

    <div v-else-if="loading && tokens.length === 0" class="ax-card" aria-busy="true">
      <div class="ax-card__body"><el-skeleton :rows="5" animated /></div>
    </div>

    <EmptyState
      v-else-if="tokens.length === 0"
      title="还没有访问令牌"
      description="创建令牌后即可通过 API 上传图片，令牌明文仅在创建时展示一次。"
    >
      <el-button type="primary" :icon="Plus" @click="openCreate">新建 Token</el-button>
    </EmptyState>

    <div v-else class="ax-card table-card">
      <div class="ax-table-scroll">
        <el-table :data="tokens" style="width: 100%">
          <el-table-column prop="name" label="名称" min-width="160">
            <template #default="{ row }">
              <span class="token-name">
                <el-icon :size="14"><Key /></el-icon>
                {{ row.name }}
              </span>
            </template>
          </el-table-column>
          <el-table-column label="令牌前缀" min-width="190">
            <template #default="{ row }">
              <code class="ax-mono">{{ row.prefix }}</code>
            </template>
          </el-table-column>
          <el-table-column label="创建时间" min-width="170">
            <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column label="最近使用" min-width="170">
            <template #default="{ row }">
              <span v-if="row.last_used_at">{{ formatDateTime(row.last_used_at) }}</span>
              <span v-else class="ax-muted">从未使用</span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="110" align="right">
            <template #default="{ row }">
              <el-button
                link
                type="danger"
                :loading="revokingId === row.id"
                @click="revoke(row)"
              >
                撤销
              </el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </div>

    <!-- 新建令牌 -->
    <el-dialog
      v-model="createOpen"
      title="新建访问令牌"
      width="min(440px, 92vw)"
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
        <el-form-item label="名称" prop="name">
          <el-input
            v-model="createForm.name"
            placeholder="例如：CI 上传 / 博客配图"
            maxlength="40"
            show-word-limit
            @keyup.enter="submitCreate"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createOpen = false">取消</el-button>
        <el-button type="primary" :loading="creating" @click="submitCreate">创建</el-button>
      </template>
    </el-dialog>

    <!-- 明文令牌（仅显示一次） -->
    <el-dialog
      :model-value="plaintextOpen"
      title="令牌创建成功"
      width="min(520px, 92vw)"
      :close-on-click-modal="false"
      :close-on-press-escape="false"
      :show-close="false"
    >
      <el-alert
        type="warning"
        :closable="false"
        show-icon
        title="该令牌仅显示一次"
        description="请立即复制并妥善保存。关闭此窗口后，将无法再次查看明文令牌。"
      />
      <div class="token-result">
        <CopyField label="访问令牌" :value="createdToken?.token ?? ''" />
        <p class="token-result__hint">
          使用方式：请求头
          <code class="ax-mono">Authorization: Bearer &lt;token&gt;</code>
        </p>
      </div>
      <template #footer>
        <el-button type="primary" @click="closePlaintext">我已保存，关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.table-card {
  overflow: hidden;
}

.token-name {
  display: inline-flex;
  align-items: center;
  gap: var(--ax-space-2);
  color: var(--ax-text);
  font-weight: var(--ax-weight-medium);
}

.token-result {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-4);
  margin-top: var(--ax-space-5);
}

.token-result__hint {
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
  overflow-wrap: anywhere;
}
</style>

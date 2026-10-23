<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import type { FormInstance, FormRules } from 'element-plus'
import { Delete, Key, Plus, Refresh } from '@element-plus/icons-vue'

import { toApiError } from '@/api/client'
import { createToken, listTokens, revokeToken } from '@/api/tokens'
import type { CreatedToken, Token } from '@/api/types'
import CopyField from '@/components/CopyField.vue'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'
import PageHeader from '@/components/PageHeader.vue'
import { formatDateTime } from '@/utils/format'

const { t } = useI18n()

const tokens = ref<Token[]>([])
const loading = ref(false)
const errorMessage = ref('')
const revokingId = ref<string | null>(null)

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

const createRules = computed<FormRules>(() => ({
  name: [
    { required: true, message: t('tokens.validation.nameRequired'), trigger: 'blur' },
    { min: 1, max: 40, message: t('tokens.validation.nameLength'), trigger: 'blur' },
  ],
}))

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
    ElMessage.success(t('tokens.messages.created'))
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
      t('tokens.confirm.revokeMessage', { name: token.name }),
      t('tokens.confirm.revokeTitle'),
      {
        confirmButtonText: t('tokens.confirm.revoke'),
        cancelButtonText: t('common.cancel'),
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
    ElMessage.success(t('tokens.messages.revoked'))
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
    <PageHeader :title="t('tokens.title')" :description="t('tokens.description')">
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
        <el-button type="primary" :icon="Plus" @click="openCreate">{{ t('tokens.create') }}</el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="load" />

    <div v-else-if="loading && tokens.length === 0" class="ax-card" aria-busy="true">
      <div class="ax-card__body"><el-skeleton :rows="5" animated /></div>
    </div>

    <EmptyState
      v-else-if="tokens.length === 0"
      :title="t('tokens.empty.title')"
      :description="t('tokens.empty.description')"
    >
      <el-button type="primary" :icon="Plus" @click="openCreate">{{ t('tokens.create') }}</el-button>
    </EmptyState>

    <div v-else class="ax-card table-card">
      <div class="ax-table-scroll">
        <el-table :data="tokens" style="width: 100%">
          <el-table-column prop="name" :label="t('tokens.columns.name')" min-width="160">
            <template #default="{ row }">
              <span class="token-name">
                <el-icon :size="14"><Key /></el-icon>
                {{ row.name }}
              </span>
            </template>
          </el-table-column>
          <el-table-column :label="t('tokens.columns.prefix')" min-width="190">
            <template #default="{ row }">
              <code class="ax-mono">{{ row.prefix }}</code>
            </template>
          </el-table-column>
          <el-table-column :label="t('tokens.columns.createdAt')" min-width="170">
            <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column :label="t('tokens.columns.lastUsed')" min-width="170">
            <template #default="{ row }">
              <span v-if="row.last_used_at">{{ formatDateTime(row.last_used_at) }}</span>
              <span v-else class="ax-muted">{{ t('tokens.neverUsed') }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('tokens.columns.actions')" width="130" align="right">
            <template #default="{ row }">
              <div class="ax-row-actions">
                <el-button
                  link
                  type="danger"
                  :icon="Delete"
                  :loading="revokingId === row.id"
                  @click="revoke(row)"
                >
                  {{ t('tokens.revoke') }}
                </el-button>
              </div>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </div>

    <!-- 新建令牌 -->
    <el-dialog
      v-model="createOpen"
      :title="t('tokens.dialog.createTitle')"
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
        <el-form-item :label="t('tokens.dialog.name')" prop="name">
          <el-input
            v-model="createForm.name"
            :placeholder="t('tokens.dialog.namePlaceholder')"
            maxlength="40"
            show-word-limit
            @keyup.enter="submitCreate"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createOpen = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="creating" @click="submitCreate">{{ t('tokens.dialog.submit') }}</el-button>
      </template>
    </el-dialog>

    <!-- 明文令牌（仅显示一次） -->
    <el-dialog
      :model-value="plaintextOpen"
      :title="t('tokens.plaintext.title')"
      width="min(520px, 92vw)"
      append-to-body
      :close-on-click-modal="false"
      :close-on-press-escape="false"
      :show-close="false"
    >
      <el-alert
        type="warning"
        :closable="false"
        show-icon
        :title="t('tokens.plaintext.alertTitle')"
        :description="t('tokens.plaintext.alertDescription')"
      />
      <div class="token-result">
        <CopyField :label="t('tokens.plaintext.label')" :value="createdToken?.token ?? ''" />
        <p class="token-result__hint">
          {{ t('tokens.plaintext.usageBefore') }}
          <code class="ax-mono">Authorization: Bearer &lt;token&gt;</code>
        </p>
      </div>
      <template #footer>
        <el-button type="primary" @click="closePlaintext">{{ t('tokens.plaintext.close') }}</el-button>
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

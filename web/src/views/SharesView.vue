<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import { Delete, Refresh } from '@element-plus/icons-vue'

import { toApiError } from '@/api/client'
import { listShares, revokeShare } from '@/api/shares'
import type { Share } from '@/api/types'
import CopyField from '@/components/CopyField.vue'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'
import PageHeader from '@/components/PageHeader.vue'
import { formatDateTime } from '@/utils/format'

const { t } = useI18n()

const shares = ref<Share[]>([])
const loading = ref(false)
const errorMessage = ref('')
const pendingId = ref('')

async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    shares.value = (await listShares()) ?? []
  } catch (error) {
    errorMessage.value = toApiError(error).message
  } finally {
    loading.value = false
  }
}

function targetLabel(share: Share): string {
  return share.target_type === 'album' ? t('shares.type.album') : t('shares.type.image')
}

async function remove(share: Share): Promise<void> {
  try {
    await ElMessageBox.confirm(t('shares.confirm.revokeMessage'), t('shares.confirm.revokeTitle'), {
      confirmButtonText: t('shares.actions.revoke'),
      cancelButtonText: t('common.cancel'),
      type: 'warning',
      confirmButtonClass: 'el-button--danger',
    })
  } catch {
    return
  }
  pendingId.value = share.id
  try {
    await revokeShare(share.id)
    shares.value = shares.value.filter((s) => s.id !== share.id)
    ElMessage.success(t('shares.messages.revoked'))
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    pendingId.value = ''
  }
}

onMounted(load)
</script>

<template>
  <div class="ax-page">
    <PageHeader :title="t('shares.title')" :description="t('shares.description')">
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="load" />

    <div v-else-if="loading && shares.length === 0" class="ax-card" aria-busy="true">
      <div class="ax-card__body"><el-skeleton :rows="5" animated /></div>
    </div>

    <EmptyState
      v-else-if="shares.length === 0"
      :title="t('shares.empty.title')"
      :description="t('shares.empty.description')"
    />

    <div v-else class="ax-card table-card">
      <div class="ax-table-scroll">
        <el-table :data="shares" style="width: 100%">
          <el-table-column :label="t('shares.columns.type')" width="90">
            <template #default="{ row }">
              <el-tag size="small" effect="plain">{{ targetLabel(row) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('shares.columns.link')" min-width="320">
            <template #default="{ row }">
              <CopyField :value="row.url" />
            </template>
          </el-table-column>
          <el-table-column :label="t('shares.columns.password')" width="90">
            <template #default="{ row }">
              <el-tag v-if="row.has_password" size="small" type="warning" effect="plain">
                {{ t('shares.passwordRequired') }}
              </el-tag>
              <span v-else class="ax-muted">{{ t('common.none') }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('shares.columns.views')" width="110">
            <template #default="{ row }">
              {{ row.view_count }}{{ row.max_views > 0 ? ` / ${row.max_views}` : '' }}
            </template>
          </el-table-column>
          <el-table-column :label="t('shares.columns.createdAt')" min-width="170">
            <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column :label="t('common.actions')" width="120" align="right">
            <template #default="{ row }">
              <div class="ax-row-actions">
                <el-button
                  link
                  type="danger"
                  :icon="Delete"
                  :loading="pendingId === row.id"
                  @click="remove(row)"
                >
                  {{ t('shares.actions.revoke') }}
                </el-button>
              </div>
            </template>
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
</style>

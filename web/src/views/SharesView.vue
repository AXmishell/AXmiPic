<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Refresh } from '@element-plus/icons-vue'

import { toApiError } from '@/api/client'
import { listShares, revokeShare } from '@/api/shares'
import type { Share } from '@/api/types'
import CopyField from '@/components/CopyField.vue'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'
import PageHeader from '@/components/PageHeader.vue'
import { formatDateTime } from '@/utils/format'

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
  return share.target_type === 'album' ? '相册' : '图片'
}

async function remove(share: Share): Promise<void> {
  try {
    await ElMessageBox.confirm('撤销后该链接将立即失效，操作不可恢复。', '撤销分享', {
      confirmButtonText: '撤销',
      cancelButtonText: '取消',
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
    ElMessage.success('分享已撤销')
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
    <PageHeader title="我的分享" description="管理图片与相册的分享链接，可随时撤销。">
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="load" />

    <div v-else-if="loading && shares.length === 0" class="ax-card" aria-busy="true">
      <div class="ax-card__body"><el-skeleton :rows="5" animated /></div>
    </div>

    <EmptyState
      v-else-if="shares.length === 0"
      title="还没有分享链接"
      description="在图片管理或相册页点击「分享」即可创建链接。"
    />

    <div v-else class="ax-card table-card">
      <div class="ax-table-scroll">
        <el-table :data="shares" style="width: 100%">
          <el-table-column label="类型" width="90">
            <template #default="{ row }">
              <el-tag size="small" effect="plain">{{ targetLabel(row) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="链接" min-width="320">
            <template #default="{ row }">
              <CopyField :value="row.url" />
            </template>
          </el-table-column>
          <el-table-column label="密码" width="90">
            <template #default="{ row }">
              <el-tag v-if="row.has_password" size="small" type="warning" effect="plain">需要</el-tag>
              <span v-else class="ax-muted">无</span>
            </template>
          </el-table-column>
          <el-table-column label="访问量" width="110">
            <template #default="{ row }">
              {{ row.view_count }}{{ row.max_views > 0 ? ` / ${row.max_views}` : '' }}
            </template>
          </el-table-column>
          <el-table-column label="创建时间" min-width="170">
            <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column label="操作" width="120" align="right">
            <template #default="{ row }">
              <div class="ax-row-actions">
                <el-button
                  link
                  type="danger"
                  :icon="Delete"
                  :loading="pendingId === row.id"
                  @click="remove(row)"
                >
                  撤销
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

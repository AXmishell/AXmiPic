<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Refresh } from '@element-plus/icons-vue'

import { getNotifyLogs, type NotifyLog } from '@/api/billing'
import { toApiError } from '@/api/client'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'
import PageHeader from '@/components/PageHeader.vue'
import TableSkeleton from '@/components/TableSkeleton.vue'
import { formatDateTime } from '@/utils/format'

const { t } = useI18n()

const logChannel = ref<'email' | 'sms'>('email')
const notifyLogs = ref<NotifyLog[]>([])
const logsTotal = ref(0)
const logsPage = ref(1)
const logsPageSize = ref(20)
const logsLoading = ref(false)
const errorMessage = ref('')

async function loadNotifyLogs(): Promise<void> {
  logsLoading.value = true
  errorMessage.value = ''
  try {
    const data = await getNotifyLogs({
      channel: logChannel.value,
      page: logsPage.value,
      page_size: logsPageSize.value,
    })
    notifyLogs.value = data.items
    logsTotal.value = data.total
  } catch (error) {
    errorMessage.value = toApiError(error).message
  } finally {
    logsLoading.value = false
  }
}

function switchLogChannel(channel: 'email' | 'sms'): void {
  if (logChannel.value === channel) return
  logChannel.value = channel
  logsPage.value = 1
  notifyLogs.value = []
  void loadNotifyLogs()
}

function onLogsPageChange(page: number): void {
  logsPage.value = page
  void loadNotifyLogs()
}

function onLogsSizeChange(size: number): void {
  logsPageSize.value = size
  logsPage.value = 1
  void loadNotifyLogs()
}

onMounted(loadNotifyLogs)
</script>

<template>
  <div class="ax-page">
    <PageHeader :title="t('notifyLogs.title')" :description="t('notifyLogs.description')">
      <template #actions>
        <el-radio-group
          :model-value="logChannel"
          @change="switchLogChannel($event as 'email' | 'sms')"
        >
          <el-radio-button value="email">{{ t('notifyLogs.channels.email') }}</el-radio-button>
          <el-radio-button value="sms">{{ t('notifyLogs.channels.sms') }}</el-radio-button>
        </el-radio-group>
        <el-button :icon="Refresh" :loading="logsLoading" @click="loadNotifyLogs">{{ t('common.refresh') }}</el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="loadNotifyLogs" />

    <TableSkeleton
      v-else-if="logsLoading && notifyLogs.length === 0"
      :rows="8"
      :columns="5"
      :title="t('notifyLogs.loading')"
    />

    <article v-else class="ax-card">
      <div class="ax-card__body">
        <EmptyState
          v-if="notifyLogs.length === 0"
          :title="t('notifyLogs.empty.title')"
          :description="logChannel === 'email' ? t('notifyLogs.empty.email') : t('notifyLogs.empty.sms')"
        />
        <el-table v-else :data="notifyLogs" style="width: 100%">
          <el-table-column :label="t('notifyLogs.columns.time')" width="180">
            <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column prop="to" :label="t('notifyLogs.columns.recipient')" min-width="180" show-overflow-tooltip />
          <el-table-column
            v-if="logChannel === 'email'"
            prop="subject"
            :label="t('notifyLogs.columns.subject')"
            min-width="160"
            show-overflow-tooltip
          />
          <el-table-column :label="t('notifyLogs.columns.status')" width="100">
            <template #default="{ row }">
              <el-tooltip v-if="row.status === 'failed' && row.error" :content="row.error" placement="top">
                <el-tag type="danger" size="small" effect="plain">{{ t('notifyLogs.status.failed') }}</el-tag>
              </el-tooltip>
              <el-tag v-else type="success" size="small" effect="plain">{{ t('notifyLogs.status.success') }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="provider" :label="t('notifyLogs.columns.channel')" width="100" />
          <el-table-column prop="body" :label="t('notifyLogs.columns.body')" min-width="260" show-overflow-tooltip />
        </el-table>
        <div v-if="logsTotal > 0" class="notify-logs__pager">
          <el-pagination
            background
            layout="sizes, prev, pager, next, total"
            :page-sizes="[10, 20, 50, 100]"
            :total="logsTotal"
            :page-size="logsPageSize"
            :current-page="logsPage"
            @current-change="onLogsPageChange"
            @size-change="onLogsSizeChange"
          />
        </div>
      </div>
    </article>
  </div>
</template>

<style scoped>
.notify-logs__pager {
  display: flex;
  justify-content: flex-end;
  margin-top: var(--ax-space-3);
}
</style>

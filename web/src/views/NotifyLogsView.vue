<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Refresh } from '@element-plus/icons-vue'

import { getNotifyLogs, type NotifyLog } from '@/api/billing'
import { toApiError } from '@/api/client'
import ErrorState from '@/components/ErrorState.vue'
import PageHeader from '@/components/PageHeader.vue'
import { formatDateTime } from '@/utils/format'

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

onMounted(loadNotifyLogs)
</script>

<template>
  <div class="ax-page">
    <PageHeader title="邮件日志" description="查看邮件与短信通知的发送记录，日志保留 30 天。">
      <template #actions>
        <el-radio-group
          :model-value="logChannel"
          @change="switchLogChannel($event as 'email' | 'sms')"
        >
          <el-radio-button value="email">邮件</el-radio-button>
          <el-radio-button value="sms">短信</el-radio-button>
        </el-radio-group>
        <el-button :icon="Refresh" :loading="logsLoading" @click="loadNotifyLogs">刷新</el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="loadNotifyLogs" />

    <article v-else class="ax-card">
      <div class="ax-card__body">
        <el-skeleton v-if="logsLoading && notifyLogs.length === 0" :rows="4" animated />
        <template v-else>
          <el-table :data="notifyLogs" style="width: 100%">
            <el-table-column label="时间" width="180">
              <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
            </el-table-column>
            <el-table-column prop="to" label="接收方" min-width="180" show-overflow-tooltip />
            <el-table-column
              v-if="logChannel === 'email'"
              prop="subject"
              label="主题"
              min-width="160"
              show-overflow-tooltip
            />
            <el-table-column label="状态" width="100">
              <template #default="{ row }">
                <el-tooltip v-if="row.status === 'failed' && row.error" :content="row.error" placement="top">
                  <el-tag type="danger" size="small" effect="plain">失败</el-tag>
                </el-tooltip>
                <el-tag v-else type="success" size="small" effect="plain">成功</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="provider" label="渠道" width="100" />
            <el-table-column prop="body" label="内容" min-width="260" show-overflow-tooltip />
          </el-table>
          <el-empty v-if="notifyLogs.length === 0" description="暂无日志" :image-size="60" />
        </template>
        <div class="notify-logs__pager">
          <el-pagination
            layout="prev, pager, next, total"
            :total="logsTotal"
            :page-size="logsPageSize"
            :current-page="logsPage"
            @current-change="onLogsPageChange"
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

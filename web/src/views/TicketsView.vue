<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { Plus, Refresh, View } from '@element-plus/icons-vue'

import { createTicket, getTicket, listTickets, replyTicket } from '@/api/billing'
import { toApiError } from '@/api/client'
import type { Ticket, TicketPriority, TicketStatus } from '@/api/types'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'
import PageHeader from '@/components/PageHeader.vue'
import { formatDateTime } from '@/utils/format'

const tickets = ref<Ticket[]>([])
const loading = ref(false)
const errorMessage = ref('')
const statusFilter = ref<'' | TicketStatus>('')

const createOpen = ref(false)
const creating = ref(false)
const formRef = ref<FormInstance>()
const form = reactive<{ subject: string; category: string; body: string; priority: TicketPriority }>({
  subject: '',
  category: '',
  body: '',
  priority: 'normal',
})
const rules: FormRules = {
  subject: [{ required: true, message: '请输入标题', trigger: 'blur' }],
  body: [{ required: true, message: '请输入问题描述', trigger: 'blur' }],
}

const detailOpen = ref(false)
const detail = ref<Ticket | null>(null)
const replyBody = ref('')
const replying = ref(false)

const statusOptions: { value: TicketStatus; label: string; tag: string }[] = [
  { value: 'open', label: '待处理', tag: 'warning' },
  { value: 'answered', label: '已回复', tag: 'success' },
  { value: 'closed', label: '已关闭', tag: 'info' },
]

const priorityOptions: { value: TicketPriority; label: string }[] = [
  { value: 'low', label: '低' },
  { value: 'normal', label: '普通' },
  { value: 'high', label: '高' },
]

const filtered = computed(() =>
  statusFilter.value ? tickets.value.filter((t) => t.status === statusFilter.value) : tickets.value,
)

function statusMeta(status: TicketStatus) {
  return statusOptions.find((s) => s.value === status) ?? statusOptions[0]!
}

async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    tickets.value = (await listTickets()) ?? []
  } catch (error) {
    errorMessage.value = toApiError(error).message
  } finally {
    loading.value = false
  }
}

function openCreate(): void {
  Object.assign(form, { subject: '', category: '', body: '', priority: 'normal' })
  createOpen.value = true
}

async function submitCreate(): Promise<void> {
  const instance = formRef.value
  if (!instance || creating.value) return
  const valid = await instance.validate().catch(() => false)
  if (!valid) return
  creating.value = true
  try {
    await createTicket({ ...form, subject: form.subject.trim(), body: form.body.trim() })
    ElMessage.success('工单已提交')
    createOpen.value = false
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    creating.value = false
  }
}

async function openDetail(ticket: Ticket): Promise<void> {
  replyBody.value = ''
  detail.value = await getTicket(ticket.id).catch(() => ticket)
  detailOpen.value = true
}

async function submitReply(): Promise<void> {
  const ticket = detail.value
  if (!ticket || !replyBody.value.trim() || replying.value) return
  replying.value = true
  try {
    detail.value = await replyTicket(ticket.id, replyBody.value.trim())
    replyBody.value = ''
    ElMessage.success('回复已提交')
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    replying.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="ax-page">
    <PageHeader title="工单" description="提交问题并跟踪处理进度。">
      <template #actions>
        <el-select v-model="statusFilter" placeholder="全部状态" class="ticket-filter">
          <el-option label="全部状态" value="" />
          <el-option v-for="s in statusOptions" :key="s.value" :label="s.label" :value="s.value" />
        </el-select>
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
        <el-button type="primary" :icon="Plus" @click="openCreate">新建工单</el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="load" />

    <div v-else-if="loading && tickets.length === 0" class="ax-card" aria-busy="true">
      <div class="ax-card__body"><el-skeleton :rows="5" animated /></div>
    </div>

    <EmptyState
      v-else-if="filtered.length === 0"
      title="暂无工单"
      description="遇到问题时可点击「新建工单」反馈。"
    />

    <div v-else class="ax-card table-card">
      <div class="ax-table-scroll">
        <el-table :data="filtered" style="width: 100%">
          <el-table-column label="标题" min-width="240">
            <template #default="{ row }">
              <el-button link type="primary" @click="openDetail(row)">{{ row.subject }}</el-button>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="100">
            <template #default="{ row }">
              <el-tag size="small" :type="(statusMeta(row.status).tag as any)" effect="plain">
                {{ statusMeta(row.status).label }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="更新时间" min-width="170">
            <template #default="{ row }">{{ formatDateTime(row.updated_at) }}</template>
          </el-table-column>
          <el-table-column label="操作" width="120" align="right">
            <template #default="{ row }">
              <div class="ax-row-actions">
                <el-button link type="primary" :icon="View" @click="openDetail(row)">查看</el-button>
              </div>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </div>

    <!-- 新建工单 -->
    <el-dialog v-model="createOpen" title="新建工单" width="min(520px, 92vw)" append-to-body>
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
        <el-form-item label="标题" prop="subject">
          <el-input v-model="form.subject" maxlength="200" />
        </el-form-item>
        <el-form-item label="分类">
          <el-input v-model="form.category" placeholder="例如：上传问题、账号问题" maxlength="32" />
        </el-form-item>
        <el-form-item label="优先级">
          <el-select v-model="form.priority" style="width: 100%">
            <el-option v-for="p in priorityOptions" :key="p.value" :label="p.label" :value="p.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="问题描述" prop="body">
          <el-input v-model="form.body" type="textarea" :rows="5" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createOpen = false">取消</el-button>
        <el-button type="primary" :loading="creating" @click="submitCreate">提交</el-button>
      </template>
    </el-dialog>

    <!-- 工单详情 -->
    <el-drawer v-model="detailOpen" :title="detail?.subject ?? '工单详情'" size="min(520px, 92vw)" append-to-body>
      <div v-if="detail" class="ticket-detail">
        <div class="ticket-detail__meta">
          <el-tag size="small" :type="(statusMeta(detail.status).tag as any)" effect="plain">
            {{ statusMeta(detail.status).label }}
          </el-tag>
          <span class="ax-muted">{{ formatDateTime(detail.created_at) }}</span>
        </div>
        <ul class="ticket-messages">
          <li
            v-for="message in detail.messages"
            :key="message.id"
            class="ticket-message"
            :class="{ 'is-admin': message.author_role === 'admin' }"
          >
            <div class="ticket-message__head">
              <strong>{{ message.author_role === 'admin' ? '客服' : '我' }}</strong>
              <span class="ax-muted">{{ formatDateTime(message.created_at) }}</span>
            </div>
            <p class="ticket-message__body">{{ message.body }}</p>
          </li>
        </ul>
        <div class="ticket-reply">
          <el-input v-model="replyBody" type="textarea" :rows="3" placeholder="输入回复内容" />
          <el-button type="primary" :loading="replying" @click="submitReply">回复</el-button>
        </div>
      </div>
    </el-drawer>
  </div>
</template>

<style scoped>
.table-card {
  overflow: hidden;
}

.ticket-filter {
  width: 140px;
}

.ticket-detail {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-4);
}

.ticket-detail__meta {
  display: flex;
  align-items: center;
  gap: var(--ax-space-3);
}

.ticket-messages {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-3);
  margin: 0;
  padding: 0;
  list-style: none;
}

.ticket-message {
  padding: var(--ax-space-3);
  background: var(--ax-tint-weak);
  border: 1px solid var(--ax-border-subtle);
  border-radius: var(--ax-radius-md);
}

.ticket-message.is-admin {
  background: var(--ax-accent-soft);
  border-color: var(--ax-accent-ring);
}

.ticket-message__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--ax-space-1);
  font-size: var(--ax-text-xs);
}

.ticket-message__body {
  margin: 0;
  color: var(--ax-text-2);
  font-size: var(--ax-text-sm);
  white-space: pre-wrap;
}

.ticket-reply {
  display: flex;
  flex-direction: column;
  gap: var(--ax-space-2);
}

.ticket-reply :deep(.el-button) {
  align-self: flex-end;
}
</style>

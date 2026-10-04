<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { Plus, Refresh } from '@element-plus/icons-vue'

import { toApiError } from '@/api/client'
import {
  createAnnouncement,
  createPage,
  deleteAnnouncement,
  deletePage,
  listAllAnnouncements,
  listPages,
  listReports,
  updateAnnouncement,
  updatePage,
  updateReport,
} from '@/api/site'
import type {
  Announcement,
  AnnouncementInput,
  AnnouncementLevel,
  Page,
  PageInput,
  Report,
  ReportStatus,
} from '@/api/types'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'
import PageHeader from '@/components/PageHeader.vue'
import { formatDateTime } from '@/utils/format'

type Tab = 'announcements' | 'reports' | 'pages'
const activeTab = ref<Tab>('announcements')

const announcements = ref<Announcement[]>([])
const reports = ref<Report[]>([])
const pages = ref<Page[]>([])
const loading = ref(false)
const errorMessage = ref('')

const levelOptions: { value: AnnouncementLevel; label: string; tag: string }[] = [
  { value: 'info', label: '信息', tag: 'info' },
  { value: 'success', label: '成功', tag: 'success' },
  { value: 'warning', label: '警告', tag: 'warning' },
  { value: 'danger', label: '紧急', tag: 'danger' },
]

const reportStatusOptions: { value: ReportStatus; label: string; tag: string }[] = [
  { value: 'pending', label: '待处理', tag: 'warning' },
  { value: 'resolved', label: '已处理', tag: 'success' },
  { value: 'rejected', label: '已驳回', tag: 'info' },
]

const reportFilter = ref<'' | ReportStatus>('')

async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    const [annList, reportList, pageList] = await Promise.all([
      listAllAnnouncements(),
      listReports(reportFilter.value || undefined),
      listPages(),
    ])
    announcements.value = annList ?? []
    reports.value = reportList ?? []
    pages.value = pageList ?? []
  } catch (error) {
    errorMessage.value = toApiError(error).message
  } finally {
    loading.value = false
  }
}

async function loadReports(): Promise<void> {
  try {
    reports.value = (await listReports(reportFilter.value || undefined)) ?? []
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  }
}

// ---- 公告 ----
const annOpen = ref(false)
const annSaving = ref(false)
const annFormRef = ref<FormInstance>()
const editingAnnId = ref('')
const annForm = reactive<AnnouncementInput>({ title: '', content: '', level: 'info', pinned: false, published: true })
const annRules: FormRules = {
  title: [
    { required: true, message: '请输入标题', trigger: 'blur' },
    { max: 200, message: '标题最多 200 个字符', trigger: 'blur' },
  ],
}

function openCreateAnnouncement(): void {
  editingAnnId.value = ''
  Object.assign(annForm, { title: '', content: '', level: 'info', pinned: false, published: true })
  annOpen.value = true
}

function openEditAnnouncement(item: Announcement): void {
  editingAnnId.value = item.id
  Object.assign(annForm, {
    title: item.title,
    content: item.content,
    level: item.level,
    pinned: item.pinned,
    published: item.published,
  })
  annOpen.value = true
}

async function submitAnnouncement(): Promise<void> {
  const instance = annFormRef.value
  if (!instance || annSaving.value) return
  const valid = await instance.validate().catch(() => false)
  if (!valid) return
  annSaving.value = true
  try {
    const payload: AnnouncementInput = {
      title: annForm.title.trim(),
      content: annForm.content.trim(),
      level: annForm.level,
      pinned: annForm.pinned,
      published: annForm.published,
    }
    if (editingAnnId.value) {
      await updateAnnouncement(editingAnnId.value, payload)
      ElMessage.success('公告已更新')
    } else {
      await createAnnouncement(payload)
      ElMessage.success('公告已创建')
    }
    annOpen.value = false
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    annSaving.value = false
  }
}

async function removeAnnouncement(item: Announcement): Promise<void> {
  try {
    await ElMessageBox.confirm(`将删除公告「${item.title}」。`, '删除公告', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning',
      confirmButtonClass: 'el-button--danger',
    })
  } catch {
    return
  }
  try {
    await deleteAnnouncement(item.id)
    ElMessage.success('公告已删除')
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  }
}

// ---- 页面 ----
const pageOpen = ref(false)
const pageSaving = ref(false)
const pageFormRef = ref<FormInstance>()
const editingPageId = ref('')
const pageForm = reactive<PageInput>({ slug: '', title: '', content: '', published: true })
const pageRules: FormRules = {
  slug: [
    { required: true, message: '请输入 slug', trigger: 'blur' },
    { pattern: /^[a-z0-9][a-z0-9-]*$/, message: '仅小写字母、数字与连字符', trigger: 'blur' },
  ],
  title: [{ required: true, message: '请输入标题', trigger: 'blur' }],
}

function openCreatePage(): void {
  editingPageId.value = ''
  Object.assign(pageForm, { slug: '', title: '', content: '', published: true })
  pageOpen.value = true
}

function openEditPage(page: Page): void {
  editingPageId.value = page.id
  Object.assign(pageForm, { slug: page.slug, title: page.title, content: page.content, published: page.published })
  pageOpen.value = true
}

async function submitPage(): Promise<void> {
  const instance = pageFormRef.value
  if (!instance || pageSaving.value) return
  const valid = await instance.validate().catch(() => false)
  if (!valid) return
  pageSaving.value = true
  try {
    const payload: PageInput = {
      slug: pageForm.slug.trim().toLowerCase(),
      title: pageForm.title.trim(),
      content: pageForm.content,
      published: pageForm.published,
    }
    if (editingPageId.value) {
      await updatePage(editingPageId.value, payload)
      ElMessage.success('页面已更新')
    } else {
      await createPage(payload)
      ElMessage.success('页面已创建')
    }
    pageOpen.value = false
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    pageSaving.value = false
  }
}

async function removePage(page: Page): Promise<void> {
  try {
    await ElMessageBox.confirm(`将删除页面「${page.title}」。`, '删除页面', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning',
      confirmButtonClass: 'el-button--danger',
    })
  } catch {
    return
  }
  try {
    await deletePage(page.id)
    ElMessage.success('页面已删除')
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  }
}

// ---- 举报 ----
async function handleReport(report: Report, status: ReportStatus): Promise<void> {
  const note = status === 'rejected' ? '举报不成立' : '已处理'
  try {
    await updateReport(report.id, status, note)
    ElMessage.success('举报已更新')
    await loadReports()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  }
}

onMounted(load)
</script>

<template>
  <div class="ax-page">
    <PageHeader title="站点内容" description="维护站内公告、处理用户举报、管理独立页面。">
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
        <el-button v-if="activeTab === 'announcements'" type="primary" :icon="Plus" @click="openCreateAnnouncement">
          新建公告
        </el-button>
        <el-button v-else-if="activeTab === 'pages'" type="primary" :icon="Plus" @click="openCreatePage">
          新建页面
        </el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="load" />

    <el-tabs v-else v-model="activeTab" class="site-tabs">
      <el-tab-pane :label="`公告 (${announcements.length})`" name="announcements">
        <EmptyState v-if="announcements.length === 0" title="暂无公告" description="点击「新建公告」发布站内公告。" />
        <div v-else class="ax-card table-card">
          <div class="ax-table-scroll">
            <el-table :data="announcements" style="width: 100%">
              <el-table-column label="标题" min-width="220">
                <template #default="{ row }">
                  <span class="site-title">
                    <el-tag v-if="row.pinned" size="small" type="danger" effect="plain">置顶</el-tag>
                    {{ row.title }}
                  </span>
                </template>
              </el-table-column>
              <el-table-column label="级别" width="100">
                <template #default="{ row }">
                  <el-tag size="small" :type="(levelOptions.find((l) => l.value === row.level)?.tag as any)" effect="plain">
                    {{ levelOptions.find((l) => l.value === row.level)?.label ?? row.level }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column label="状态" width="100">
                <template #default="{ row }">
                  <el-tag size="small" :type="row.published ? 'success' : 'info'" effect="plain">
                    {{ row.published ? '已发布' : '草稿' }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column label="更新时间" min-width="170">
                <template #default="{ row }">{{ formatDateTime(row.updated_at) }}</template>
              </el-table-column>
              <el-table-column label="操作" width="150" align="right">
                <template #default="{ row }">
                  <el-button link type="primary" @click="openEditAnnouncement(row)">编辑</el-button>
                  <el-button link type="danger" @click="removeAnnouncement(row)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </div>
      </el-tab-pane>

      <el-tab-pane :label="`举报 (${reports.length})`" name="reports">
        <div class="site-toolbar">
          <el-radio-group
            :model-value="reportFilter"
            @update:model-value="(v: any) => { reportFilter = v; loadReports() }"
          >
            <el-radio-button value="">全部</el-radio-button>
            <el-radio-button value="pending">待处理</el-radio-button>
            <el-radio-button value="resolved">已处理</el-radio-button>
            <el-radio-button value="rejected">已驳回</el-radio-button>
          </el-radio-group>
        </div>
        <EmptyState v-if="reports.length === 0" title="暂无举报" description="用户举报会显示在这里。" />
        <div v-else class="ax-card table-card">
          <div class="ax-table-scroll">
            <el-table :data="reports" style="width: 100%">
              <el-table-column label="原因" min-width="180">
                <template #default="{ row }">
                  <div class="site-cell">
                    <span>{{ row.reason }}</span>
                    <span class="site-cell__sub">{{ row.detail || '—' }}</span>
                  </div>
                </template>
              </el-table-column>
              <el-table-column label="举报人" width="140">
                <template #default="{ row }">{{ row.reporter_name || '匿名' }}</template>
              </el-table-column>
              <el-table-column label="状态" width="100">
                <template #default="{ row }">
                  <el-tag size="small" :type="(reportStatusOptions.find((s) => s.value === row.status)?.tag as any)" effect="plain">
                    {{ reportStatusOptions.find((s) => s.value === row.status)?.label ?? row.status }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column label="提交时间" min-width="170">
                <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
              </el-table-column>
              <el-table-column label="操作" width="170" align="right">
                <template #default="{ row }">
                  <el-button link type="primary" :disabled="row.status === 'resolved'" @click="handleReport(row, 'resolved')">
                    处理
                  </el-button>
                  <el-button link type="info" :disabled="row.status === 'rejected'" @click="handleReport(row, 'rejected')">
                    驳回
                  </el-button>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </div>
      </el-tab-pane>

      <el-tab-pane :label="`页面 (${pages.length})`" name="pages">
        <EmptyState v-if="pages.length === 0" title="暂无独立页面" description="创建后可经 /p/{slug} 公开访问。" />
        <div v-else class="ax-card table-card">
          <div class="ax-table-scroll">
            <el-table :data="pages" style="width: 100%">
              <el-table-column label="标题" min-width="200">
                <template #default="{ row }">{{ row.title }}</template>
              </el-table-column>
              <el-table-column label="Slug" width="180">
                <template #default="{ row }"><code class="ax-mono">/p/{{ row.slug }}</code></template>
              </el-table-column>
              <el-table-column label="状态" width="100">
                <template #default="{ row }">
                  <el-tag size="small" :type="row.published ? 'success' : 'info'" effect="plain">
                    {{ row.published ? '已发布' : '草稿' }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column label="更新时间" min-width="170">
                <template #default="{ row }">{{ formatDateTime(row.updated_at) }}</template>
              </el-table-column>
              <el-table-column label="操作" width="150" align="right">
                <template #default="{ row }">
                  <el-button link type="primary" @click="openEditPage(row)">编辑</el-button>
                  <el-button link type="danger" @click="removePage(row)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </div>
      </el-tab-pane>
    </el-tabs>

    <!-- 公告编辑 -->
    <el-dialog
      v-model="annOpen"
      :title="editingAnnId ? '编辑公告' : '新建公告'"
      width="min(560px, 92vw)"
      append-to-body
      :close-on-click-modal="false"
      @closed="annFormRef?.clearValidate()"
    >
      <el-form ref="annFormRef" :model="annForm" :rules="annRules" label-position="top">
        <el-form-item label="标题" prop="title">
          <el-input v-model="annForm.title" maxlength="200" show-word-limit />
        </el-form-item>
        <el-form-item label="内容">
          <el-input v-model="annForm.content" type="textarea" :rows="5" maxlength="20000" />
        </el-form-item>
        <el-form-item label="级别">
          <el-select v-model="annForm.level" style="width: 100%">
            <el-option v-for="l in levelOptions" :key="l.value" :label="l.label" :value="l.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="选项">
          <el-checkbox v-model="annForm.pinned">置顶</el-checkbox>
          <el-checkbox v-model="annForm.published">发布</el-checkbox>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="annOpen = false">取消</el-button>
        <el-button type="primary" :loading="annSaving" @click="submitAnnouncement">保存</el-button>
      </template>
    </el-dialog>

    <!-- 页面编辑 -->
    <el-dialog
      v-model="pageOpen"
      :title="editingPageId ? '编辑页面' : '新建页面'"
      width="min(640px, 92vw)"
      append-to-body
      :close-on-click-modal="false"
      @closed="pageFormRef?.clearValidate()"
    >
      <el-form ref="pageFormRef" :model="pageForm" :rules="pageRules" label-position="top">
        <el-form-item label="标题" prop="title">
          <el-input v-model="pageForm.title" maxlength="200" />
        </el-form-item>
        <el-form-item label="Slug" prop="slug">
          <el-input v-model="pageForm.slug" placeholder="例如 about" maxlength="100" />
          <div class="form-hint">公开地址：/p/{{ pageForm.slug || 'slug' }}</div>
        </el-form-item>
        <el-form-item label="内容（支持 Markdown 文本）">
          <el-input v-model="pageForm.content" type="textarea" :rows="10" maxlength="100000" />
        </el-form-item>
        <el-form-item label="发布">
          <el-switch v-model="pageForm.published" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="pageOpen = false">取消</el-button>
        <el-button type="primary" :loading="pageSaving" @click="submitPage">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.table-card {
  overflow: hidden;
}

.site-toolbar {
  margin-bottom: var(--ax-space-3);
}

.site-title {
  display: inline-flex;
  align-items: center;
  gap: var(--ax-space-2);
  color: var(--ax-text);
  font-weight: var(--ax-weight-medium);
}

.site-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.site-cell__sub {
  overflow: hidden;
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.form-hint {
  margin-top: 4px;
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}
</style>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import type { FormInstance, FormRules } from 'element-plus'
import { Check, Close, Delete, EditPen, Plus, Refresh } from '@element-plus/icons-vue'

import { toApiError } from '@/api/client'
import {
  createAnnouncement,
  createPage,
  deleteAnnouncement,
  deletePage,
  listAllAnnouncements,
  listPages,
  listReportsPaged,
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
import TableSkeleton from '@/components/TableSkeleton.vue'
import { formatDateTime } from '@/utils/format'
import { useTabQuery } from '@/utils/useTabQuery'

type Tab = 'announcements' | 'reports' | 'pages'
const SITE_TABS: readonly Tab[] = ['announcements', 'reports', 'pages']
const activeTab = useTabQuery<Tab>('tab', 'announcements', SITE_TABS)

const { t } = useI18n()

const announcements = ref<Announcement[]>([])
const reports = ref<Report[]>([])
const pages = ref<Page[]>([])
const loading = ref(false)
const errorMessage = ref('')

const levelOptions = computed<{ value: AnnouncementLevel; label: string; tag: string }[]>(() => [
  { value: 'info', label: t('site.levels.info'), tag: 'info' },
  { value: 'success', label: t('site.levels.success'), tag: 'success' },
  { value: 'warning', label: t('site.levels.warning'), tag: 'warning' },
  { value: 'danger', label: t('site.levels.danger'), tag: 'danger' },
])

const reportStatusOptions = computed<{ value: ReportStatus; label: string; tag: string }[]>(() => [
  { value: 'pending', label: t('site.reportStatus.pending'), tag: 'warning' },
  { value: 'resolved', label: t('site.reportStatus.resolved'), tag: 'success' },
  { value: 'rejected', label: t('site.reportStatus.rejected'), tag: 'info' },
])

const reportFilter = ref<'' | ReportStatus>('')
const reportPage = ref(1)
const reportPageSize = ref(20)
const reportTotal = ref(0)
const reportsLoading = ref(false)

async function fetchReports(): Promise<void> {
  reportsLoading.value = true
  try {
    const data = await listReportsPaged({
      page: reportPage.value,
      page_size: reportPageSize.value,
      status: reportFilter.value || undefined,
    })
    reports.value = data.items ?? []
    reportTotal.value = data.total ?? 0
    if (reports.value.length === 0 && reportPage.value > 1 && reportTotal.value > 0) {
      reportPage.value -= 1
      await fetchReports()
    }
  } finally {
    reportsLoading.value = false
  }
}

async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    const [annList, , pageList] = await Promise.all([
      listAllAnnouncements(),
      fetchReports(),
      listPages(),
    ])
    announcements.value = annList ?? []
    pages.value = pageList ?? []
  } catch (error) {
    errorMessage.value = toApiError(error).message
  } finally {
    loading.value = false
  }
}

async function loadReports(): Promise<void> {
  try {
    await fetchReports()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  }
}

function onReportFilterChange(value: '' | ReportStatus): void {
  reportFilter.value = value
  reportPage.value = 1
  void loadReports()
}

function onReportPageChange(page: number): void {
  reportPage.value = page
  void loadReports()
}

function onReportSizeChange(size: number): void {
  reportPageSize.value = size
  reportPage.value = 1
  void loadReports()
}

function clearReportFilter(): void {
  reportFilter.value = ''
  reportPage.value = 1
  void loadReports()
}

// ---- 公告 ----
const annOpen = ref(false)
const annSaving = ref(false)
const annFormRef = ref<FormInstance>()
const editingAnnId = ref('')
const annForm = reactive<AnnouncementInput>({ title: '', content: '', level: 'info', pinned: false, published: true })
const annRules = computed<FormRules>(() => ({
  title: [
    { required: true, message: t('site.announcement.validation.titleRequired'), trigger: 'blur' },
    { max: 200, message: t('site.announcement.validation.titleMax'), trigger: 'blur' },
  ],
}))
const annSnapshot = ref('')

function annState(): string {
  return JSON.stringify(annForm)
}

async function closeAnnDialog(done: () => void): Promise<void> {
  if (annSaving.value) return
  if (annState() === annSnapshot.value) {
    done()
    return
  }
  try {
    await ElMessageBox.confirm(t('site.discard.message'), t('site.discard.title'), {
      confirmButtonText: t('site.discard.confirm'),
      cancelButtonText: t('site.discard.cancel'),
      type: 'warning',
      confirmButtonClass: 'el-button--danger',
    })
    done()
  } catch {
    // 用户选择继续编辑。
  }
}

function openCreateAnnouncement(): void {
  editingAnnId.value = ''
  Object.assign(annForm, { title: '', content: '', level: 'info', pinned: false, published: true })
  annSnapshot.value = annState()
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
  annSnapshot.value = annState()
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
      ElMessage.success(t('site.announcement.updated'))
    } else {
      await createAnnouncement(payload)
      ElMessage.success(t('site.announcement.created'))
    }
    annSnapshot.value = annState()
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
    await ElMessageBox.confirm(t('site.announcement.deleteConfirm', { title: item.title }), t('site.announcement.deleteTitle'), {
      confirmButtonText: t('common.delete'),
      cancelButtonText: t('common.cancel'),
      type: 'warning',
      confirmButtonClass: 'el-button--danger',
    })
  } catch {
    return
  }
  try {
    await deleteAnnouncement(item.id)
    ElMessage.success(t('site.announcement.deleted'))
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
const pageRules = computed<FormRules>(() => ({
  slug: [
    { required: true, message: t('site.pageForm.validation.slugRequired'), trigger: 'blur' },
    { pattern: /^[a-z0-9][a-z0-9-]*$/, message: t('site.pageForm.validation.slugPattern'), trigger: 'blur' },
  ],
  title: [{ required: true, message: t('site.pageForm.validation.titleRequired'), trigger: 'blur' }],
}))
const pageSnapshot = ref('')

function pageState(): string {
  return JSON.stringify(pageForm)
}

async function closePageDialog(done: () => void): Promise<void> {
  if (pageSaving.value) return
  if (pageState() === pageSnapshot.value) {
    done()
    return
  }
  try {
    await ElMessageBox.confirm(t('site.discard.message'), t('site.discard.title'), {
      confirmButtonText: t('site.discard.confirm'),
      cancelButtonText: t('site.discard.cancel'),
      type: 'warning',
      confirmButtonClass: 'el-button--danger',
    })
    done()
  } catch {
    // 用户选择继续编辑。
  }
}

function openCreatePage(): void {
  editingPageId.value = ''
  Object.assign(pageForm, { slug: '', title: '', content: '', published: true })
  pageSnapshot.value = pageState()
  pageOpen.value = true
}

function openEditPage(page: Page): void {
  editingPageId.value = page.id
  Object.assign(pageForm, { slug: page.slug, title: page.title, content: page.content, published: page.published })
  pageSnapshot.value = pageState()
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
      ElMessage.success(t('site.pageForm.updated'))
    } else {
      await createPage(payload)
      ElMessage.success(t('site.pageForm.created'))
    }
    pageSnapshot.value = pageState()
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
    await ElMessageBox.confirm(t('site.pageForm.deleteConfirm', { title: page.title }), t('site.pageForm.deleteTitle'), {
      confirmButtonText: t('common.delete'),
      cancelButtonText: t('common.cancel'),
      type: 'warning',
      confirmButtonClass: 'el-button--danger',
    })
  } catch {
    return
  }
  try {
    await deletePage(page.id)
    ElMessage.success(t('site.pageForm.deleted'))
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  }
}

// ---- 举报 ----
const reportDetailOpen = ref(false)
const detailReport = ref<Report | null>(null)

function openReportDetail(report: Report): void {
  detailReport.value = report
  reportDetailOpen.value = true
}

async function handleReport(report: Report, status: ReportStatus): Promise<void> {
  const note = status === 'rejected' ? t('site.report.noteRejected') : t('site.report.noteResolved')
  try {
    const updated = await updateReport(report.id, status, note)
    ElMessage.success(t('site.report.updated'))
    if (detailReport.value?.id === updated.id) detailReport.value = updated
    await loadReports()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  }
}

onMounted(load)
</script>

<template>
  <div class="ax-page">
    <PageHeader :title="t('site.page.title')" :description="t('site.page.description')">
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
        <el-button v-if="activeTab === 'announcements'" type="primary" :icon="Plus" @click="openCreateAnnouncement">
          {{ t('site.actions.createAnnouncement') }}
        </el-button>
        <el-button v-else-if="activeTab === 'pages'" type="primary" :icon="Plus" @click="openCreatePage">
          {{ t('site.actions.createPage') }}
        </el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="load" />

    <TableSkeleton
      v-else-if="loading && announcements.length === 0 && reports.length === 0 && pages.length === 0"
      :rows="8"
      :columns="4"
      :title="t('site.loading')"
    />

    <el-tabs v-else v-model="activeTab" class="site-tabs">
      <el-tab-pane :label="t('site.tabs.announcements', { count: announcements.length })" name="announcements">
        <EmptyState
          v-if="announcements.length === 0"
          :title="t('site.empty.announcements.title')"
          :description="t('site.empty.announcements.description')"
        />
        <div v-else class="ax-card table-card">
          <div class="ax-table-scroll">
            <el-table :data="announcements" style="width: 100%">
              <el-table-column :label="t('site.columns.title')" min-width="220">
                <template #default="{ row }">
                  <span class="site-title">
                    <el-tag v-if="row.pinned" size="small" type="danger" effect="plain">{{ t('site.pinned') }}</el-tag>
                    {{ row.title }}
                  </span>
                </template>
              </el-table-column>
              <el-table-column :label="t('site.columns.level')" width="100">
                <template #default="{ row }">
                  <el-tag size="small" :type="(levelOptions.find((l) => l.value === row.level)?.tag as any)" effect="plain">
                    {{ levelOptions.find((l) => l.value === row.level)?.label ?? row.level }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column :label="t('common.status')" width="100">
                <template #default="{ row }">
                  <el-tag size="small" :type="row.published ? 'success' : 'info'" effect="plain">
                    {{ row.published ? t('site.publishState.published') : t('site.publishState.draft') }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column :label="t('site.columns.updatedAt')" min-width="170">
                <template #default="{ row }">{{ formatDateTime(row.updated_at) }}</template>
              </el-table-column>
              <el-table-column :label="t('common.actions')" fixed="right" width="170" align="right">
                <template #default="{ row }">
                  <div class="ax-row-actions">
                    <el-button link type="primary" :icon="EditPen" @click="openEditAnnouncement(row)">{{ t('common.edit') }}</el-button>
                    <el-button link type="danger" :icon="Delete" @click="removeAnnouncement(row)">{{ t('common.delete') }}</el-button>
                  </div>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </div>
      </el-tab-pane>

      <el-tab-pane :label="t('site.tabs.reports', { count: reportTotal })" name="reports">
        <div class="site-toolbar">
          <el-radio-group
            :model-value="reportFilter"
            @update:model-value="(v: any) => onReportFilterChange(v)"
          >
            <el-radio-button value="">{{ t('site.filters.all') }}</el-radio-button>
            <el-radio-button value="pending">{{ t('site.reportStatus.pending') }}</el-radio-button>
            <el-radio-button value="resolved">{{ t('site.reportStatus.resolved') }}</el-radio-button>
            <el-radio-button value="rejected">{{ t('site.reportStatus.rejected') }}</el-radio-button>
          </el-radio-group>
        </div>
        <EmptyState
          v-if="reportTotal === 0 && !reportsLoading"
          :title="reportFilter ? t('site.empty.reports.filteredTitle') : t('site.empty.reports.title')"
          :description="reportFilter ? t('site.empty.reports.filteredDescription') : t('site.empty.reports.description')"
        >
          <el-button v-if="reportFilter" @click="clearReportFilter">{{ t('site.empty.reports.clearFilter') }}</el-button>
        </EmptyState>
        <div v-else class="ax-card table-card">
          <div class="ax-table-scroll">
            <el-table v-loading="reportsLoading" :data="reports" style="width: 100%">
              <el-table-column :label="t('site.columns.reason')" min-width="180">
                <template #default="{ row }">
                  <div class="site-cell">
                    <span>{{ row.reason }}</span>
                    <span class="site-cell__sub">{{ row.detail || '—' }}</span>
                  </div>
                </template>
              </el-table-column>
              <el-table-column :label="t('site.columns.reporter')" width="140">
                <template #default="{ row }">{{ row.reporter_name || t('site.anonymous') }}</template>
              </el-table-column>
              <el-table-column :label="t('common.status')" width="100">
                <template #default="{ row }">
                  <el-tag size="small" :type="(reportStatusOptions.find((s) => s.value === row.status)?.tag as any)" effect="plain">
                    {{ reportStatusOptions.find((s) => s.value === row.status)?.label ?? row.status }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column :label="t('site.columns.submittedAt')" min-width="170">
                <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
              </el-table-column>
              <el-table-column :label="t('common.actions')" fixed="right" width="240" align="right">
                <template #default="{ row }">
                  <div class="ax-row-actions">
                    <el-button link type="primary" @click="openReportDetail(row)">{{ t('site.tableActions.detail') }}</el-button>
                    <el-button link type="primary" :icon="Check" :disabled="row.status === 'resolved'" @click="handleReport(row, 'resolved')">
                      {{ t('site.tableActions.resolve') }}
                    </el-button>
                    <el-button link type="info" :icon="Close" :disabled="row.status === 'rejected'" @click="handleReport(row, 'rejected')">
                      {{ t('site.tableActions.reject') }}
                    </el-button>
                  </div>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </div>
        <div v-if="reportTotal > 0" class="site-pager">
          <el-pagination
            background
            layout="sizes, prev, pager, next, total"
            :page-sizes="[10, 20, 50, 100]"
            :total="reportTotal"
            :current-page="reportPage"
            :page-size="reportPageSize"
            @current-change="onReportPageChange"
            @size-change="onReportSizeChange"
          />
        </div>
      </el-tab-pane>

      <el-tab-pane :label="t('site.tabs.pages', { count: pages.length })" name="pages">
        <EmptyState
          v-if="pages.length === 0"
          :title="t('site.empty.pages.title')"
          :description="t('site.empty.pages.description', { path: '/p/{slug}' })"
        />
        <div v-else class="ax-card table-card">
          <div class="ax-table-scroll">
            <el-table :data="pages" style="width: 100%">
              <el-table-column :label="t('site.columns.title')" min-width="200">
                <template #default="{ row }">{{ row.title }}</template>
              </el-table-column>
              <el-table-column :label="t('site.columns.slug')" width="180">
                <template #default="{ row }"><code class="ax-mono">/p/{{ row.slug }}</code></template>
              </el-table-column>
              <el-table-column :label="t('common.status')" width="100">
                <template #default="{ row }">
                  <el-tag size="small" :type="row.published ? 'success' : 'info'" effect="plain">
                    {{ row.published ? t('site.publishState.published') : t('site.publishState.draft') }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column :label="t('site.columns.updatedAt')" min-width="170">
                <template #default="{ row }">{{ formatDateTime(row.updated_at) }}</template>
              </el-table-column>
              <el-table-column :label="t('common.actions')" fixed="right" width="170" align="right">
                <template #default="{ row }">
                  <div class="ax-row-actions">
                    <el-button link type="primary" :icon="EditPen" @click="openEditPage(row)">{{ t('common.edit') }}</el-button>
                    <el-button link type="danger" :icon="Delete" @click="removePage(row)">{{ t('common.delete') }}</el-button>
                  </div>
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
      :title="editingAnnId ? t('site.announcement.editTitle') : t('site.announcement.createTitle')"
      width="min(560px, 92vw)"
      append-to-body
      :close-on-click-modal="false"
      :before-close="closeAnnDialog"
      @closed="annFormRef?.clearValidate()"
    >
      <el-form ref="annFormRef" :model="annForm" :rules="annRules" label-position="top">
        <el-form-item :label="t('site.announcement.title')" prop="title">
          <el-input v-model="annForm.title" maxlength="200" show-word-limit />
        </el-form-item>
        <el-form-item :label="t('site.announcement.content')">
          <el-input v-model="annForm.content" type="textarea" :rows="5" maxlength="20000" />
        </el-form-item>
        <el-form-item :label="t('site.announcement.level')">
          <el-select v-model="annForm.level" style="width: 100%">
            <el-option v-for="l in levelOptions" :key="l.value" :label="l.label" :value="l.value" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('site.announcement.options')">
          <el-checkbox v-model="annForm.pinned">{{ t('site.announcement.pinned') }}</el-checkbox>
          <el-checkbox v-model="annForm.published">{{ t('site.announcement.publish') }}</el-checkbox>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="annOpen = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="annSaving" @click="submitAnnouncement">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- 页面编辑 -->
    <el-dialog
      v-model="pageOpen"
      :title="editingPageId ? t('site.pageForm.editTitle') : t('site.pageForm.createTitle')"
      width="min(640px, 92vw)"
      append-to-body
      :close-on-click-modal="false"
      :before-close="closePageDialog"
      @closed="pageFormRef?.clearValidate()"
    >
      <el-form ref="pageFormRef" :model="pageForm" :rules="pageRules" label-position="top">
        <el-form-item :label="t('site.pageForm.title')" prop="title">
          <el-input v-model="pageForm.title" maxlength="200" />
        </el-form-item>
        <el-form-item :label="t('site.pageForm.slug')" prop="slug">
          <el-input v-model="pageForm.slug" :placeholder="t('site.pageForm.slugPlaceholder')" maxlength="100" />
          <div class="form-hint">{{ t('site.pageForm.publicUrl', { slug: pageForm.slug || 'slug' }) }}</div>
        </el-form-item>
        <el-form-item :label="t('site.pageForm.content')">
          <el-input v-model="pageForm.content" type="textarea" :rows="10" maxlength="100000" />
        </el-form-item>
        <el-form-item :label="t('site.pageForm.publish')">
          <el-switch v-model="pageForm.published" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="pageOpen = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="pageSaving" @click="submitPage">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-drawer
      v-model="reportDetailOpen"
      :title="t('site.report.detailTitle')"
      size="min(460px, 92vw)"
      append-to-body
    >
      <template v-if="detailReport">
        <div class="report-detail__head">
          <div>
            <p class="report-detail__reason">{{ detailReport.reason }}</p>
            <p class="report-detail__hint">{{ t('site.report.idHint', { id: detailReport.id }) }}</p>
          </div>
          <el-tag
            size="small"
            :type="(reportStatusOptions.find((s) => s.value === detailReport?.status)?.tag as any)"
            effect="plain"
          >
            {{ reportStatusOptions.find((s) => s.value === detailReport?.status)?.label ?? detailReport.status }}
          </el-tag>
        </div>

        <dl class="report-detail__list">
          <div class="report-detail__row">
            <dt>{{ t('site.report.detail') }}</dt>
            <dd class="report-detail__multiline">{{ detailReport.detail || '—' }}</dd>
          </div>
          <div class="report-detail__row">
            <dt>{{ t('site.report.reporter') }}</dt>
            <dd>{{ detailReport.reporter_name || t('site.anonymous') }}</dd>
          </div>
          <div class="report-detail__row">
            <dt>{{ t('site.report.imageId') }}</dt>
            <dd class="ax-mono">{{ detailReport.image_id || '—' }}</dd>
          </div>
          <div class="report-detail__row">
            <dt>{{ t('site.report.handlerNote') }}</dt>
            <dd>{{ detailReport.handler_note || '—' }}</dd>
          </div>
          <div class="report-detail__row">
            <dt>{{ t('site.report.submittedAt') }}</dt>
            <dd>{{ formatDateTime(detailReport.created_at) }}</dd>
          </div>
          <div class="report-detail__row">
            <dt>{{ t('site.report.updatedAt') }}</dt>
            <dd>{{ formatDateTime(detailReport.updated_at) }}</dd>
          </div>
        </dl>

        <div class="report-detail__actions">
          <template v-if="detailReport.status === 'pending'">
            <el-button @click="handleReport(detailReport, 'rejected')">{{ t('site.report.reject') }}</el-button>
            <el-button type="primary" @click="handleReport(detailReport, 'resolved')">
              {{ t('site.report.markResolved') }}
            </el-button>
          </template>
          <el-button v-else @click="reportDetailOpen = false">{{ t('site.report.close') }}</el-button>
        </div>
      </template>
    </el-drawer>
  </div>
</template>

<style scoped>
.table-card {
  overflow: hidden;
}

.site-toolbar {
  margin-bottom: var(--ax-space-3);
}

.site-pager {
  display: flex;
  justify-content: flex-end;
  margin-top: var(--ax-space-3);
}

@media (max-width: 768px) {
  .site-pager {
    justify-content: center;
  }
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

.report-detail__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--ax-space-4);
  padding-bottom: var(--ax-space-4);
  border-bottom: 1px solid var(--ax-border-subtle);
}

.report-detail__reason {
  color: var(--ax-text);
  font-size: var(--ax-text-md);
  font-weight: var(--ax-weight-medium);
  overflow-wrap: anywhere;
}

.report-detail__hint {
  margin-top: var(--ax-space-1);
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.report-detail__list {
  display: flex;
  flex-direction: column;
  margin: var(--ax-space-2) 0 0;
}

.report-detail__row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--ax-space-4);
  padding: var(--ax-space-3) 0;
  border-bottom: 1px solid var(--ax-border-subtle);
}

.report-detail__row:last-child {
  border-bottom: none;
}

.report-detail__row dt {
  flex: none;
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
}

.report-detail__row dd {
  margin: 0;
  min-width: 0;
  color: var(--ax-text-2);
  font-size: var(--ax-text-sm);
  text-align: right;
  overflow-wrap: anywhere;
}

.report-detail__multiline {
  max-width: 60%;
  white-space: pre-wrap;
  text-align: left;
}

.report-detail__actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--ax-space-2);
  margin-top: var(--ax-space-5);
}
</style>

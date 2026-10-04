<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { Folder, Picture, Plus, Refresh, EditPen, Delete, Share } from '@element-plus/icons-vue'

import { createAlbum, deleteAlbum, listAlbums, updateAlbum } from '@/api/albums'
import { toApiError } from '@/api/client'
import type { Album } from '@/api/types'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'
import PageHeader from '@/components/PageHeader.vue'
import ShareDialog from '@/components/ShareDialog.vue'
import { useAuthStore } from '@/stores/auth'
import { formatDateTime, formatNumber } from '@/utils/format'

const router = useRouter()
const auth = useAuthStore()

const albums = ref<Album[]>([])
const loading = ref(false)
const errorMessage = ref('')
const removingId = ref<string | null>(null)

const dialogOpen = ref(false)
const saving = ref(false)
const editingId = ref<string | null>(null)
const formRef = ref<FormInstance>()
const form = reactive<{ name: string; intro: string; permission: 'public' | 'private' }>({
  name: '',
  intro: '',
  permission: 'private',
})

const dialogTitle = computed(() => (editingId.value ? '编辑相册' : '新建相册'))

const rules: FormRules = {
  name: [
    { required: true, message: '请输入相册名称', trigger: 'blur' },
    { min: 1, max: 100, message: '名称长度为 1 到 100 个字符', trigger: 'blur' },
  ],
  intro: [{ max: 255, message: '简介最多 255 个字符', trigger: 'blur' }],
}

async function load(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    albums.value = (await listAlbums()) ?? []
  } catch (error) {
    errorMessage.value = toApiError(error).message
  } finally {
    loading.value = false
  }
}

function openCreate(): void {
  editingId.value = null
  form.name = ''
  form.intro = ''
  form.permission = 'private'
  dialogOpen.value = true
  void nextTick(() => formRef.value?.clearValidate())
}

function openEdit(album: Album): void {
  editingId.value = album.id
  form.name = album.name
  form.intro = album.intro
  form.permission = album.permission
  dialogOpen.value = true
  void nextTick(() => formRef.value?.clearValidate())
}

async function submit(): Promise<void> {
  const instance = formRef.value
  if (!instance || saving.value) return
  const valid = await instance.validate().catch(() => false)
  if (!valid) return

  saving.value = true
  try {
    const input = { name: form.name.trim(), intro: form.intro.trim(), permission: form.permission }
    if (editingId.value) {
      await updateAlbum(editingId.value, input)
      ElMessage.success('相册已更新')
    } else {
      await createAlbum(input)
      ElMessage.success('相册已创建')
    }
    dialogOpen.value = false
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    saving.value = false
  }
}

async function remove(album: Album): Promise<void> {
  try {
    await ElMessageBox.confirm(
      `删除相册「${album.name}」后，其中的图片会保留但不再归入任何相册。`,
      '删除相册',
      {
        confirmButtonText: '删除',
        cancelButtonText: '取消',
        type: 'warning',
        confirmButtonClass: 'el-button--danger',
      },
    )
  } catch {
    return
  }
  removingId.value = album.id
  try {
    await deleteAlbum(album.id)
    ElMessage.success('相册已删除')
    await load()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    removingId.value = null
  }
}

function viewImages(album: Album): void {
  void router.push({ name: 'user-images', query: { album_id: album.id } })
}

// 分享
const shareOpen = ref(false)
const shareTarget = ref<Album | null>(null)

function openShare(album: Album): void {
  if (!auth.hasFeature('share')) {
    ElMessage.warning('当前角色未开启分享功能')
    return
  }
  shareTarget.value = album
  shareOpen.value = true
}

onMounted(load)
</script>

<template>
  <div class="ax-page">
    <PageHeader title="相册" description="用相册归类图片，便于管理和浏览。">
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
        <el-button type="primary" :icon="Plus" @click="openCreate">新建相册</el-button>
      </template>
    </PageHeader>

    <ErrorState v-if="errorMessage" :message="errorMessage" @retry="load" />

    <div v-else-if="loading && albums.length === 0" class="ax-card" aria-busy="true">
      <div class="ax-card__body"><el-skeleton :rows="5" animated /></div>
    </div>

    <EmptyState
      v-else-if="albums.length === 0"
      title="还没有相册"
      description="创建相册后，即可在图片管理页把图片归入其中。"
    >
      <el-button type="primary" :icon="Plus" @click="openCreate">新建相册</el-button>
    </EmptyState>

    <div v-else class="ax-card table-card">
      <div class="ax-table-scroll">
        <el-table :data="albums" style="width: 100%">
          <el-table-column prop="name" label="名称" min-width="180">
            <template #default="{ row }">
              <span class="album-name">
                <el-icon :size="14"><Folder /></el-icon>
                {{ row.name }}
              </span>
            </template>
          </el-table-column>
          <el-table-column label="图片数" width="110">
            <template #default="{ row }">{{ formatNumber(row.image_count) }}</template>
          </el-table-column>
          <el-table-column label="可见性" width="100">
            <template #default="{ row }">
              <el-tag size="small" :type="row.permission === 'public' ? 'success' : 'info'" effect="plain">
                {{ row.permission === 'public' ? '公开' : '私密' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="intro" label="简介" min-width="200">
            <template #default="{ row }">
              <span v-if="row.intro">{{ row.intro }}</span>
              <span v-else class="ax-muted">—</span>
            </template>
          </el-table-column>
          <el-table-column label="创建时间" min-width="170">
            <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column label="操作" width="270" align="right">
            <template #default="{ row }">
              <div class="ax-row-actions">
                <el-button link type="primary" :icon="Picture" @click="viewImages(row)">查看图片</el-button>
                <el-button
                  v-if="auth.hasFeature('share')"
                  link
                  type="primary"
                  :icon="Share"
                  @click="openShare(row)"
                >
                  分享
                </el-button>
                <el-button link type="primary" :icon="EditPen" @click="openEdit(row)">编辑</el-button>
                <el-button
                  link
                  type="danger"
                  :icon="Delete"
                  :loading="removingId === row.id"
                  @click="remove(row)"
                >
                  删除
                </el-button>
              </div>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </div>

    <el-dialog
      v-model="dialogOpen"
      :title="dialogTitle"
      width="min(440px, 92vw)"
      append-to-body
      :close-on-click-modal="false"
      @closed="formRef?.clearValidate()"
    >
      <el-form
        ref="formRef"
        :model="form"
        :rules="rules"
        label-position="top"
        @submit.prevent="submit"
      >
        <el-form-item label="名称" prop="name">
          <el-input
            v-model="form.name"
            placeholder="例如：旅行 / 壁纸"
            maxlength="100"
            show-word-limit
          />
        </el-form-item>
        <el-form-item label="简介" prop="intro">
          <el-input
            v-model="form.intro"
            type="textarea"
            :rows="3"
            placeholder="选填"
            maxlength="255"
            show-word-limit
          />
        </el-form-item>
        <el-form-item label="可见性">
          <el-radio-group v-model="form.permission">
            <el-radio-button value="private">私密</el-radio-button>
            <el-radio-button value="public">公开</el-radio-button>
          </el-radio-group>
          <div class="form-hint">公开后，任何人无需登录即可浏览相册及其中的图片。</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogOpen = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submit">保存</el-button>
      </template>
    </el-dialog>

    <ShareDialog
      v-model="shareOpen"
      target-type="album"
      :target-id="shareTarget?.id ?? ''"
      :target-label="shareTarget?.name ?? ''"
    />
  </div>
</template>

<style scoped>
.table-card {
  overflow: hidden;
}

.album-name {
  display: inline-flex;
  align-items: center;
  gap: var(--ax-space-2);
  color: var(--ax-text);
  font-weight: var(--ax-weight-medium);
}

.form-hint {
  margin-top: 4px;
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
  line-height: 1.5;
}
</style>

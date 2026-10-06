<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage, type UploadFile } from 'element-plus'

import {
  installPlugin,
  installPluginArchive,
  listInstalledPlugins,
  listPluginRegistry,
  reloadPlugin,
  removePlugin,
  setPluginEnabled,
  type PluginMarketEntry,
  type PluginStatus,
} from '@/api/billing'
import { toApiError } from '@/api/client'
import PageHeader from '@/components/PageHeader.vue'
import PluginConfigForm from '@/components/PluginConfigForm.vue'

const installed = ref<PluginStatus[]>([])
const installedLoading = ref(false)
const registry = ref<PluginMarketEntry[]>([])
const registryConfigured = ref(true)
const marketLoading = ref(false)
const registryError = ref('')
const installing = ref(false)
const toggling = ref('')
const removing = ref('')

const installUrl = ref('')
const installSha = ref('')
const installSig = ref('')

const uploadFile = ref<File | null>(null)
const uploadSha = ref('')
const uploadSig = ref('')

const configOpen = ref(false)
const configName = ref('')

/** 载入已安装插件及其状态。 */
async function loadInstalled(): Promise<void> {
  installedLoading.value = true
  try {
    installed.value = (await listInstalledPlugins()).items ?? []
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    installedLoading.value = false
  }
}

/** 拉取插件市场索引。 */
async function loadRegistry(): Promise<void> {
  marketLoading.value = true
  registryError.value = ''
  try {
    const res = await listPluginRegistry()
    registry.value = res.items ?? []
    registryConfigured.value = res.configured
  } catch (error) {
    registry.value = []
    registryError.value = toApiError(error).message
  } finally {
    marketLoading.value = false
  }
}

/** 判断插件是否已安装。 */
function isInstalled(name: string): boolean {
  return installed.value.some((p) => p.name === name)
}

/** 从市场安装。 */
async function installFromMarket(entry: PluginMarketEntry): Promise<void> {
  installing.value = true
  try {
    await installPlugin({ name: entry.name, version: entry.version })
    ElMessage.success(`已安装 ${entry.name}`)
    await loadInstalled()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    installing.value = false
  }
}

/** 按 URL 安装。 */
async function installFromURL(): Promise<void> {
  if (!installUrl.value.trim()) {
    ElMessage.warning('请输入插件归档地址')
    return
  }
  installing.value = true
  try {
    await installPlugin({
      url: installUrl.value.trim(),
      sha256: installSha.value.trim(),
      signature: installSig.value.trim(),
    })
    ElMessage.success('安装成功')
    installUrl.value = ''
    installSha.value = ''
    installSig.value = ''
    await loadInstalled()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    installing.value = false
  }
}

/** 记录选择的本地归档文件。 */
function onFileChange(file: UploadFile): void {
  uploadFile.value = (file.raw as File) ?? null
}

/** 清空选择的文件。 */
function onFileRemove(): void {
  uploadFile.value = null
}

/** 上传本地归档安装。 */
async function installFromUpload(): Promise<void> {
  if (!uploadFile.value) {
    ElMessage.warning('请选择插件归档（zip）')
    return
  }
  installing.value = true
  try {
    await installPluginArchive(
      uploadFile.value,
      uploadSha.value.trim(),
      uploadSig.value.trim(),
    )
    ElMessage.success('安装成功')
    uploadFile.value = null
    uploadSha.value = ''
    uploadSig.value = ''
    await loadInstalled()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    installing.value = false
  }
}

/** 启用或暂停插件。 */
async function toggleEnabled(row: PluginStatus): Promise<void> {
  toggling.value = row.name
  try {
    await setPluginEnabled(row.name, !row.enabled)
    ElMessage.success(row.enabled ? `已暂停 ${row.name}` : `已启用 ${row.name}`)
    await loadInstalled()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    toggling.value = ''
  }
}

/** 卸载插件。 */
async function removeByName(name: string): Promise<void> {
  removing.value = name
  try {
    await removePlugin(name)
    ElMessage.success(`已卸载 ${name}`)
    await loadInstalled()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    removing.value = ''
  }
}

/** 重新加载插件。 */
async function reloadByName(name: string): Promise<void> {
  try {
    await reloadPlugin(name)
    ElMessage.success(`已重新加载 ${name}`)
    await loadInstalled()
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  }
}

/** 打开插件配置对话框。 */
function openConfig(row: PluginStatus): void {
  configName.value = row.name
  configOpen.value = true
}

onMounted(() => {
  void loadInstalled()
  void loadRegistry()
})
</script>

<template>
  <div>
    <PageHeader title="插件市场" description="在线安装、配置并管理运行时插件（WASM / 进程）。">
      <template #actions>
        <el-button :loading="marketLoading" @click="loadRegistry">刷新市场</el-button>
      </template>
    </PageHeader>

    <el-alert
      v-if="registryError"
      :title="registryError"
      type="warning"
      :closable="false"
      show-icon
      style="margin-bottom: 16px"
    />

    <article class="ax-card">
      <header class="ax-card__head">
        <h2 class="ax-card__title">已安装插件</h2>
        <el-tag size="small" type="info" effect="plain">{{ installed.length }} 个</el-tag>
      </header>
      <div class="ax-card__body">
        <el-table v-loading="installedLoading" :data="installed" size="small" empty-text="暂无已安装插件">
          <el-table-column label="名称" min-width="180">
            <template #default="{ row }">
              <span>{{ row.title || row.name }}</span>
              <span class="plugin-key">{{ row.name }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="category" label="类别" width="130" />
          <el-table-column label="运行时" width="90">
            <template #default="{ row }">{{ row.runtime }}</template>
          </el-table-column>
          <el-table-column label="状态" width="120">
            <template #default="{ row }">
              <el-tag size="small" :type="row.enabled ? 'success' : 'info'" effect="plain">
                {{ row.enabled ? '已启用' : '已暂停' }}
              </el-tag>
              <el-tag v-if="row.configured" size="small" type="warning" effect="plain" class="plugin-tag">
                已配置
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="330">
            <template #default="{ row }">
              <el-button
                size="small"
                :type="row.enabled ? 'warning' : 'success'"
                :loading="toggling === row.name"
                @click="toggleEnabled(row)"
              >
                {{ row.enabled ? '暂停' : '启用' }}
              </el-button>
              <el-button size="small" @click="openConfig(row)">配置</el-button>
              <el-button size="small" @click="reloadByName(row.name)">重新加载</el-button>
              <el-button
                size="small"
                type="danger"
                :loading="removing === row.name"
                @click="removeByName(row.name)"
              >
                卸载
              </el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </article>

    <article class="ax-card">
      <header class="ax-card__head">
        <h2 class="ax-card__title">本地上传安装</h2>
        <el-tag size="small" type="info" effect="plain">zip 归档</el-tag>
      </header>
      <div class="ax-card__body">
        <el-upload
          drag
          :auto-upload="false"
          :limit="1"
          accept=".zip"
          :on-change="onFileChange"
          :on-remove="onFileRemove"
        >
          <div class="upload-hint">将插件 zip 拖到此处，或<em>点击选择</em></div>
        </el-upload>
        <div class="install-row">
          <el-input v-model="uploadSha" placeholder="sha256（可选）" style="max-width: 300px" />
          <el-input
            v-model="uploadSig"
            placeholder="Ed25519 签名 base64（可选）"
            style="max-width: 340px"
          />
        </div>
        <div class="install-actions">
          <el-button type="primary" :loading="installing" @click="installFromUpload">上传并安装</el-button>
        </div>
      </div>
    </article>

    <article class="ax-card">
      <header class="ax-card__head">
        <h2 class="ax-card__title">市场索引</h2>
        <el-tag size="small" type="info" effect="plain">{{ registry.length }} 个</el-tag>
      </header>
      <div class="ax-card__body">
        <el-table
          v-loading="marketLoading"
          :data="registry"
          size="small"
          :empty-text="registryConfigured ? '索引为空' : '未配置插件索引（plugins.index_url）'"
        >
          <el-table-column prop="name" label="名称" min-width="150" />
          <el-table-column prop="version" label="版本" width="90" />
          <el-table-column prop="runtime" label="运行时" width="90" />
          <el-table-column prop="description" label="说明" />
          <el-table-column label="操作" width="110">
            <template #default="{ row }">
              <el-button
                v-if="!isInstalled(row.name)"
                type="primary"
                size="small"
                :loading="installing"
                @click="installFromMarket(row)"
              >
                安装
              </el-button>
              <el-tag v-else size="small" type="success">已安装</el-tag>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </article>

    <article class="ax-card">
      <header class="ax-card__head">
        <h2 class="ax-card__title">按 URL 安装</h2>
      </header>
      <div class="ax-card__body">
        <el-form label-position="top" @submit.prevent>
          <el-form-item label="归档地址（zip）">
            <el-input v-model="installUrl" placeholder="https://example.com/smsbao-0.1.0.zip" />
          </el-form-item>
          <div class="install-row">
            <el-input v-model="installSha" placeholder="sha256（可选）" style="max-width: 300px" />
            <el-input
              v-model="installSig"
              placeholder="Ed25519 签名 base64（可选）"
              style="max-width: 340px"
            />
          </div>
        </el-form>
        <div class="install-actions">
          <el-button type="primary" :loading="installing" @click="installFromURL">安装</el-button>
        </div>
      </div>
    </article>

    <el-dialog v-model="configOpen" :title="`配置插件：${configName}`" width="640px">
      <PluginConfigForm v-if="configOpen" :name="configName" @saved="loadInstalled" />
    </el-dialog>
  </div>
</template>

<style scoped>
.plugin-key {
  margin-left: var(--ax-space-2);
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}

.plugin-tag {
  margin-left: 4px;
}

.upload-hint {
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
}

.upload-hint em {
  color: var(--ax-accent-bright);
  font-style: normal;
}

.install-row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--ax-space-2);
  margin-top: var(--ax-space-3);
}

.install-actions {
  display: flex;
  gap: var(--ax-space-2);
  margin-top: var(--ax-space-3);
}
</style>

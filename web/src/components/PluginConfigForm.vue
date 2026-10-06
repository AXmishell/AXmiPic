<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'

import {
  getPluginConfig,
  testPlugin,
  updatePluginConfig,
  type PluginConfig,
} from '@/api/billing'
import { toApiError } from '@/api/client'

const props = defineProps<{ name: string }>()
const emit = defineEmits<{ (event: 'saved'): void }>()

const config = ref<PluginConfig | null>(null)
const values = reactive<Record<string, string>>({})
const secretInputs = reactive<Record<string, string>>({})
const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const testTo = ref('')
const testBody = ref('')

/** 当前插件的配置字段定义。 */
const fields = computed(() => config.value?.fields ?? [])

/** 载入插件配置（秘钥仅返回是否已设置）。 */
async function load(): Promise<void> {
  if (!props.name) {
    config.value = null
    return
  }
  loading.value = true
  try {
    const cfg = await getPluginConfig(props.name)
    config.value = cfg
    for (const key of Object.keys(values)) delete values[key]
    for (const key of Object.keys(secretInputs)) delete secretInputs[key]
    Object.assign(values, cfg.values ?? {})
  } catch (error) {
    ElMessage.error(toApiError(error).message)
    config.value = null
  } finally {
    loading.value = false
  }
}

watch(() => props.name, load, { immediate: true })

/** 保存配置并通知父组件。 */
async function save(): Promise<void> {
  if (!props.name) return
  const payload: Record<string, string> = { ...values }
  for (const [key, value] of Object.entries(secretInputs)) {
    if (value) payload[key] = value
  }
  saving.value = true
  try {
    await updatePluginConfig(props.name, payload)
    ElMessage.success('插件配置已保存并即时生效')
    await load()
    emit('saved')
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    saving.value = false
  }
}

/** 用当前配置发送一条测试短信。 */
async function test(): Promise<void> {
  if (!props.name) return
  if (!testTo.value.trim()) {
    ElMessage.warning('请填写测试接收号码')
    return
  }
  testing.value = true
  try {
    await testPlugin(props.name, { to: testTo.value.trim(), body: testBody.value })
    ElMessage.success('测试已发送，请查收')
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    testing.value = false
  }
}

defineExpose({ reload: load })
</script>

<template>
  <div v-loading="loading" class="plugin-config">
    <el-form label-position="top" @submit.prevent>
      <div class="plugin-config__grid">
        <el-form-item v-for="field in fields" :key="field.key" :label="field.label">
          <el-switch v-if="field.type === 'bool'" v-model="values[field.key]" />
          <el-select
            v-else-if="field.type === 'select'"
            v-model="values[field.key]"
            style="width: 100%"
          >
            <el-option v-for="opt in field.options ?? []" :key="opt" :label="opt" :value="opt" />
          </el-select>
          <el-input
            v-else-if="field.secret"
            v-model="secretInputs[field.key]"
            type="password"
            show-password
            :placeholder="config?.secrets?.[field.key] ? '已设置，留空保持不变' : '请输入'"
          />
          <el-input
            v-else
            v-model="values[field.key]"
            :placeholder="field.help || field.default || ''"
          />
        </el-form-item>
      </div>
    </el-form>

    <div class="plugin-config__actions">
      <el-input v-model="testTo" placeholder="测试接收号码" style="max-width: 200px" />
      <el-input v-model="testBody" placeholder="测试内容（可选）" style="max-width: 260px" />
      <el-button :loading="testing" @click="test">测试发送</el-button>
      <el-button type="primary" :loading="saving" @click="save">保存配置</el-button>
    </div>
  </div>
</template>

<style scoped>
.plugin-config__grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 0 var(--ax-space-4);
}

.plugin-config__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--ax-space-2);
  align-items: center;
}
</style>

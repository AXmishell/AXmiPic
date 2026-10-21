<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'

import { getPluginConfig, updatePluginConfig, type PluginConfig } from '@/api/billing'
import { toApiError } from '@/api/client'

const props = defineProps<{ name: string }>()
const emit = defineEmits<{ (event: 'saved'): void }>()

const config = ref<PluginConfig | null>(null)
const values = reactive<Record<string, string>>({})
const secretInputs = reactive<Record<string, string>>({})
const loading = ref(false)
const saving = ref(false)

/** 当前插件的配置字段定义。 */
const fields = computed(() => config.value?.fields ?? [])

/** 读取某字段当前值；未填写时回退到字段默认值。 */
function fieldValue(key: string): string {
  const current = values[key]
  if (current !== undefined && current !== '') return current
  return fields.value.find((field) => field.key === key)?.default ?? ''
}

/** 根据 visible_when 过滤当前应显示的字段。 */
const visibleFields = computed(() =>
  fields.value.filter((field) => {
    const condition = field.visible_when
    if (!condition) return true
    return (condition.values ?? []).includes(fieldValue(condition.key))
  }),
)

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
    // 插件配置在后端统一是字符串；开关字段必须归一化为 "true"/"false"，
    // 否则 Element Plus 会把它们改成布尔值，提交时导致 JSON 类型不匹配。
    for (const field of cfg.fields ?? []) {
      if (field.type === 'bool') {
        values[field.key] = values[field.key] === 'true' ? 'true' : 'false'
      }
    }
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
  const visibleKeys = new Set(visibleFields.value.map((field) => field.key))
  const payload: Record<string, string> = {}
  for (const [key, value] of Object.entries(values)) {
    if (visibleKeys.has(key)) payload[key] = value
  }
  for (const [key, value] of Object.entries(secretInputs)) {
    if (value && visibleKeys.has(key)) payload[key] = value
  }
  // 再兜底一次，确保开关字段始终以字符串提交。
  for (const field of visibleFields.value) {
    if (field.type === 'bool') {
      payload[field.key] = payload[field.key] === 'true' ? 'true' : 'false'
    }
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

defineExpose({ reload: load })
</script>

<template>
  <div v-loading="loading" class="plugin-config">
    <el-form label-position="top" @submit.prevent>
      <div class="plugin-config__grid">
        <el-form-item v-for="field in visibleFields" :key="field.key" :label="field.label">
          <el-switch
            v-if="field.type === 'bool'"
            v-model="values[field.key]"
            active-value="true"
            inactive-value="false"
          />
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

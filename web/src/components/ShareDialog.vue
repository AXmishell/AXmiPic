<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'

import { toApiError } from '@/api/client'
import { createShare } from '@/api/shares'
import type { Share, ShareTargetType } from '@/api/types'
import CopyField from '@/components/CopyField.vue'
import { useAuthStore } from '@/stores/auth'

const props = defineProps<{
  modelValue: boolean
  targetType: ShareTargetType
  targetId: string
  targetLabel?: string
}>()

const emit = defineEmits<{ (event: 'update:modelValue', value: boolean): void }>()

const auth = useAuthStore()
const form = reactive({ password: '', expiresInHours: 0, maxViews: 0 })
const creating = ref(false)
const result = ref<Share | null>(null)

const open = computed({
  get: () => props.modelValue,
  set: (value: boolean) => emit('update:modelValue', value),
})
const allowPassword = computed(() => auth.hasFeature('share_password'))
const targetName = computed(() => (props.targetType === 'album' ? '相册' : '图片'))

watch(
  () => props.modelValue,
  (value) => {
    if (value) {
      form.password = ''
      form.expiresInHours = 0
      form.maxViews = 0
      result.value = null
    }
  },
)

async function submit(): Promise<void> {
  if (creating.value) return
  creating.value = true
  try {
    result.value = await createShare({
      target_type: props.targetType,
      target_id: props.targetId,
      password: allowPassword.value ? form.password.trim() : '',
      expires_in_hours: form.expiresInHours > 0 ? form.expiresInHours : undefined,
      max_views: form.maxViews > 0 ? form.maxViews : undefined,
    })
    ElMessage.success('分享链接已创建')
  } catch (error) {
    ElMessage.error(toApiError(error).message)
  } finally {
    creating.value = false
  }
}
</script>

<template>
  <el-dialog v-model="open" title="创建分享链接" width="min(520px, 92vw)" append-to-body>
    <p class="share-target">
      分享{{ targetName }}：<strong>{{ targetLabel || targetId }}</strong>
    </p>

    <template v-if="!result">
      <el-form label-position="top">
        <el-form-item v-if="allowPassword" label="访问密码（可选）">
          <el-input v-model="form.password" type="password" show-password placeholder="留空表示无需密码" />
        </el-form-item>
        <el-form-item label="有效期（小时，可选）">
          <el-input-number v-model="form.expiresInHours" :min="0" :max="8760" />
          <span class="form-hint">0 表示长期有效</span>
        </el-form-item>
        <el-form-item label="最大访问次数（可选）">
          <el-input-number v-model="form.maxViews" :min="0" />
          <span class="form-hint">0 表示不限次数</span>
        </el-form-item>
      </el-form>
    </template>

    <template v-else>
      <CopyField label="分享链接" :value="result.url" />
      <div class="share-meta">
        <el-tag v-if="result.has_password" size="small" type="warning" effect="plain">需要密码</el-tag>
        <el-tag v-else size="small" type="info" effect="plain">无需密码</el-tag>
        <el-tag v-if="result.expires_at" size="small" effect="plain">到期时间 {{ result.expires_at }}</el-tag>
        <el-tag v-if="result.max_views > 0" size="small" effect="plain">最多 {{ result.max_views }} 次</el-tag>
      </div>
    </template>

    <template #footer>
      <el-button @click="open = false">关闭</el-button>
      <el-button v-if="!result" type="primary" :loading="creating" @click="submit">创建分享</el-button>
      <el-button v-else type="primary" @click="submit">再创建一个</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.share-target {
  margin: 0 0 var(--ax-space-3);
  color: var(--ax-text-2);
  font-size: var(--ax-text-sm);
}

.share-meta {
  display: flex;
  flex-wrap: wrap;
  gap: var(--ax-space-2);
  margin-top: var(--ax-space-3);
}

.form-hint {
  margin-left: var(--ax-space-2);
  color: var(--ax-text-4);
  font-size: var(--ax-text-xs);
}
</style>

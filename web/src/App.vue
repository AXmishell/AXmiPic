<script setup lang="ts">
import { computed, watchEffect } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import en from 'element-plus/es/locale/lang/en'
import zhCn from 'element-plus/es/locale/lang/zh-cn'

import LocaleSwitcher from '@/components/LocaleSwitcher.vue'
import { useLocaleStore } from '@/stores/locale'

const route = useRoute()
const localeStore = useLocaleStore()
const { t } = useI18n()

const elementLocale = computed(() => (localeStore.locale === 'en-US' ? en : zhCn))

/** 标题随路由与语言变化，避免切换语言后仍显示旧文案。 */
watchEffect(() => {
  const titleKey = route.meta.title
  document.title = titleKey ? `${t(titleKey)} · AXmiPic` : t('appTitle')
  const description = document.querySelector<HTMLMetaElement>('meta[name="description"]')
  if (description) description.content = t('appDescription')
})

/** 控制台布局的顶栏自带切换器；公开页面使用悬浮切换器。 */
const showFloatingSwitcher = computed(
  () =>
    !route.meta.requiresAuth &&
    !route.meta.requiresAdmin &&
    // 首页顶栏自带语言切换器，避免与悬浮入口重复。
    route.name !== 'home',
)
</script>

<template>
  <el-config-provider :locale="elementLocale">
    <router-view />
    <LocaleSwitcher v-if="showFloatingSwitcher" variant="floating" />
  </el-config-provider>
</template>

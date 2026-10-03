<script setup lang="ts">
import { useRouter } from 'vue-router'

const router = useRouter()
</script>

<template>
  <main class="not-found">
    <p class="not-found__code">404</p>
    <h1 class="not-found__title">页面不存在</h1>
    <p class="not-found__desc">你访问的地址不存在或已被移除。</p>
    <el-button type="primary" @click="router.replace('/')">返回首页</el-button>
  </main>
</template>

<style scoped>
.not-found {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 100dvh;
  padding: var(--ax-space-8);
  text-align: center;
}

.not-found__code {
  color: var(--ax-text-4);
  font-family: var(--ax-font-mono);
  font-size: 64px;
  font-weight: var(--ax-weight-semibold);
  letter-spacing: -0.04em;
  line-height: 1;
}

.not-found__title {
  margin-top: var(--ax-space-4);
  font-size: var(--ax-text-xl);
}

.not-found__desc {
  margin: var(--ax-space-2) 0 var(--ax-space-6);
  color: var(--ax-text-3);
  font-size: var(--ax-text-sm);
}
</style>

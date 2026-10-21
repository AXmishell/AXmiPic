<script setup lang="ts">
withDefaults(
  defineProps<{
    rows?: number
    columns?: number
    title?: string
  }>(),
  {
    rows: 6,
    columns: 5,
    title: '加载中',
  },
)
</script>

<template>
  <div class="table-skeleton ax-card" aria-busy="true" :aria-label="title">
    <div class="table-skeleton__head">
      <el-skeleton-item variant="text" class="table-skeleton__title" />
      <el-skeleton-item variant="text" class="table-skeleton__action" />
    </div>
    <div class="table-skeleton__body">
      <div v-for="row in rows" :key="row" class="table-skeleton__row">
        <el-skeleton-item
          v-for="column in columns"
          :key="column"
          variant="text"
          class="table-skeleton__cell"
          :class="{ 'is-primary': column === 1 }"
          :style="{ width: `${column === 1 ? 28 : 12 + ((row + column) % 3) * 3}%` }"
        />
      </div>
    </div>
  </div>
</template>

<style scoped>
.table-skeleton {
  overflow: hidden;
}

.table-skeleton__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 52px;
  padding: var(--ax-space-3) var(--ax-space-5);
  border-bottom: 1px solid var(--ax-border-subtle);
}

.table-skeleton__title {
  width: 112px;
  height: 12px;
}

.table-skeleton__action {
  width: 44px;
  height: 12px;
}

.table-skeleton__body {
  display: flex;
  flex-direction: column;
}

.table-skeleton__row {
  display: flex;
  align-items: center;
  gap: 8%;
  min-height: 56px;
  padding: 0 var(--ax-space-5);
  border-bottom: 1px solid var(--ax-border-subtle);
}

.table-skeleton__row:last-child {
  border-bottom: none;
}

.table-skeleton__cell {
  flex: none;
  height: 11px;
}

.table-skeleton__cell.is-primary {
  height: 13px;
}
</style>

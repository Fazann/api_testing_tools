<script setup lang="ts">
import DataTable from '@/components/base/DataTable.vue';
import type { DiffEntry } from '@/types';

defineProps<{ diffs: DiffEntry[] }>();
const COLUMNS = [
  { key: 'kind', label: 'Change', class: 'kind' },
  { key: 'path', label: 'Path', class: 'mono' },
  { key: 'before', label: 'Baseline', class: 'mono' },
  { key: 'after', label: 'Actual', class: 'mono' },
];
const labels: Record<string, string> = {
  added: 'Added',
  removed: 'Removed',
  changed: 'Changed',
  type_changed: 'Type changed',
};
</script>

<template>
  <DataTable class="diff" :columns="COLUMNS" :rows="diffs" :row-class="(d) => d.kind">
    <template #cell-kind="{ row }">{{ labels[row.kind] ?? row.kind }}</template>
    <template #cell-before="{ row }">{{ row.before ?? '' }}</template>
    <template #cell-after="{ row }">{{ row.after ?? '' }}</template>
  </DataTable>
</template>

<style scoped>
.diff :deep(tr).added td {
  background: var(--add-bg);
}
.diff :deep(tr).removed td {
  background: var(--rem-bg);
}
.diff :deep(tr).changed td,
.diff :deep(tr).type_changed td {
  background: var(--chg-bg);
}
.diff :deep(.kind) {
  white-space: nowrap;
  font-weight: 600;
}
</style>

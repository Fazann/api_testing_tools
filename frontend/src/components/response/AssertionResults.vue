<script setup lang="ts">
import DataTable from '@/components/base/DataTable.vue';
import { KIND_LABELS, OPERATOR_LABELS } from '@/services/model';
import type { AssertionResult } from '@/types';

defineProps<{ results: AssertionResult[] }>();
const COLUMNS = [
  { key: 'passed', label: 'Result' },
  { key: 'kind', label: 'Check' },
  { key: 'expected', label: 'Expected', class: 'mono' },
  { key: 'actual', label: 'Actual', class: 'mono' },
  { key: 'message', label: 'Message' },
];
</script>

<template>
  <DataTable v-if="results.length" :columns="COLUMNS" :rows="results">
    <template #cell-passed="{ row }">
      <span :class="row.passed ? 'pass' : 'fail'">{{ row.passed ? '✔ Pass' : '✘ Fail' }}</span>
    </template>
    <template #cell-kind="{ row }">
      {{ row.kind === 'script' ? 'Script test' : row.kind === 'socket_step' ? 'Socket step' : KIND_LABELS[row.kind] }}
      <code v-if="row.path">{{ row.path }}</code>
      <span v-if="row.operator" class="muted"> {{ OPERATOR_LABELS[row.operator] ?? row.operator }}</span>
      <div v-if="row.description" class="muted small">{{ row.description }}</div>
    </template>
  </DataTable>
  <p v-else class="muted">No tests ran. Add tests in the Tests tab, or pm.test() in a post-response script.</p>
</template>

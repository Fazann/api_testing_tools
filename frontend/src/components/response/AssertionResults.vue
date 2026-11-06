<script setup lang="ts">
import DataTable from '@/components/base/DataTable.vue';
import { KIND_LABELS, OPERATOR_LABELS } from '@/services/model';
import type { AssertionResult } from '@/types';

defineProps<{ results: AssertionResult[] }>();
// Kinds that only appear in results, not in a request's saved assertions.
const SPECIAL_KINDS: Record<string, string> = { script: 'Script test', socket_step: 'Socket step', rule: 'Rule' };
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
      {{ SPECIAL_KINDS[row.kind] ?? KIND_LABELS[row.kind as keyof typeof KIND_LABELS] }}
      <code v-if="row.path">{{ row.path }}</code>
      <span v-if="row.operator" class="muted"> {{ OPERATOR_LABELS[row.operator] ?? row.operator }}</span>
      <div v-if="row.description" class="muted small">{{ row.description }}</div>
    </template>
  </DataTable>
  <p v-else class="muted">No tests ran. Add tests in the Tests tab, or pm.test() in a post-response script.</p>
</template>

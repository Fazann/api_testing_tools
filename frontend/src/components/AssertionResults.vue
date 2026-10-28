<script setup lang="ts">
import { KIND_LABELS, OPERATOR_LABELS } from '../services/model';
import type { AssertionResult } from '../types';

defineProps<{ results: AssertionResult[] }>();
</script>

<template>
  <table v-if="results.length" class="grid">
    <thead>
      <tr>
        <th>Result</th>
        <th>Check</th>
        <th>Expected</th>
        <th>Actual</th>
        <th>Message</th>
      </tr>
    </thead>
    <tbody>
      <tr v-for="(r, i) in results" :key="i">
        <td>
          <span :class="r.passed ? 'pass' : 'fail'">{{ r.passed ? '✔ Pass' : '✘ Fail' }}</span>
        </td>
        <td>
          {{ KIND_LABELS[r.kind] ?? r.kind }}
          <code v-if="r.path">{{ r.path }}</code>
          <span v-if="r.operator" class="muted"> {{ OPERATOR_LABELS[r.operator] ?? r.operator }}</span>
          <div v-if="r.description" class="muted small">{{ r.description }}</div>
        </td>
        <td class="mono">{{ r.expected }}</td>
        <td class="mono">{{ r.actual }}</td>
        <td>{{ r.message }}</td>
      </tr>
    </tbody>
  </table>
  <p v-else class="muted">No tests ran. Add tests in the Tests tab.</p>
</template>

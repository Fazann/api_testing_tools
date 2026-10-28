<script setup lang="ts">
import type { DiffEntry } from '../types';

defineProps<{ diffs: DiffEntry[] }>();
const labels: Record<string, string> = {
  added: 'Added',
  removed: 'Removed',
  changed: 'Changed',
  type_changed: 'Type changed',
};
</script>

<template>
  <table class="grid diff">
    <thead>
      <tr>
        <th>Change</th>
        <th>Path</th>
        <th>Baseline</th>
        <th>Actual</th>
      </tr>
    </thead>
    <tbody>
      <tr v-for="(d, i) in diffs" :key="i" :class="d.kind">
        <td class="kind">{{ labels[d.kind] ?? d.kind }}</td>
        <td class="mono">{{ d.path }}</td>
        <td class="mono">{{ d.before ?? '' }}</td>
        <td class="mono">{{ d.after ?? '' }}</td>
      </tr>
    </tbody>
  </table>
</template>

<style scoped>
.diff tr.added td {
  background: var(--add-bg);
}
.diff tr.removed td {
  background: var(--rem-bg);
}
.diff tr.changed td,
.diff tr.type_changed td {
  background: var(--chg-bg);
}
.kind {
  white-space: nowrap;
  font-weight: 600;
}
</style>

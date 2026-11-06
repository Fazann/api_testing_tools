<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';

import BaseButton from '@/components/base/BaseButton.vue';
import EmptyState from '@/components/base/EmptyState.vue';
import SelectableList from '@/components/base/SelectableList.vue';
import SelectField from '@/components/base/SelectField.vue';
import StatusBadge from '@/components/base/StatusBadge.vue';
import TextField from '@/components/base/TextField.vue';
import RunSummary from '@/components/run/RunSummary.vue';
import { useAsyncAction } from '@/composables/useAsyncAction';
import { api } from '@/services/api';
import { useUi } from '@/stores/ui';
import { useWorkspace } from '@/stores/workspace';
import type { Run } from '@/types';
import { formatDuration, formatTime } from '@/utils/format';

const ui = useUi();
const ws = useWorkspace();

const collectionId = ref('');
const requestId = ref('');
const status = ref('');
const from = ref('');
const to = ref('');
const runs = ref<Run[]>([]);
const selected = ref<Run | null>(null);
const { busy: loading, run } = useAsyncAction();

const requests = computed(() => (collectionId.value ? (ws.trees[collectionId.value]?.requests ?? []) : []));
const all = (label: string) => [{ value: '', label }];
const collectionOptions = computed(() => [
  ...all('All'),
  ...ws.collections.map((c) => ({ value: c.id, label: c.name })),
]);
const requestOptions = computed(() => [...all('All'), ...requests.value.map((r) => ({ value: r.id, label: r.name }))]);
const STATUS_OPTIONS = [
  ...all('Any'),
  { value: 'passed', label: 'Passed' },
  { value: 'failed', label: 'Failed' },
  { value: 'error', label: 'Error' },
  { value: 'cancelled', label: 'Cancelled' },
];

async function load() {
  const list = await run(() =>
    api.listRuns({
      collectionId: collectionId.value,
      requestId: requestId.value,
      status: status.value,
      from: from.value ? new Date(`${from.value}T00:00:00`).toISOString() : null,
      to: to.value ? new Date(`${to.value}T23:59:59.999`).toISOString() : null,
      limit: 500,
    }),
  );
  if (list) runs.value = list;
}

watch(collectionId, async (id) => {
  requestId.value = '';
  if (id && !ws.trees[id]) await ws.loadTree(id).catch(ui.error);
});
watch([collectionId, requestId, status, from, to], load);
onMounted(async () => {
  await load();
  // Another screen asked to show a specific run (ui.showRun).
  const id = ui.historyRunId;
  if (id) {
    ui.historyRunId = '';
    await openById(id);
  }
});

async function openById(id: string) {
  await run(async () => {
    selected.value = await api.getRun(id);
  });
}
const open = (r: Run) => openById(r.id);

async function exportReport(format: 'json' | 'html') {
  const id = selected.value?.id;
  if (!id) return;
  await run(async () => {
    const path = await api.exportReport(id, format);
    if (path) ui.toast(`Report saved to ${path}`, 'success');
  });
}

async function remove(r: Run) {
  const ok = await ui.confirm({ title: 'Delete this run from history?', confirmLabel: 'Delete', danger: true });
  if (!ok) return;
  await run(async () => {
    await api.deleteRun(r.id);
    if (selected.value?.id === r.id) selected.value = null;
    await load();
  });
}

async function clearAll() {
  const ok = await ui.confirm({
    title: 'Clear all run history?',
    message: 'Every stored run and result will be permanently deleted. Collections and baselines are kept.',
    confirmLabel: 'Clear history',
    danger: true,
  });
  if (!ok) return;
  await run(async () => {
    const n = await api.clearHistory();
    selected.value = null;
    ui.toast(`Deleted ${n} runs`, 'success');
    await load();
  });
}
</script>

<template>
  <div class="history">
    <div class="list">
      <div class="filters">
        <SelectField v-model="collectionId" class="wide" label="Collection" :options="collectionOptions" />
        <SelectField
          v-model="requestId"
          class="wide"
          label="Request"
          :options="requestOptions"
          :disabled="!collectionId"
        />
        <SelectField v-model="status" label="Status" :options="STATUS_OPTIONS" />
        <TextField v-model="from" label="From" type="date" />
        <TextField v-model="to" label="To" type="date" />
      </div>
      <div class="row bar">
        <span class="muted small">{{ runs.length }} run(s){{ loading ? ' · loading…' : '' }}</span>
        <span class="spacer" />
        <BaseButton variant="danger" small :disabled="runs.length === 0" @click="clearAll">Clear history</BaseButton>
      </div>
      <SelectableList
        class="rows"
        :items="runs"
        :item-key="(r) => r.id"
        :active-key="selected?.id"
        stacked
        label="Runs"
        @select="open"
      >
        <template #default="{ item: r }">
          <span class="row"
            ><StatusBadge :status="r.status" /><strong class="grow name">{{
              r.collectionName || 'Single request'
            }}</strong></span
          >
          <span class="row small muted">
            <span>{{ formatTime(r.startedAt) }}</span
            ><span class="spacer" /> <span class="pass">{{ r.passedCount }}</span
            >/<span class="fail">{{ r.failedCount }}</span
            >/<span class="warn">{{ r.errorCount }}</span> · {{ formatDuration(r.durationMs) }}
          </span>
          <span class="small muted">{{ r.environmentName || 'No environment' }}</span>
        </template>
        <template v-if="!loading" #empty>
          <EmptyState
            small
            message="No runs match these filters. Runs are recorded every time you send a request or run a collection."
          />
        </template>
      </SelectableList>
    </div>
    <div class="detail panel">
      <EmptyState
        v-if="!selected"
        title="Run history"
        message="Select a run to see its results, compare with earlier runs and export a report."
      />
      <template v-else>
        <div class="row head">
          <h2 class="grow">{{ selected.collectionName || selected.results?.[0]?.request.name || 'Run' }}</h2>
          <BaseButton @click="exportReport('html')">Export HTML</BaseButton>
          <BaseButton @click="exportReport('json')">Export JSON</BaseButton>
          <BaseButton variant="danger" @click="remove(selected)">Delete</BaseButton>
        </div>
        <RunSummary :run="selected" :results="selected.results ?? []" />
      </template>
    </div>
  </div>
</template>

<style scoped>
.history {
  display: flex;
  flex: 1;
  min-height: 0;
}
.list {
  width: 340px;
  flex-shrink: 0;
  border-right: 1px solid var(--line);
  display: flex;
  flex-direction: column;
  background: var(--bg-subtle);
}
.filters {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
  padding: 10px;
  border-bottom: 1px solid var(--line);
}
.filters .wide {
  grid-column: span 2;
}
.bar {
  padding: 6px 10px;
  border-bottom: 1px solid var(--line);
}
.rows {
  flex: 1;
  overflow: auto;
  padding: 6px;
}
.name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.detail {
  flex: 1;
  overflow: auto;
  min-width: 0;
}
.head h2 {
  margin: 0;
  font-size: 17px;
}
.head {
  margin-bottom: 10px;
  flex-wrap: wrap;
}
</style>

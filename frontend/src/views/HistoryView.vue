<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';

import RunSummary from '../components/RunSummary.vue';
import { api } from '../services/api';
import { formatDuration, formatTime } from '../services/model';
import { useUi } from '../stores/ui';
import { useWorkspace } from '../stores/workspace';
import type { Run } from '../types';

const ui = useUi();
const ws = useWorkspace();

const collectionId = ref('');
const requestId = ref('');
const status = ref('');
const from = ref('');
const to = ref('');
const runs = ref<Run[]>([]);
const selected = ref<Run | null>(null);
const loading = ref(false);

const requests = computed(() => (collectionId.value ? (ws.trees[collectionId.value]?.requests ?? []) : []));

async function load() {
  loading.value = true;
  try {
    runs.value = await api.listRuns({
      collectionId: collectionId.value,
      requestId: requestId.value,
      status: status.value,
      from: from.value ? new Date(`${from.value}T00:00:00`).toISOString() : null,
      to: to.value ? new Date(`${to.value}T23:59:59.999`).toISOString() : null,
      limit: 500,
    });
  } catch (e) {
    ui.error(e);
  } finally {
    loading.value = false;
  }
}

watch(collectionId, async (id) => {
  requestId.value = '';
  if (id && !ws.trees[id]) await ws.loadTree(id).catch(ui.error);
});
watch([collectionId, requestId, status, from, to], load);
onMounted(load);

async function open(r: Run) {
  try {
    selected.value = await api.getRun(r.id);
  } catch (e) {
    ui.error(e);
  }
}

async function exportReport(format: 'json' | 'html') {
  if (!selected.value) return;
  try {
    const path = await api.exportReport(selected.value.id, format);
    if (path) ui.toast(`Report saved to ${path}`, 'success');
  } catch (e) {
    ui.error(e);
  }
}

async function remove(r: Run) {
  const ok = await ui.confirm({ title: 'Delete this run from history?', confirmLabel: 'Delete', danger: true });
  if (!ok) return;
  try {
    await api.deleteRun(r.id);
    if (selected.value?.id === r.id) selected.value = null;
    await load();
  } catch (e) {
    ui.error(e);
  }
}

async function clearAll() {
  const ok = await ui.confirm({
    title: 'Clear all run history?',
    message: 'Every stored run and result will be permanently deleted. Collections and baselines are kept.',
    confirmLabel: 'Clear history',
    danger: true,
  });
  if (!ok) return;
  try {
    const n = await api.clearHistory();
    selected.value = null;
    ui.toast(`Deleted ${n} runs`, 'success');
    await load();
  } catch (e) {
    ui.error(e);
  }
}
</script>

<template>
  <div class="history">
    <div class="list">
      <div class="filters">
        <label class="field"
          ><span>Collection</span>
          <select v-model="collectionId">
            <option value="">All</option>
            <option v-for="c in ws.collections" :key="c.id" :value="c.id">{{ c.name }}</option>
          </select>
        </label>
        <label class="field"
          ><span>Request</span>
          <select v-model="requestId" :disabled="!collectionId">
            <option value="">All</option>
            <option v-for="r in requests" :key="r.id" :value="r.id">{{ r.name }}</option>
          </select>
        </label>
        <label class="field"
          ><span>Status</span>
          <select v-model="status">
            <option value="">Any</option>
            <option value="passed">Passed</option>
            <option value="failed">Failed</option>
            <option value="error">Error</option>
            <option value="cancelled">Cancelled</option>
          </select>
        </label>
        <label class="field"><span>From</span><input v-model="from" type="date" /></label>
        <label class="field"><span>To</span><input v-model="to" type="date" /></label>
      </div>
      <div class="row bar">
        <span class="muted small">{{ runs.length }} run(s){{ loading ? ' · loading…' : '' }}</span>
        <span class="spacer" />
        <button class="danger small" :disabled="runs.length === 0" @click="clearAll">Clear history</button>
      </div>
      <div class="rows">
        <div v-if="!loading && runs.length === 0" class="empty small">
          <p>No runs match these filters. Runs are recorded every time you send a request or run a collection.</p>
        </div>
        <button
          v-for="r in runs"
          :key="r.id"
          class="ghost run"
          :class="{ active: selected?.id === r.id }"
          @click="open(r)"
        >
          <span class="row"
            ><span class="badge" :class="r.status">{{ r.status }}</span
            ><strong class="grow name">{{ r.collectionName || 'Single request' }}</strong></span
          >
          <span class="row small muted">
            <span>{{ formatTime(r.startedAt) }}</span
            ><span class="spacer" /> <span class="pass">{{ r.passedCount }}</span
            >/<span class="fail">{{ r.failedCount }}</span
            >/<span class="warn">{{ r.errorCount }}</span> · {{ formatDuration(r.durationMs) }}
          </span>
          <span class="small muted">{{ r.environmentName || 'No environment' }}</span>
        </button>
      </div>
    </div>
    <div class="detail panel">
      <div v-if="!selected" class="empty">
        <h2>Run history</h2>
        <p>Select a run to see its results, compare with earlier runs and export a report.</p>
      </div>
      <template v-else>
        <div class="row head">
          <h2 class="grow">{{ selected.collectionName || selected.results?.[0]?.request.name || 'Run' }}</h2>
          <button @click="exportReport('html')">Export HTML</button>
          <button @click="exportReport('json')">Export JSON</button>
          <button class="danger" @click="remove(selected)">Delete</button>
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
.filters .field:first-child,
.filters .field:nth-child(2) {
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
.run {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 2px;
  align-items: stretch;
  text-align: left;
  padding: 8px;
  border-bottom: 1px solid var(--line);
  border-radius: 0;
}
.run.active {
  background: var(--bg-muted);
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

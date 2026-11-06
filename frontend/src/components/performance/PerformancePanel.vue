<script setup lang="ts">
import { computed, ref, watch } from 'vue';

import { api } from '../services/api';
import { formatDuration, formatTime } from '../services/model';
import { useEnvironments } from '../stores/environments';
import { useUi } from '../stores/ui';
import { useWorkspace } from '../stores/workspace';
import type { PerformanceCheck, ResponseTimeTrend } from '../types';
import ResponseTimeChart from './ResponseTimeChart.vue';

const ws = useWorkspace();
const envs = useEnvironments();
const ui = useUi();

const req = computed(() => ws.draft!);
const saved = computed(
  () => !!req.value.id && !!ws.trees[req.value.collectionId]?.requests.some((r) => r.id === req.value.id),
);
const trend = ref<ResponseTimeTrend | null>(null);
const form = ref<PerformanceCheck | null>(null);
const savedForm = ref('');
const loading = ref(false);
const saving = ref(false);
const showTable = ref(false);

const dirty = computed(() => !!form.value && JSON.stringify(form.value) !== savedForm.value);
const envName = computed(() => envs.selected?.name ?? 'No environment');
const slowCount = computed(() => trend.value?.points.filter((p) => p.slower).length ?? 0);
const latestFirst = computed(() => [...(trend.value?.points ?? [])].reverse().slice(0, 50));

const MODES = [
  { value: 'warn', label: 'Warn', hint: 'Show "slower than usual" but keep the request passing' },
  { value: 'fail', label: 'Fail', hint: 'Fail the request, so the run fails too (and CI, with the CLI)' },
  { value: 'off', label: 'Off', hint: 'Do not check this request' },
] as const;

async function load() {
  const id = req.value.id;
  if (!saved.value) {
    trend.value = null;
    form.value = null;
    return;
  }
  loading.value = true;
  try {
    const t = await api.getResponseTimeTrend(id, envs.selectedId);
    if (req.value.id !== id) return;
    trend.value = t;
    // Keep unsaved edits to the settings when only the chart refreshes.
    if (!dirty.value || form.value?.requestId !== id) {
      form.value = { ...t.check };
      savedForm.value = JSON.stringify(form.value);
    }
  } catch (e) {
    ui.error(e);
  } finally {
    loading.value = false;
  }
}

// Reload for another request or environment, and after each send.
watch(
  () => [req.value.id, envs.selectedId, saved.value, ws.currentResult?.id] as const,
  ([id], prev) => {
    if (prev && prev[0] !== id) form.value = null;
    void load();
  },
  { immediate: true },
);

async function save() {
  if (!form.value) return;
  saving.value = true;
  try {
    const c = await api.savePerformanceCheck({ ...form.value, requestId: req.value.id });
    form.value = { ...c };
    savedForm.value = JSON.stringify(form.value);
    ui.toast('Performance check saved', 'success');
    await load();
  } catch (e) {
    ui.error(e);
  } finally {
    saving.value = false;
  }
}

function reset() {
  if (!form.value) return;
  form.value = { ...form.value, mode: 'warn', factor: 1.5, minDeltaMs: 100, window: 20, minSamples: 5 };
}
</script>

<template>
  <div class="perf stack">
    <p v-if="!saved" class="muted small">Save this request to track its response times.</p>
    <template v-else>
      <p class="muted small intro">
        Every send and collection run records how long this request took in <strong>{{ envName }}</strong
        >. A response is <strong>slower than usual</strong> when it takes at least
        <strong>{{ form?.factor ?? 1.5 }}×</strong> the median of the last {{ form?.window ?? 20 }} responses
        <em>and</em> at least <strong>{{ form?.minDeltaMs ?? 100 }} ms</strong> longer. Race tests and permission
        matrices are not counted.
      </p>

      <div v-if="trend" class="tiles">
        <div class="tile">
          <span class="label">Median</span
          ><strong>{{ trend.stats.count ? formatDuration(trend.stats.medianMs) : '—' }}</strong>
        </div>
        <div class="tile">
          <span class="label">p95</span
          ><strong>{{ trend.stats.count ? formatDuration(trend.stats.p95Ms) : '—' }}</strong>
        </div>
        <div class="tile">
          <span class="label">Fastest / slowest</span>
          <strong>{{
            trend.stats.count ? `${formatDuration(trend.stats.minMs)} / ${formatDuration(trend.stats.maxMs)}` : '—'
          }}</strong>
        </div>
        <div class="tile">
          <span class="label">Next response is slower from</span>
          <strong v-if="trend.check.mode === 'off'" class="muted">check off</strong>
          <strong v-else-if="trend.next.thresholdMs">{{ formatDuration(trend.next.thresholdMs) }}</strong>
          <strong v-else class="muted">{{ trend.next.samples }}/{{ trend.next.needed }} responses</strong>
        </div>
      </div>

      <div v-if="trend && trend.points.length" class="chart-card">
        <div class="row chart-head">
          <strong class="grow">Last {{ trend.points.length }} responses</strong>
          <span v-if="slowCount" class="small"
            ><span class="key slow" aria-hidden="true" />{{ slowCount }} slower than usual</span
          >
          <button class="small ghost" :aria-pressed="showTable" @click="showTable = !showTable">
            {{ showTable ? 'Show chart' : 'Show table' }}
          </button>
        </div>
        <ResponseTimeChart
          v-if="!showTable"
          :points="trend.points"
          :reference-ms="trend.next.referenceMs"
          :threshold-ms="trend.check.mode === 'off' ? 0 : trend.next.thresholdMs"
        />
        <div v-else class="table-wrap">
          <table class="grid">
            <thead>
              <tr>
                <th>When</th>
                <th class="num">Time</th>
                <th>HTTP</th>
                <th>Result</th>
                <th />
              </tr>
            </thead>
            <tbody>
              <tr v-for="p in latestFirst" :key="p.resultId">
                <td>{{ formatTime(p.at) }}</td>
                <td class="num">{{ formatDuration(p.durationMs) }}</td>
                <td>{{ p.statusCode || '—' }}</td>
                <td>{{ p.status }}</td>
                <td>
                  <span v-if="p.slower" class="small"
                    ><span class="key slow" aria-hidden="true" />slower than usual</span
                  >
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
      <p v-else-if="trend && !loading" class="muted small">
        No responses in {{ envName }} yet. Send the request or run its collection to start the trend.
      </p>

      <fieldset v-if="form" class="settings">
        <legend>When a response is slower than usual</legend>
        <div class="row modes" role="radiogroup" aria-label="Performance check mode">
          <label v-for="m in MODES" :key="m.value" class="mode" :title="m.hint">
            <input v-model="form.mode" type="radio" name="perf-mode" :value="m.value" />
            {{ m.label }}
          </label>
        </div>
        <p class="muted small">{{ MODES.find((m) => m.value === form!.mode)?.hint }}</p>
        <div v-if="form.mode !== 'off'" class="row fields">
          <label class="field">
            <span>At least × median</span>
            <input v-model.number="form.factor" type="number" min="1.1" max="10" step="0.1" />
          </label>
          <label class="field">
            <span>And at least (ms) more</span>
            <input v-model.number="form.minDeltaMs" type="number" min="0" max="600000" step="10" />
          </label>
          <label class="field">
            <span>Median of last</span>
            <input v-model.number="form.window" type="number" min="5" max="200" />
          </label>
          <label class="field">
            <span>Start after</span>
            <input v-model.number="form.minSamples" type="number" min="3" :max="form.window" />
          </label>
        </div>
        <div class="row">
          <button class="primary small" :disabled="!dirty || saving" @click="save">Save check</button>
          <button class="small ghost" @click="reset">Defaults</button>
        </div>
      </fieldset>
    </template>
  </div>
</template>

<style scoped>
.perf {
  gap: 12px;
}
.intro {
  margin: 0;
  max-width: 760px;
}
.tiles {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 8px;
}
.tile {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 8px 10px;
  border: 1px solid var(--line);
  border-radius: var(--radius);
}
.tile .label {
  color: var(--fg-muted);
  font-size: 12px;
}
.tile strong {
  font-size: 16px;
  font-variant-numeric: tabular-nums;
}
.chart-card {
  border: 1px solid var(--line);
  border-radius: var(--radius);
  padding: 8px 10px;
}
.chart-head {
  gap: 10px;
  margin-bottom: 4px;
}
.key {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-right: 5px;
  vertical-align: middle;
}
.key.slow {
  background: var(--warn);
}
.table-wrap {
  max-height: 260px;
  overflow: auto;
}
.num {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.settings {
  border: 1px solid var(--line);
  border-radius: var(--radius);
  padding: 8px 12px 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.settings legend {
  font-weight: 600;
  padding: 0 4px;
}
.settings p {
  margin: 0;
}
.modes {
  gap: 16px;
}
.mode {
  display: flex;
  align-items: center;
  gap: 4px;
}
.fields {
  flex-wrap: wrap;
  gap: 10px;
}
.fields .field {
  width: 150px;
}
</style>

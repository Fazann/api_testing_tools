<script setup lang="ts">
import { computed, ref, watch } from 'vue';

import BaseButton from '@/components/base/BaseButton.vue';
import DataTable from '@/components/base/DataTable.vue';
import RadioGroup from '@/components/base/RadioGroup.vue';
import StatTile from '@/components/base/StatTile.vue';
import TextField from '@/components/base/TextField.vue';
import { useAsyncAction } from '@/composables/useAsyncAction';
import { api } from '@/services/api';
import { useEnvironments } from '@/stores/environments';
import { useUi } from '@/stores/ui';
import { useWorkspace } from '@/stores/workspace';
import type { PerformanceCheck, ResponseTimeTrend } from '@/types';
import { formatDuration, formatTime } from '@/utils/format';

import ResponseTimeChart from './ResponseTimeChart.vue';
import SlowDot from './SlowDot.vue';

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
const { busy: loading, run: runLoad } = useAsyncAction();
const { busy: saving, run: runSave } = useAsyncAction();
const showTable = ref(false);

const dirty = computed(() => !!form.value && JSON.stringify(form.value) !== savedForm.value);
const envName = computed(() => envs.selected?.name ?? 'No environment');
const slowCount = computed(() => trend.value?.points.filter((p) => p.slower).length ?? 0);
const latestFirst = computed(() => [...(trend.value?.points ?? [])].reverse().slice(0, 50));

const TABLE_COLUMNS = [
  { key: 'at', label: 'When' },
  { key: 'durationMs', label: 'Time', class: 'num' },
  { key: 'statusCode', label: 'HTTP' },
  { key: 'status', label: 'Result' },
  { key: 'slower' },
];
const statText = (ms: number) => (trend.value?.stats.count ? formatDuration(ms) : '—');

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
  await runLoad(async () => {
    const t = await api.getResponseTimeTrend(id, envs.selectedId);
    if (req.value.id !== id) return;
    trend.value = t;
    // Keep unsaved edits to the settings when only the chart refreshes.
    if (!dirty.value || form.value?.requestId !== id) {
      form.value = { ...t.check };
      savedForm.value = JSON.stringify(form.value);
    }
  });
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
  const current = form.value;
  if (!current) return;
  const ok = await runSave(async () => {
    const c = await api.savePerformanceCheck({ ...current, requestId: req.value.id });
    form.value = { ...c };
    savedForm.value = JSON.stringify(form.value);
    ui.toast('Performance check saved', 'success');
    return true;
  });
  if (ok) await load();
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
        <StatTile label="Median" :value="statText(trend.stats.medianMs)" />
        <StatTile label="p95" :value="statText(trend.stats.p95Ms)" />
        <StatTile
          label="Fastest / slowest"
          :value="trend.stats.count ? `${statText(trend.stats.minMs)} / ${statText(trend.stats.maxMs)}` : '—'"
        />
        <StatTile label="Next response is slower from">
          <span v-if="trend.check.mode === 'off'" class="muted">check off</span>
          <template v-else-if="trend.next.thresholdMs">{{ formatDuration(trend.next.thresholdMs) }}</template>
          <span v-else class="muted">{{ trend.next.samples }}/{{ trend.next.needed }} responses</span>
        </StatTile>
      </div>

      <div v-if="trend && trend.points.length" class="chart-card">
        <div class="row chart-head">
          <strong class="grow">Last {{ trend.points.length }} responses</strong>
          <SlowDot v-if="slowCount" class="small">{{ slowCount }} slower than usual</SlowDot>
          <BaseButton variant="ghost" small :aria-pressed="showTable" @click="showTable = !showTable">
            {{ showTable ? 'Show chart' : 'Show table' }}
          </BaseButton>
        </div>
        <ResponseTimeChart
          v-if="!showTable"
          :points="trend.points"
          :reference-ms="trend.next.referenceMs"
          :threshold-ms="trend.check.mode === 'off' ? 0 : trend.next.thresholdMs"
        />
        <div v-else class="table-wrap">
          <DataTable :columns="TABLE_COLUMNS" :rows="latestFirst" :row-key="(p) => p.resultId">
            <template #cell-at="{ row }">{{ formatTime(row.at) }}</template>
            <template #cell-durationMs="{ row }">{{ formatDuration(row.durationMs) }}</template>
            <template #cell-statusCode="{ row }">{{ row.statusCode || '—' }}</template>
            <template #cell-slower="{ row }">
              <SlowDot v-if="row.slower" class="small">slower than usual</SlowDot>
            </template>
          </DataTable>
        </div>
      </div>
      <p v-else-if="trend && !loading" class="muted small">
        No responses in {{ envName }} yet. Send the request or run its collection to start the trend.
      </p>

      <fieldset v-if="form" class="settings">
        <legend>When a response is slower than usual</legend>
        <RadioGroup v-model="form.mode" :options="MODES" name="perf-mode" label="Performance check mode" />
        <p class="muted small">{{ MODES.find((m) => m.value === form!.mode)?.hint }}</p>
        <div v-if="form.mode !== 'off'" class="row fields">
          <TextField v-model="form.factor" label="At least × median" type="number" min="1.1" max="10" step="0.1" />
          <TextField
            v-model="form.minDeltaMs"
            label="And at least (ms) more"
            type="number"
            min="0"
            max="600000"
            step="10"
          />
          <TextField v-model="form.window" label="Median of last" type="number" min="5" max="200" />
          <TextField v-model="form.minSamples" label="Start after" type="number" min="3" :max="form.window" />
        </div>
        <div class="row">
          <BaseButton variant="primary" small :disabled="!dirty || saving" @click="save">Save check</BaseButton>
          <BaseButton variant="ghost" small @click="reset">Defaults</BaseButton>
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
.chart-card {
  border: 1px solid var(--line);
  border-radius: var(--radius);
  padding: 8px 10px;
}
.chart-head {
  gap: 10px;
  margin-bottom: 4px;
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
.fields {
  flex-wrap: wrap;
  gap: 10px;
}
.fields .field {
  width: 150px;
}
</style>

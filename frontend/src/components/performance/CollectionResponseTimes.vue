<script setup lang="ts">
import { computed, ref, watch } from 'vue';

import { api } from '@/api';
import DataTable from '@/components/base/DataTable.vue';
import MethodBadge from '@/components/base/MethodBadge.vue';
import SelectField from '@/components/base/SelectField.vue';
import { useAsyncAction } from '@/composables/useAsyncAction';
import { sparkPath } from '@/services/responseTimes';
import { useEnvironments } from '@/stores/environments';
import type { CollectionTree, Request, ResponseTimeRow } from '@/types';
import { formatChange, formatDuration } from '@/utils/format';

import SlowDot from './SlowDot.vue';

const props = defineProps<{ tree: CollectionTree }>();
const emit = defineEmits<{ open: [request: Request] }>();
const envs = useEnvironments();
const { run } = useAsyncAction();

const rows = ref<ResponseTimeRow[] | null>(null);
type Sort = 'order' | 'median' | 'p95' | 'change';
const sort = ref<Sort>('order');
const SORTS: { value: Sort; label: string }[] = [
  { value: 'order', label: 'Collection order' },
  { value: 'median', label: 'Slowest median' },
  { value: 'p95', label: 'Slowest p95' },
  { value: 'change', label: 'Biggest slowdown' },
];
const COLUMNS = [
  { key: 'method', label: 'Method' },
  { key: 'name', label: 'Name', class: 'name' },
  { key: 'median', label: 'Median', class: 'num' },
  { key: 'p95', label: 'p95', class: 'num' },
  { key: 'latest', label: 'Latest', class: 'num' },
  { key: 'change', label: 'Change', class: 'num' },
  { key: 'trend', label: 'Trend' },
  { key: 'count', label: 'Responses', class: 'num muted' },
];
/** A change of at least this much is highlighted. */
const NOTABLE_PCT = 20;

watch(
  () => [props.tree.collection.id, envs.selectedId] as const,
  async ([id, env]) => {
    rows.value = null;
    const r = await run(() => api.getCollectionResponseTimes(id, env));
    if (r && props.tree.collection.id === id && envs.selectedId === env) rows.value = r;
  },
  { immediate: true },
);

const withData = computed(() => (rows.value ?? []).filter((r) => r.stats.count > 0));
const slowNow = computed(() => withData.value.filter((r) => r.lastSlower).length);
const sorted = computed(() => {
  const list = [...(rows.value ?? [])];
  const key: Record<Sort, (r: ResponseTimeRow) => number> = {
    order: () => 0,
    median: (r) => r.stats.medianMs,
    p95: (r) => r.stats.p95Ms,
    change: (r) => r.changePct ?? -Infinity,
  };
  if (sort.value !== 'order') list.sort((a, b) => key[sort.value](b) - key[sort.value](a));
  return list;
});

function open(row: ResponseTimeRow) {
  const r = props.tree.requests.find((x) => x.id === row.requestId);
  if (r) emit('open', r);
}

function changeClass(pct: number | null) {
  if (pct === null) return 'muted';
  return pct >= NOTABLE_PCT ? 'worse' : pct <= -NOTABLE_PCT ? 'better' : '';
}
</script>

<template>
  <div class="stack">
    <p class="muted small intro">
      Response times of each request in <strong>{{ envs.selected?.name ?? 'No environment' }}</strong> over its last 30
      responses. <strong>Change</strong> compares the median of the latest 5 with the ones before. Open a request's
      <strong>Performance</strong> tab for its chart and settings.
    </p>
    <p v-if="!rows" class="muted small">Loading…</p>
    <template v-else>
      <div class="row summary">
        <span>{{ withData.length }} of {{ rows.length }} requests have response times</span>
        <SlowDot v-if="slowNow" class="small">{{ slowNow }} slower than usual in their latest response</SlowDot>
        <span class="spacer" />
        <label class="row small sort">Sort <SelectField v-model="sort" :options="SORTS" /></label>
      </div>
      <DataTable
        :columns="COLUMNS"
        :rows="sorted"
        :row-key="(r) => r.requestId"
        :row-title="(r) => `Open ${r.name}`"
        clickable
        empty="This collection has no requests."
        @row-click="open"
      >
        <template #cell-method="{ row }"><MethodBadge :method="row.method" /></template>
        <template #cell-name="{ row }">
          {{ row.name }}
          <span v-if="row.mode === 'fail'" class="tag" title="A slowdown fails this request">fails if slow</span>
          <span v-else-if="row.mode === 'off'" class="tag muted" title="Response-time check is off">check off</span>
        </template>
        <template #cell-median="{ row }">
          <template v-if="row.stats.count">{{ formatDuration(row.stats.medianMs) }}</template>
          <span v-else class="muted small">No responses yet</span>
        </template>
        <template #cell-p95="{ row }">{{ row.stats.count ? formatDuration(row.stats.p95Ms) : '' }}</template>
        <template #cell-latest="{ row }">
          <template v-if="row.stats.count">
            <SlowDot v-if="row.lastSlower" label="slower than usual," />{{ formatDuration(row.lastMs) }}
          </template>
        </template>
        <template #cell-change="{ row }">
          <span v-if="row.stats.count" :class="changeClass(row.changePct)">{{ formatChange(row.changePct) }}</span>
        </template>
        <template #cell-trend="{ row }">
          <svg v-if="row.recent.length > 1" class="spark" width="80" height="18" viewBox="0 0 80 18" aria-hidden="true">
            <path :d="sparkPath(row.recent, 80, 18)" />
          </svg>
        </template>
        <template #cell-count="{ row }">{{ row.stats.count || '' }}</template>
      </DataTable>
    </template>
  </div>
</template>

<style scoped>
.intro {
  margin: 0;
}
.summary {
  gap: 12px;
  align-items: center;
}
.sort {
  gap: 6px;
  align-items: center;
}
:deep(.name) {
  font-weight: 600;
}
.tag {
  font-weight: 400;
  font-size: 11px;
  margin-left: 6px;
  padding: 0 5px;
  border: 1px solid var(--line);
  border-radius: 999px;
}
:deep(.num) {
  white-space: nowrap;
}
.worse {
  color: var(--fail);
  font-weight: 600;
}
.better {
  color: var(--pass);
}
.spark path {
  fill: none;
  stroke: var(--accent);
  stroke-width: 1.5;
  stroke-linejoin: round;
}
</style>

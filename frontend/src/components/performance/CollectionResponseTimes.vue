<script setup lang="ts">
import { computed, ref, watch } from 'vue';

import { api } from '../services/api';
import { formatDuration } from '../services/model';
import { formatChange, sparkPath } from '../services/responseTimes';
import { useEnvironments } from '../stores/environments';
import { useUi } from '../stores/ui';
import type { CollectionTree, Request, ResponseTimeRow } from '../types';

const props = defineProps<{ tree: CollectionTree }>();
const emit = defineEmits<{ open: [request: Request] }>();
const envs = useEnvironments();
const ui = useUi();

const rows = ref<ResponseTimeRow[] | null>(null);
type Sort = 'order' | 'median' | 'p95' | 'change';
const sort = ref<Sort>('order');
/** A change of at least this much is highlighted. */
const NOTABLE_PCT = 20;

watch(
  () => [props.tree.collection.id, envs.selectedId] as const,
  async ([id, env]) => {
    rows.value = null;
    try {
      const r = await api.getCollectionResponseTimes(id, env);
      if (props.tree.collection.id === id && envs.selectedId === env) rows.value = r;
    } catch (e) {
      ui.error(e);
    }
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
        <span v-if="slowNow" class="small"
          ><span class="key slow" aria-hidden="true" />{{ slowNow }} slower than usual in their latest response</span
        >
        <span class="spacer" />
        <label class="row small sort"
          >Sort
          <select v-model="sort">
            <option value="order">Collection order</option>
            <option value="median">Slowest median</option>
            <option value="p95">Slowest p95</option>
            <option value="change">Biggest slowdown</option>
          </select>
        </label>
      </div>
      <table class="grid list">
        <thead>
          <tr>
            <th>Method</th>
            <th>Name</th>
            <th class="num">Median</th>
            <th class="num">p95</th>
            <th class="num">Latest</th>
            <th class="num">Change</th>
            <th>Trend</th>
            <th class="num">Responses</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="r in sorted"
            :key="r.requestId"
            tabindex="0"
            :title="`Open ${r.name}`"
            @click="open(r)"
            @keydown.enter.prevent="open(r)"
          >
            <td>
              <span class="method" :class="r.method">{{ r.method }}</span>
            </td>
            <td class="name">
              {{ r.name }}
              <span v-if="r.mode === 'fail'" class="tag" title="A slowdown fails this request">fails if slow</span>
              <span v-else-if="r.mode === 'off'" class="tag muted" title="Response-time check is off">check off</span>
            </td>
            <template v-if="r.stats.count">
              <td class="num">{{ formatDuration(r.stats.medianMs) }}</td>
              <td class="num">{{ formatDuration(r.stats.p95Ms) }}</td>
              <td class="num">
                <span v-if="r.lastSlower" class="key slow" title="Slower than usual" />
                <span class="sr-only" v-if="r.lastSlower">slower than usual,</span>
                {{ formatDuration(r.lastMs) }}
              </td>
              <td class="num" :class="changeClass(r.changePct)">{{ formatChange(r.changePct) }}</td>
              <td>
                <svg
                  v-if="r.recent.length > 1"
                  class="spark"
                  width="80"
                  height="18"
                  viewBox="0 0 80 18"
                  aria-hidden="true"
                >
                  <path :d="sparkPath(r.recent, 80, 18)" />
                </svg>
              </td>
              <td class="num muted">{{ r.stats.count }}</td>
            </template>
            <td v-else colspan="6" class="muted small">No responses yet</td>
          </tr>
          <tr v-if="rows.length === 0" class="none">
            <td colspan="8" class="muted small">This collection has no requests.</td>
          </tr>
        </tbody>
      </table>
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
.list tbody tr:not(.none) {
  cursor: pointer;
}
.list tbody tr:not(.none):hover,
.list tbody tr:focus-visible {
  background: var(--bg-muted);
}
.name {
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
.num {
  text-align: right;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
.worse {
  color: var(--fail);
  font-weight: 600;
}
.better {
  color: var(--pass);
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
.spark path {
  fill: none;
  stroke: var(--accent);
  stroke-width: 1.5;
  stroke-linejoin: round;
}
.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
}
</style>

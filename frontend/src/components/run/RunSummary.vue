<script setup lang="ts">
import { computed, ref } from 'vue';

import BaseButton from '@/components/base/BaseButton.vue';
import CheckboxField from '@/components/base/CheckboxField.vue';
import MethodBadge from '@/components/base/MethodBadge.vue';
import StatTile from '@/components/base/StatTile.vue';
import StatusBadge from '@/components/base/StatusBadge.vue';
import ResponseViewer from '@/components/response/ResponseViewer.vue';
import type { Run, RunResult } from '@/types';
import { formatDuration, formatTime } from '@/utils/format';

const props = defineProps<{
  run?: Run | null;
  results: RunResult[];
  running?: boolean;
  done?: number;
  total?: number;
}>();
const openId = ref('');
const onlyProblems = ref(false);

const counts = computed(() => {
  const c = { passed: 0, failed: 0, error: 0 };
  for (const r of props.results) {
    if (r.status === 'passed') c.passed++;
    else if (r.status === 'failed') c.failed++;
    else c.error++;
  }
  return c;
});
const slower = computed(() => props.results.filter((r) => r.performance?.slower).length);
const shown = computed(() => (onlyProblems.value ? props.results.filter((r) => r.status !== 'passed') : props.results));

function firstProblem(r: RunResult): string {
  if (r.error) return r.error;
  const a = r.assertionResults.find((x) => !x.passed);
  if (a) return a.message;
  if (r.regression && !r.regression.passed) return r.regression.message ?? 'Baseline differences';
  if (r.performance?.slower && r.performance.mode === 'fail') return r.performance.message;
  return '';
}
</script>

<template>
  <div class="stack summary">
    <div class="tiles">
      <StatTile label="Status">
        <StatusBadge
          :status="running ? 'running' : (run?.status ?? '')"
          :label="running ? 'running' : (run?.status ?? '—')"
        />
      </StatTile>
      <StatTile label="Passed" :value="counts.passed" tone="pass" />
      <StatTile label="Failed" :value="counts.failed" tone="fail" />
      <StatTile label="Errors" :value="counts.error" tone="warn" />
      <StatTile
        v-if="slower"
        label="Slower than usual"
        :value="`▲ ${slower}`"
        tone="warn"
        title="Responses that took much longer than their recent median"
      />
      <StatTile label="Duration" :value="run ? formatDuration(run.durationMs) : '…'" />
      <StatTile v-if="run" label="Environment" class="env" :value="run.environmentName || 'None'" />
    </div>
    <div v-if="running && total" class="progress" role="progressbar" :aria-valuenow="done" :aria-valuemax="total">
      <div class="bar" :style="{ width: `${((done ?? 0) / total) * 100}%` }" />
    </div>
    <div class="row">
      <span v-if="run" class="muted small">Started {{ formatTime(run.startedAt) }}</span>
      <span class="spacer" />
      <CheckboxField v-model="onlyProblems" class="small" label="Only failures and errors" />
    </div>
    <div v-for="r in shown" :key="r.id" class="result" :class="{ open: openId === r.id }">
      <BaseButton
        variant="ghost"
        class="head"
        :aria-expanded="openId === r.id"
        @click="openId = openId === r.id ? '' : r.id"
      >
        <StatusBadge :status="r.status" />
        <MethodBadge :method="r.request.method" />
        <strong class="name">{{ r.request.name }}</strong>
        <span v-if="r.response" class="muted">{{ r.response.statusCode }}</span>
        <span class="muted">{{ formatDuration(r.durationMs) }}</span>
        <span v-if="r.performance?.slower" class="slow small" :title="r.performance.message"
          >▲ {{ r.performance.ratio }}× slower</span
        >
        <span class="problem fail small">{{ firstProblem(r) }}</span>
      </BaseButton>
      <div v-if="openId === r.id" class="detail">
        <ResponseViewer :result="r" :request-id="r.requestId" />
      </div>
    </div>
    <p v-if="!running && results.length === 0" class="muted">No results.</p>
  </div>
</template>

<style scoped>
.slow {
  color: var(--warn);
  font-weight: 600;
  white-space: nowrap;
}
.tile.env :deep(b) {
  font-size: 13px;
}
.progress {
  height: 4px;
  background: var(--bg-muted);
  border-radius: 2px;
  overflow: hidden;
}
.bar {
  height: 100%;
  background: var(--accent);
  transition: width 0.2s;
}
.result {
  border: 1px solid var(--line);
  border-radius: var(--radius);
}
.head {
  width: 100%;
  display: flex;
  gap: 10px;
  align-items: center;
  text-align: left;
  padding: 8px 10px;
  overflow: hidden;
}
.name {
  white-space: nowrap;
}
.problem {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1;
  min-width: 0;
  text-align: right;
}
.detail {
  border-top: 1px solid var(--line);
  display: flex;
  flex-direction: column;
  max-height: 520px;
}
</style>

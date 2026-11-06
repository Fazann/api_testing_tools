<script setup lang="ts">
import { computed, ref } from 'vue';

import { formatDuration, formatTime } from '../services/model';
import type { Run, RunResult } from '../types';
import ResponseViewer from './ResponseViewer.vue';

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
      <div class="tile">
        <span class="muted small">Status</span
        ><b
          ><span class="badge" :class="running ? 'running' : run?.status">{{
            running ? 'running' : (run?.status ?? '—')
          }}</span></b
        >
      </div>
      <div class="tile">
        <span class="muted small">Passed</span><b class="pass">{{ counts.passed }}</b>
      </div>
      <div class="tile">
        <span class="muted small">Failed</span><b class="fail">{{ counts.failed }}</b>
      </div>
      <div class="tile">
        <span class="muted small">Errors</span><b class="warn">{{ counts.error }}</b>
      </div>
      <div v-if="slower" class="tile" title="Responses that took much longer than their recent median">
        <span class="muted small">Slower than usual</span><b class="warn">▲ {{ slower }}</b>
      </div>
      <div class="tile">
        <span class="muted small">Duration</span><b>{{ run ? formatDuration(run.durationMs) : '…' }}</b>
      </div>
      <div v-if="run" class="tile">
        <span class="muted small">Environment</span><b class="env">{{ run.environmentName || 'None' }}</b>
      </div>
    </div>
    <div v-if="running && total" class="progress" role="progressbar" :aria-valuenow="done" :aria-valuemax="total">
      <div class="bar" :style="{ width: `${((done ?? 0) / total) * 100}%` }" />
    </div>
    <div class="row">
      <span v-if="run" class="muted small">Started {{ formatTime(run.startedAt) }}</span>
      <span class="spacer" />
      <label class="row small"><input v-model="onlyProblems" type="checkbox" /> Only failures and errors</label>
    </div>
    <div v-for="r in shown" :key="r.id" class="result" :class="{ open: openId === r.id }">
      <button class="ghost head" :aria-expanded="openId === r.id" @click="openId = openId === r.id ? '' : r.id">
        <span class="badge" :class="r.status">{{ r.status }}</span>
        <span class="method" :class="r.request.method">{{ r.request.method }}</span>
        <strong class="name">{{ r.request.name }}</strong>
        <span v-if="r.response" class="muted">{{ r.response.statusCode }}</span>
        <span class="muted">{{ formatDuration(r.durationMs) }}</span>
        <span v-if="r.performance?.slower" class="slow small" :title="r.performance.message"
          >▲ {{ r.performance.ratio }}× slower</span
        >
        <span class="problem fail small">{{ firstProblem(r) }}</span>
      </button>
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
.tiles {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(110px, 1fr));
  gap: 8px;
}
.tile {
  border: 1px solid var(--line);
  border-radius: var(--radius);
  padding: 8px 10px;
  background: var(--bg-subtle);
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.tile b {
  font-size: 18px;
}
.tile .env {
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
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

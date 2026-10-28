<script setup lang="ts">
import { computed, ref, watch } from 'vue';

import { api } from '../services/api';
import { formatBytes, formatDuration, formatTime, prettyBody } from '../services/model';
import { useEnvironments } from '../stores/environments';
import { useUi } from '../stores/ui';
import type { Baseline, PreviousComparison, RunResult } from '../types';
import ApproveBaselineDialog from './ApproveBaselineDialog.vue';
import AssertionResults from './AssertionResults.vue';
import DiffTable from './DiffTable.vue';

const props = defineProps<{ result: RunResult; requestId?: string; allowApprove?: boolean }>();
const ui = useUi();
const envs = useEnvironments();

type Tab = 'body' | 'headers' | 'tests' | 'regression' | 'previous' | 'request';
const tab = ref<Tab>('tests');
const approving = ref(false);
const baseline = ref<Baseline | null>(null);
const previous = ref<PreviousComparison | null>(null);

const resp = computed(() => props.result.response);
const failedTests = computed(() => props.result.assertionResults.filter((a) => !a.passed).length);
const body = computed(() => (resp.value ? prettyBody(resp.value.body, resp.value.contentType) : ''));
const statusClass = computed(() => {
  const c = resp.value?.statusCode ?? 0;
  return c >= 200 && c < 300 ? 'pass' : c >= 400 ? 'fail' : 'warn';
});

watch(
  () => props.result.id,
  () => {
    previous.value = null;
    if (props.result.regression && !props.result.regression.passed) tab.value = 'regression';
    else if (failedTests.value > 0 || props.result.assertionResults.length > 0) tab.value = 'tests';
    else tab.value = 'body';
  },
  { immediate: true },
);

async function openApprove() {
  if (props.requestId) baseline.value = await api.getBaseline(props.requestId, envs.selectedId).catch(() => null);
  approving.value = true;
}

async function loadPrevious() {
  tab.value = 'previous';
  if (previous.value || !props.result.runId) return;
  try {
    previous.value = await api.compareWithPrevious(props.result.runId, props.result.id);
  } catch (e) {
    ui.error(e);
  }
}

async function copyBody() {
  try {
    await navigator.clipboard.writeText(resp.value?.body ?? '');
    ui.toast('Copied response body');
  } catch {
    ui.toast('Clipboard is not available', 'error');
  }
}
</script>

<template>
  <section class="response">
    <div class="row summary">
      <span class="badge" :class="result.status">{{ result.status }}</span>
      <template v-if="resp">
        <strong :class="statusClass">{{ resp.status || resp.statusCode }}</strong>
        <span class="muted">{{ formatDuration(result.durationMs) }}</span>
        <span class="muted"
          >{{ formatBytes(resp.bodyBytes) }}<span v-if="resp.truncated" class="warn"> (truncated at limit)</span></span
        >
      </template>
      <span v-if="result.assertionResults.length" :class="failedTests ? 'fail' : 'pass'">
        {{ result.assertionResults.length - failedTests }}/{{ result.assertionResults.length }} tests passed
      </span>
      <span v-if="result.regression" :class="result.regression.passed ? 'pass' : 'fail'">
        {{ result.regression.passed ? 'Matches baseline' : `${result.regression.diffs.length} baseline difference(s)` }}
      </span>
      <span class="spacer" />
      <span class="muted small">{{ formatTime(result.createdAt) }}</span>
      <button
        v-if="allowApprove && resp && requestId"
        class="small"
        title="Review and approve this response as the baseline"
        @click="openApprove"
      >
        Approve as baseline…
      </button>
    </div>
    <div v-if="result.error" class="callout error mono small err">{{ result.error }}</div>
    <nav class="tabs" role="tablist">
      <button :class="{ active: tab === 'tests' }" role="tab" @click="tab = 'tests'">
        Tests<span v-if="result.assertionResults.length" class="count" :class="{ fail: failedTests }">{{
          failedTests ? `${failedTests} failed` : result.assertionResults.length
        }}</span>
      </button>
      <button v-if="result.regression" :class="{ active: tab === 'regression' }" role="tab" @click="tab = 'regression'">
        Baseline diff<span class="count">{{ result.regression.diffs.length }}</span>
      </button>
      <button :class="{ active: tab === 'body' }" role="tab" :disabled="!resp" @click="tab = 'body'">Body</button>
      <button :class="{ active: tab === 'headers' }" role="tab" :disabled="!resp" @click="tab = 'headers'">
        Headers<span v-if="resp" class="count">{{ resp.headers?.length ?? 0 }}</span>
      </button>
      <button v-if="requestId" :class="{ active: tab === 'previous' }" role="tab" @click="loadPrevious">
        vs previous run
      </button>
      <button :class="{ active: tab === 'request' }" role="tab" @click="tab = 'request'">Sent request</button>
    </nav>
    <div class="tab-body">
      <AssertionResults v-if="tab === 'tests'" :results="result.assertionResults" />
      <div v-else-if="tab === 'regression' && result.regression" class="stack">
        <p :class="result.regression.passed ? 'pass' : 'fail'">{{ result.regression.message }}</p>
        <DiffTable v-if="result.regression.diffs.length" :diffs="result.regression.diffs" />
        <p v-if="!result.regression.passed && allowApprove" class="muted small">
          If this change is intended, review the response and choose <strong>Approve as baseline…</strong>. Otherwise,
          fix the regression.
        </p>
      </div>
      <div v-else-if="tab === 'body' && resp" class="stack">
        <div class="row">
          <span class="muted small">{{ resp.contentType }}</span
          ><span class="spacer" /><button class="ghost small" @click="copyBody">Copy</button>
        </div>
        <pre class="code body">{{ body || '(empty body)' }}</pre>
      </div>
      <table v-else-if="tab === 'headers' && resp" class="grid">
        <tbody>
          <tr v-for="(h, i) in resp.headers" :key="i">
            <td class="mono">{{ h.key }}</td>
            <td class="mono">{{ h.value }}</td>
          </tr>
        </tbody>
      </table>
      <div v-else-if="tab === 'previous'" class="stack">
        <p v-if="!previous" class="muted">Loading…</p>
        <template v-else-if="!previous.found"
          ><p class="muted">{{ previous.message }}</p></template
        >
        <template v-else>
          <p>
            Compared with the run from {{ formatTime(previous.previousStartedAt) }}: status
            <span class="badge" :class="previous.statusBefore">{{ previous.statusBefore }}</span> →
            <span class="badge" :class="previous.statusAfter">{{ previous.statusAfter }}</span
            >, HTTP {{ previous.codeBefore || '—' }} → {{ previous.codeAfter || '—' }}.
          </p>
          <div v-if="previous.newlyFailing.length" class="callout error">
            <strong>Newly failing tests</strong>
            <ul class="small">
              <li v-for="(m, i) in previous.newlyFailing" :key="i">{{ m }}</li>
            </ul>
          </div>
          <DiffTable v-if="previous.diffs.length" :diffs="previous.diffs" />
          <p v-else class="muted">Response bodies are identical (or not comparable JSON).</p>
        </template>
      </div>
      <div v-else-if="tab === 'request'" class="stack">
        <p class="mono small">
          <span class="method" :class="result.request.method">{{ result.request.method }}</span>
          {{ result.request.url }}
        </p>
        <table v-if="result.request.headers?.length" class="grid">
          <tbody>
            <tr v-for="(h, i) in result.request.headers" :key="i">
              <td class="mono">{{ h.key }}</td>
              <td class="mono">{{ h.value }}</td>
            </tr>
          </tbody>
        </table>
        <pre v-if="result.request.body" class="code">{{ result.request.body }}</pre>
        <p class="muted small">Secret values and credential headers are redacted.</p>
      </div>
    </div>
    <ApproveBaselineDialog
      v-if="approving && requestId"
      :result="result"
      :request-id="requestId"
      :existing-ignored="baseline?.ignoredPaths"
      @close="approving = false"
    />
  </section>
</template>

<style scoped>
.response {
  display: flex;
  flex-direction: column;
  min-height: 0;
  flex: 1;
}
.summary {
  padding: 8px 16px;
  flex-wrap: wrap;
}
.err {
  margin: 0 16px 8px;
  white-space: pre-wrap;
}
.tabs {
  padding: 0 12px;
}
.tab-body {
  padding: 12px 16px;
  overflow: auto;
  flex: 1;
  min-height: 0;
}
.body {
  max-height: none;
}
.count.fail {
  color: var(--fail);
}
</style>

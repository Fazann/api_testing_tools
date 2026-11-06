<script setup lang="ts">
import { computed, ref, watch } from 'vue';

import BaseButton from '@/components/base/BaseButton.vue';
import CalloutBox from '@/components/base/CalloutBox.vue';
import MethodBadge from '@/components/base/MethodBadge.vue';
import StatusBadge from '@/components/base/StatusBadge.vue';
import TabBar, { type Tab as TabItem } from '@/components/base/TabBar.vue';
import { useAsyncAction } from '@/composables/useAsyncAction';
import { api } from '@/services/api';
import { HIGHLIGHT_LIMIT, highlightJson } from '@/services/highlight';
import { useEnvironments } from '@/stores/environments';
import { useUi } from '@/stores/ui';
import type { Baseline, PreviousComparison, RunResult } from '@/types';
import { formatBytes, formatDuration, formatTime, prettyBody } from '@/utils/format';

import ApproveBaselineDialog from './ApproveBaselineDialog.vue';
import AssertionResults from './AssertionResults.vue';
import DiffTable from './DiffTable.vue';
import HeaderTable from './HeaderTable.vue';

const props = defineProps<{ result: RunResult; requestId?: string; allowApprove?: boolean }>();
const ui = useUi();
const envs = useEnvironments();
const { run } = useAsyncAction();

type Tab = 'body' | 'headers' | 'tests' | 'regression' | 'previous' | 'request' | 'console';
const tab = ref<Tab>('tests');
const approving = ref(false);
const baseline = ref<Baseline | null>(null);
const previous = ref<PreviousComparison | null>(null);

const resp = computed(() => props.result.response);
/** Only shown when the response was actually checked and found slower. */
const perf = computed(() => props.result.performance ?? null);
const failedTests = computed(() => props.result.assertionResults.filter((a) => !a.passed).length);
const logs = computed(() => props.result.logs ?? []);
// Results saved in history before the backend fix carry "diffs": null for a matching baseline.
const regressionDiffs = computed(() => props.result.regression?.diffs ?? []);
const body = computed(() => (resp.value ? prettyBody(resp.value.body, resp.value.contentType) : ''));
/** Colored tokens when the body is valid JSON (and not too large to highlight). */
const bodyTokens = computed(() => {
  const text = body.value;
  if (!text || text.length > HIGHLIGHT_LIMIT) return null;
  try {
    JSON.parse(text);
  } catch {
    return null;
  }
  return highlightJson(text);
});
const tabs = computed<TabItem<Tab>[]>(() => [
  {
    id: 'tests',
    label: 'Tests',
    count: props.result.assertionResults.length
      ? failedTests.value
        ? `${failedTests.value} failed`
        : props.result.assertionResults.length
      : undefined,
    alert: failedTests.value > 0,
  },
  { id: 'regression', label: 'Baseline diff', count: regressionDiffs.value.length, hidden: !props.result.regression },
  { id: 'body', label: 'Body', disabled: !resp.value },
  { id: 'headers', label: 'Headers', count: resp.value?.headers?.length ?? undefined, disabled: !resp.value },
  { id: 'previous', label: 'vs previous run', hidden: !props.requestId },
  { id: 'request', label: 'Sent request' },
  { id: 'console', label: 'Console', count: logs.value.length, hidden: !logs.value.length },
]);
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
  if (previous.value || !props.result.runId) return;
  const { runId, id } = props.result;
  previous.value = (await run(() => api.compareWithPrevious(runId, id))) ?? null;
}
watch(tab, (t) => t === 'previous' && loadPrevious());

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
      <StatusBadge :status="result.status" />
      <template v-if="resp">
        <strong :class="statusClass">{{ resp.status || resp.statusCode }}</strong>
        <span class="muted">{{ formatDuration(result.durationMs) }}</span>
        <span v-if="perf?.slower" class="perf-slow" :class="perf.mode" :title="perf.message"
          >▲ {{ perf.ratio }}× slower than usual{{ perf.mode === 'fail' ? ' (fails)' : '' }}</span
        >
        <span class="muted"
          >{{ formatBytes(resp.bodyBytes) }}<span v-if="resp.truncated" class="warn"> (truncated at limit)</span></span
        >
      </template>
      <span v-if="result.assertionResults.length" :class="failedTests ? 'fail' : 'pass'">
        {{ result.assertionResults.length - failedTests }}/{{ result.assertionResults.length }} tests passed
      </span>
      <span v-if="result.regression" :class="result.regression.passed ? 'pass' : 'fail'">
        {{ result.regression.passed ? 'Matches baseline' : `${regressionDiffs.length} baseline difference(s)` }}
      </span>
      <span class="spacer" />
      <span class="muted small">{{ formatTime(result.createdAt) }}</span>
      <BaseButton
        v-if="allowApprove && resp && requestId"
        small
        title="Review and approve this response as the baseline"
        @click="openApprove"
      >
        Approve as baseline…
      </BaseButton>
    </div>
    <CalloutBox v-if="result.error" tone="error" small class="mono err">{{ result.error }}</CalloutBox>
    <TabBar v-model="tab" :tabs="tabs" />
    <div class="tab-body">
      <AssertionResults v-if="tab === 'tests'" :results="result.assertionResults" />
      <div v-else-if="tab === 'regression' && result.regression" class="stack">
        <p :class="result.regression.passed ? 'pass' : 'fail'">{{ result.regression.message }}</p>
        <DiffTable v-if="regressionDiffs.length" :diffs="regressionDiffs" />
        <p v-if="!result.regression.passed && allowApprove" class="muted small">
          If this change is intended, review the response and choose <strong>Approve as baseline…</strong>. Otherwise,
          fix the regression.
        </p>
      </div>
      <div v-else-if="tab === 'body' && resp" class="stack">
        <div class="row">
          <span class="muted small">{{ resp.contentType }}</span
          ><span class="spacer" /><BaseButton variant="ghost" small @click="copyBody">Copy</BaseButton>
        </div>
        <pre
          v-if="bodyTokens"
          class="code body json-view"
        ><span v-for="(t, i) in bodyTokens" :key="i" :class="t.kind && `tk-${t.kind}`">{{ t.text }}</span></pre>
        <pre v-else class="code body">{{ body || '(empty body)' }}</pre>
      </div>
      <HeaderTable v-else-if="tab === 'headers' && resp" :headers="resp.headers ?? []" />
      <div v-else-if="tab === 'previous'" class="stack">
        <p v-if="!previous" class="muted">Loading…</p>
        <template v-else-if="!previous.found"
          ><p class="muted">{{ previous.message }}</p></template
        >
        <template v-else>
          <p>
            Compared with the run from {{ formatTime(previous.previousStartedAt) }}: status
            <StatusBadge :status="previous.statusBefore" /> → <StatusBadge :status="previous.statusAfter" />, HTTP
            {{ previous.codeBefore || '—' }} → {{ previous.codeAfter || '—' }}.
          </p>
          <CalloutBox v-if="previous.newlyFailing.length" tone="error">
            <strong>Newly failing tests</strong>
            <ul class="small">
              <li v-for="(m, i) in previous.newlyFailing" :key="i">{{ m }}</li>
            </ul>
          </CalloutBox>
          <DiffTable v-if="previous.diffs?.length" :diffs="previous.diffs" />
          <p v-else class="muted">Response bodies are identical (or not comparable JSON).</p>
        </template>
      </div>
      <div v-else-if="tab === 'console'" class="console mono small">
        <div v-for="(l, i) in logs" :key="i" class="log" :class="l.level">
          <span class="muted phase">{{ l.phase }}</span>
          <span class="message">{{ l.message }}</span>
        </div>
        <p class="muted small">Secret values are redacted.</p>
      </div>
      <div v-else-if="tab === 'request'" class="stack">
        <p class="mono small">
          <MethodBadge :method="result.request.method" />
          {{ result.request.url }}
        </p>
        <HeaderTable v-if="result.request.headers?.length" :headers="result.request.headers" />
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
.perf-slow {
  color: var(--warn);
  font-weight: 600;
}
.perf-slow.fail {
  color: var(--fail);
}
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
.log {
  display: flex;
  gap: 10px;
  padding: 3px 0;
  border-bottom: 1px solid var(--line);
}
.log .phase {
  flex: none;
  width: 92px;
}
.log .message {
  white-space: pre-wrap;
  word-break: break-word;
}
.log.warn .message {
  color: var(--warn);
}
.log.error .message {
  color: var(--fail);
}
</style>

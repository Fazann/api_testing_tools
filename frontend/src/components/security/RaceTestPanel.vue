<script setup lang="ts">
import { computed, ref, watch } from 'vue';

import BaseButton from '@/components/base/BaseButton.vue';
import BaseInput from '@/components/base/BaseInput.vue';
import CalloutBox from '@/components/base/CalloutBox.vue';
import CheckboxField from '@/components/base/CheckboxField.vue';
import DataTable from '@/components/base/DataTable.vue';
import SelectField from '@/components/base/SelectField.vue';
import TextField from '@/components/base/TextField.vue';
import AssertionResults from '@/components/response/AssertionResults.vue';
import { useAsyncAction } from '@/composables/useAsyncAction';
import { api } from '@/services/api';
import { isSocket } from '@/services/socket';
import { useEnvironments } from '@/stores/environments';
import { useUi } from '@/stores/ui';
import { useWorkspace } from '@/stores/workspace';
import type { RaceCheck, RaceReport, RaceTest, RunResult } from '@/types';
import { uid } from '@/utils/object';

const ws = useWorkspace();
const envs = useEnvironments();
const ui = useUi();

const req = computed(() => ws.draft!);
const saved = computed(
  () => !!req.value.id && !!ws.trees[req.value.collectionId]?.requests.some((r) => r.id === req.value.id),
);
const others = computed(() =>
  (ws.trees[req.value.collectionId]?.requests ?? []).filter((r) => r.id !== req.value.id && !isSocket(r.kind)),
);

function defaults(requestId: string): RaceTest {
  return {
    requestId,
    count: 10,
    successOp: 'lte',
    successCount: 1,
    uniquePath: '',
    allowServerErrors: false,
    check: null,
  };
}
function emptyCheck(): RaceCheck {
  return { requestId: '', path: '', deltaPerSuccess: 0, tolerance: 0 };
}

const form = ref<RaceTest>(defaults(''));
const useCheck = ref(false);
const check = ref<RaceCheck>(emptyCheck());
const report = ref<RaceReport | null>(null);
const opId = ref('');
const expanded = ref<string | null>(null);
const { run: runBackend } = useAsyncAction();

watch(
  () => req.value.id,
  async (id) => {
    report.value = null;
    expanded.value = null;
    form.value = defaults(id);
    useCheck.value = false;
    check.value = emptyCheck();
    if (!id) return;
    const t = await runBackend(() => api.getRaceTest(id));
    if (t && req.value.id === id) {
      form.value = { ...defaults(id), ...t, requestId: id };
      if (t.check) {
        useCheck.value = true;
        check.value = { ...t.check };
      }
    }
  },
  { immediate: true },
);

const SUCCESS_RULES = [
  { value: 'lte', label: 'At most' },
  { value: 'eq', label: 'Exactly' },
  { value: 'gte', label: 'At least' },
  { value: '', label: 'No limit' },
] as const;
const requestOptions = computed(() => [
  { value: '', label: 'Choose a request…', disabled: true },
  ...others.value.map((r) => ({ value: r.id, label: `${r.method} ${r.name}` })),
]);
const RESULT_COLUMNS = [
  { key: 'n', label: '#' },
  { key: 'status', label: 'Status' },
  { key: 'durationMs', label: 'Time' },
  { key: 'body', label: 'Body', class: 'mono body-preview' },
];

async function run() {
  if (opId.value) return;
  const t: RaceTest = { ...form.value, requestId: req.value.id, check: useCheck.value ? { ...check.value } : null };
  const env = envs.selected;
  const warnings = await runBackend(() => api.preflightRequest(req.value, envs.selectedId));
  if (!warnings) return;
  const details = warnings.filter((w) => w.level !== 'info' || w.requiresConfirmation).map((w) => w.message);
  if (ws.dirty) details.unshift('This request has unsaved changes. The saved version is sent.');
  const ok = await ui.confirm({
    title: `Send ${t.count} × ${req.value.method} ${req.value.name} at once?`,
    message:
      `All ${t.count} copies are released at the same moment` +
      (env ? ` against "${env.name}"` : '') +
      '. If the request changes data (payments, coupons, stock), it may be applied several times — ' +
      'that is what this test looks for. Use test accounts and controlled data only.',
    details,
    confirmLabel: `Send ${t.count} requests`,
    danger: !!env?.protected || req.value.method !== 'GET',
  });
  if (!ok) return;
  opId.value = uid();
  report.value = null;
  expanded.value = null;
  const id = opId.value;
  const result = await runBackend(() => api.runRaceTest(id, t, envs.selectedId, true));
  if (result) report.value = result;
  opId.value = '';
}

function cancel() {
  if (opId.value) api.cancel(opId.value).catch(ui.error);
}

function statusClass(code: number): string {
  if (code === 0 || code >= 500) return 'fail';
  if (code >= 200 && code < 300) return 'pass';
  return 'muted';
}

function statusLabel(r: RunResult): string {
  return r.response ? String(r.response.statusCode) : r.status === 'cancelled' ? 'cancelled' : 'no response';
}

const verdictText = computed(() => {
  const r = report.value;
  if (!r) return '';
  if (r.cancelled) return 'Cancelled';
  return r.passed ? 'No race condition found' : 'Race condition found';
});
</script>

<template>
  <div class="race stack">
    <p class="muted small intro">
      Sends this request many times at <strong>exactly the same moment</strong> and checks what a correct server
      guarantees under concurrency: a coupon redeemed once, a balance charged once per success, unique IDs, no 5xx. Each
      copy runs its pre-request script first, so it can sign itself or make its own idempotency key.
    </p>

    <CalloutBox v-if="!saved" small>Save this request before running a race test.</CalloutBox>

    <div class="form">
      <TextField v-model="form.count" label="Parallel requests" type="number" min="2" max="50" />
      <div class="field">
        <span>Successful (2xx) responses allowed</span>
        <div class="row">
          <SelectField v-model="form.successOp" :options="SUCCESS_RULES" aria-label="Success rule" />
          <BaseInput
            v-if="form.successOp"
            v-model="form.successCount"
            type="number"
            min="0"
            :max="form.count"
            aria-label="Expected successes"
          />
        </div>
      </div>
      <TextField
        v-model="form.uniquePath"
        label="Must be unique per success (JSON path, optional)"
        input-class="mono"
        placeholder="data.transaction_id"
      />
      <CheckboxField v-model="form.allowServerErrors" class="small allow5xx" label="Accept 5xx responses" />
    </div>

    <CheckboxField v-model="useCheck">
      <strong>Check the state before and after</strong> — catches lost updates and double charges
    </CheckboxField>
    <div v-if="useCheck" class="form check">
      <SelectField v-model="check.requestId" label="Request that reads the value" :options="requestOptions" />
      <TextField v-model="check.path" label="JSON path to a number" input-class="mono" placeholder="data.balance" />
      <TextField
        v-model="check.deltaPerSuccess"
        label="Change per success"
        type="number"
        step="any"
        placeholder="-10"
      />
      <TextField v-model="check.tolerance" label="Tolerance" type="number" step="any" min="0" />
      <p class="muted small full">
        Example: a withdrawal of 10 reads <code>data.balance</code> with change <code>-10</code>. With 3 successful
        responses the balance must drop by exactly 30.
      </p>
    </div>

    <div class="row">
      <BaseButton v-if="opId" variant="danger" @click="cancel">Cancel</BaseButton>
      <BaseButton v-else variant="primary" :disabled="!saved" @click="run">Run race test</BaseButton>
      <span v-if="opId" class="muted small">Sending {{ form.count }} requests…</span>
    </div>

    <section v-if="report" class="report stack">
      <div class="row verdict" :class="report.cancelled ? 'warn' : report.passed ? 'pass' : 'fail'">
        <strong>{{ report.passed ? '✔' : '✘' }} {{ verdictText }}</strong>
        <span class="spacer" />
        <span class="muted small">
          {{ report.successes }} of {{ report.count }} succeeded · {{ report.minMs }}–{{ report.maxMs }} ms (avg
          {{ report.avgMs }})
        </span>
      </div>
      <div class="row codes">
        <span v-for="s in report.statusCounts" :key="s.code" class="chip" :class="statusClass(s.code)">
          {{ s.code === 0 ? 'no response' : s.code }} × {{ s.count }}
        </span>
        <span v-if="report.before !== undefined" class="muted small">
          State: {{ report.before }} → {{ report.after ?? '?' }}
        </span>
      </div>
      <AssertionResults :results="report.checks" />

      <h3 class="section-title">Responses</h3>
      <DataTable
        class="responses"
        :columns="RESULT_COLUMNS"
        :rows="report.results"
        :row-key="(r) => r.id"
        clickable
        @row-click="(r) => (expanded = expanded === r.id ? null : r.id)"
      >
        <template #cell-n="{ index }">{{ index + 1 }}</template>
        <template #cell-status="{ row }">
          <span :class="statusClass(row.response?.statusCode ?? 0)">{{ statusLabel(row) }}</span>
        </template>
        <template #cell-durationMs="{ row }">{{ row.durationMs }} ms</template>
        <template #cell-body="{ row }">{{ row.error || row.response?.body.slice(0, 120) }}</template>
        <template #after-row="{ row }">
          <tr v-if="expanded === row.id">
            <td colspan="4">
              <pre class="code">{{ row.error || row.response?.body }}</pre>
            </td>
          </tr>
        </template>
      </DataTable>
      <p class="muted small">The run is saved in History with every response and the verdict.</p>
    </section>
  </div>
</template>

<style scoped>
.intro {
  margin: 0;
  max-width: 760px;
}
.form {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 10px 16px;
  max-width: 900px;
  align-items: end;
}
.form :deep(input[type='number']) {
  width: 110px;
}
.check {
  padding-left: 22px;
}
.full {
  grid-column: 1 / -1;
  margin: 0;
}
.allow5xx {
  padding-bottom: 6px;
}
.report {
  border-top: 1px solid var(--line);
  padding-top: 10px;
}
.verdict {
  font-size: 14px;
}
.codes {
  flex-wrap: wrap;
}
.chip {
  font-family: var(--mono);
  font-size: 12px;
  padding: 1px 8px;
  border: 1px solid var(--line);
  border-radius: 999px;
}
.responses :deep(.body-preview) {
  max-width: 520px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>

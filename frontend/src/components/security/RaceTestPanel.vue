<script setup lang="ts">
import { computed, ref, watch } from 'vue';

import { api } from '../services/api';
import { uid } from '../services/model';
import { useEnvironments } from '../stores/environments';
import { useUi } from '../stores/ui';
import { useWorkspace } from '../stores/workspace';
import type { RaceCheck, RaceReport, RaceTest, RunResult } from '../types';
import AssertionResults from './AssertionResults.vue';

const ws = useWorkspace();
const envs = useEnvironments();
const ui = useUi();

const req = computed(() => ws.draft!);
const saved = computed(
  () => !!req.value.id && !!ws.trees[req.value.collectionId]?.requests.some((r) => r.id === req.value.id),
);
const others = computed(() => (ws.trees[req.value.collectionId]?.requests ?? []).filter((r) => r.id !== req.value.id));

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

watch(
  () => req.value.id,
  async (id) => {
    report.value = null;
    expanded.value = null;
    form.value = defaults(id);
    useCheck.value = false;
    check.value = emptyCheck();
    if (!id) return;
    try {
      const t = await api.getRaceTest(id);
      if (t && req.value.id === id) {
        form.value = { ...defaults(id), ...t, requestId: id };
        if (t.check) {
          useCheck.value = true;
          check.value = { ...t.check };
        }
      }
    } catch (e) {
      ui.error(e);
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

async function run() {
  if (opId.value) return;
  const t: RaceTest = { ...form.value, requestId: req.value.id, check: useCheck.value ? { ...check.value } : null };
  const env = envs.selected;
  try {
    const warnings = await api.preflightRequest(req.value, envs.selectedId);
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
    report.value = await api.runRaceTest(opId.value, t, envs.selectedId, true);
  } catch (e) {
    ui.error(e);
  } finally {
    opId.value = '';
  }
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

    <p v-if="!saved" class="callout small">Save this request before running a race test.</p>

    <div class="form">
      <label class="field">
        <span>Parallel requests</span>
        <input v-model.number="form.count" type="number" min="2" max="50" />
      </label>
      <div class="field">
        <span>Successful (2xx) responses allowed</span>
        <div class="row">
          <select v-model="form.successOp" aria-label="Success rule">
            <option v-for="r in SUCCESS_RULES" :key="r.value" :value="r.value">{{ r.label }}</option>
          </select>
          <input
            v-if="form.successOp"
            v-model.number="form.successCount"
            type="number"
            min="0"
            :max="form.count"
            aria-label="Expected successes"
          />
        </div>
      </div>
      <label class="field">
        <span>Must be unique per success (JSON path, optional)</span>
        <input v-model="form.uniquePath" class="mono" placeholder="data.transaction_id" />
      </label>
      <label class="row small allow5xx">
        <input v-model="form.allowServerErrors" type="checkbox" /> Accept 5xx responses
      </label>
    </div>

    <label class="row">
      <input v-model="useCheck" type="checkbox" />
      <span><strong>Check the state before and after</strong> — catches lost updates and double charges</span>
    </label>
    <div v-if="useCheck" class="form check">
      <label class="field">
        <span>Request that reads the value</span>
        <select v-model="check.requestId">
          <option value="" disabled>Choose a request…</option>
          <option v-for="r in others" :key="r.id" :value="r.id">{{ r.method }} {{ r.name }}</option>
        </select>
      </label>
      <label class="field">
        <span>JSON path to a number</span>
        <input v-model="check.path" class="mono" placeholder="data.balance" />
      </label>
      <label class="field">
        <span>Change per success</span>
        <input v-model.number="check.deltaPerSuccess" type="number" step="any" placeholder="-10" />
      </label>
      <label class="field">
        <span>Tolerance</span>
        <input v-model.number="check.tolerance" type="number" step="any" min="0" />
      </label>
      <p class="muted small full">
        Example: a withdrawal of 10 reads <code>data.balance</code> with change <code>-10</code>. With 3 successful
        responses the balance must drop by exactly 30.
      </p>
    </div>

    <div class="row">
      <button v-if="opId" class="danger" @click="cancel">Cancel</button>
      <button v-else class="primary" :disabled="!saved" @click="run">Run race test</button>
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
      <table class="grid responses">
        <thead>
          <tr>
            <th>#</th>
            <th>Status</th>
            <th>Time</th>
            <th>Body</th>
          </tr>
        </thead>
        <tbody>
          <template v-for="(r, i) in report.results" :key="r.id">
            <tr class="clickable" @click="expanded = expanded === r.id ? null : r.id">
              <td>{{ i + 1 }}</td>
              <td :class="statusClass(r.response?.statusCode ?? 0)">{{ statusLabel(r) }}</td>
              <td>{{ r.durationMs }} ms</td>
              <td class="mono body-preview">{{ r.error || r.response?.body.slice(0, 120) }}</td>
            </tr>
            <tr v-if="expanded === r.id">
              <td colspan="4">
                <pre class="code">{{ r.error || r.response?.body }}</pre>
              </td>
            </tr>
          </template>
        </tbody>
      </table>
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
.form input[type='number'] {
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
.clickable {
  cursor: pointer;
}
.clickable:hover {
  background: var(--bg-subtle);
}
.body-preview {
  max-width: 520px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>

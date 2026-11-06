<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';

import BaseButton from '@/components/base/BaseButton.vue';
import BaseInput from '@/components/base/BaseInput.vue';
import CalloutBox from '@/components/base/CalloutBox.vue';
import CheckboxField from '@/components/base/CheckboxField.vue';
import DataTable from '@/components/base/DataTable.vue';
import FormField from '@/components/base/FormField.vue';
import MethodBadge from '@/components/base/MethodBadge.vue';
import SelectField from '@/components/base/SelectField.vue';
import StatTile from '@/components/base/StatTile.vue';
import DiffTable from '@/components/response/DiffTable.vue';
import { useAsyncAction } from '@/composables/useAsyncAction';
import { api, onCompareProgress } from '@/services/api';
import { isSocket } from '@/services/socket';
import { useEnvironments } from '@/stores/environments';
import { useUi } from '@/stores/ui';
import type { CollectionTree, CompareReport, CompareResult, CompareSide, EnvComparison, Request } from '@/types';
import { formatDuration } from '@/utils/format';
import { isDestructive } from '@/utils/http';
import { lines, uid } from '@/utils/object';

// Compares the collection between two environments and lists what differs. See docs/environment-comparison.md.
const props = defineProps<{ tree: CollectionTree }>();
const emit = defineEmits<{ open: [r: Request] }>();
const envs = useEnvironments();
const ui = useUi();
const { run } = useAsyncAction();

const cfg = ref<EnvComparison | null>(null);
const ignored = ref('');
const report = ref<CompareReport | null>(null);
const showSame = ref(false);
const expanded = ref<string | null>(null);
const opId = ref('');
const progress = ref({ done: 0, total: 0 });
let unsubscribe: (() => void) | null = null;
onBeforeUnmount(() => unsubscribe?.());

const IGNORED_PLACEHOLDER = 'meta.request_id\nitems[*].updated_at\nheader:X-Build-Id';
const envOptions = computed(() => envs.list.map((e) => ({ value: e.id, label: e.name + (e.protected ? ' 🔒' : '') })));
const envName = (id: string) => envs.list.find((e) => e.id === id)?.name ?? '';

onMounted(async () => {
  const c = await run(() => api.getEnvComparison(props.tree.collection.id));
  if (!c) return;
  // Pre-fill with the selected environment and another one, so the first run takes one click.
  const ids = envs.list.map((e) => e.id);
  if (!ids.includes(c.envA))
    c.envA = envs.selectedId && ids.includes(envs.selectedId) ? envs.selectedId : (ids[0] ?? '');
  if (!ids.includes(c.envB) || c.envB === c.envA) c.envB = ids.find((id) => id !== c.envA) ?? '';
  cfg.value = c;
  c.ignoredPaths ??= [];
  ignored.value = c.ignoredPaths.join('\n');
});

const http = computed(() => props.tree.requests.filter((r) => !isSocket(r.kind)));
const modifying = computed(() => http.value.filter((r) => isDestructive(r.method)));
const compared = computed(() =>
  cfg.value?.includeModifying ? http.value : http.value.filter((r) => !isDestructive(r.method)),
);

function swap() {
  if (!cfg.value) return;
  [cfg.value.envA, cfg.value.envB] = [cfg.value.envB, cfg.value.envA];
}

async function compare() {
  const c = cfg.value;
  if (!c || opId.value) return;
  if (!c.envA || !c.envB) return ui.toast('Choose the two environments to compare', 'error');
  if (c.envA === c.envB) return ui.toast('Choose two different environments', 'error');
  if (compared.value.length === 0)
    return ui.toast('There are no read-only HTTP requests to compare; include requests that may modify data', 'error');
  const protectedEnvs = [c.envA, c.envB].filter((id) => envs.list.find((e) => e.id === id)?.protected);
  let confirmed = false;
  if (c.includeModifying && modifying.value.length > 0) {
    confirmed = await ui.confirm({
      title: 'Send requests that may modify data to both environments?',
      message:
        `${modifying.value.length} request(s) may create or change data in ` +
        `"${envName(c.envA)}" and "${envName(c.envB)}".`,
      details: [
        modifying.value
          .slice(0, 5)
          .map((r) => `${r.method} ${r.name}`)
          .join(', '),
        ...protectedEnvs.map((id) => `"${envName(id)}" is a protected environment.`),
      ],
      confirmLabel: 'Compare anyway',
      danger: true,
    });
    if (!confirmed) return;
  }
  c.ignoredPaths = lines(ignored.value);
  const id = uid();
  opId.value = id;
  expanded.value = null;
  const detectCount = c.detectChanging ? compared.value.filter((r) => !isDestructive(r.method)).length : 0;
  progress.value = { done: 0, total: 2 * (compared.value.length + detectCount) };
  unsubscribe?.();
  unsubscribe = onCompareProgress((p) => {
    if (p.opId === id) progress.value = { done: p.done, total: p.total };
  });
  const result = await run(() => api.compareEnvironments(id, { ...c, ignoredPaths: [...c.ignoredPaths] }, confirmed));
  opId.value = '';
  unsubscribe?.();
  unsubscribe = null;
  if (result) report.value = result;
}

function cancel() {
  if (opId.value) api.cancel(opId.value).catch(ui.error);
}

const rows = computed(() =>
  (report.value?.requests ?? []).filter(
    (r) => showSame.value || (r.verdict !== 'same' && r.verdict !== 'skipped') || r.slower,
  ),
);
const COLUMNS = [
  { key: 'name', label: 'Request' },
  { key: 'verdict', label: 'Result', width: '150px' },
  { key: 'a', label: 'A', width: '130px' },
  { key: 'b', label: 'B', width: '130px' },
  { key: 'diffs', label: 'Differences', class: 'num', width: '110px' },
];
const VERDICT_LABEL: Record<CompareResult['verdict'], string> = {
  same: 'Same',
  different: 'Different',
  error: 'Error',
  skipped: 'Skipped',
};
const VERDICT_TONE: Record<CompareResult['verdict'], string> = {
  same: 'pass',
  different: 'fail',
  error: 'fail',
  skipped: 'muted',
};

function sideText(s: CompareSide, verdict: CompareResult['verdict']): string {
  if (verdict === 'skipped') return '—';
  if (!s.statusCode) return s.status === 'cancelled' ? 'cancelled' : 'no response';
  return `${s.statusCode} · ${formatDuration(s.durationMs)}`;
}

function diffCount(r: CompareResult): number {
  return (r.statusChanged ? 1 : 0) + r.headers.length + r.body.length + (r.bodyMessage && !r.body.length ? 1 : 0);
}

function requestOf(r: CompareResult): Request | undefined {
  return props.tree.requests.find((q) => q.id === r.requestId);
}

function toggle(r: CompareResult) {
  expanded.value = expanded.value === r.requestId ? null : r.requestId;
}

function ratio(r: CompareResult): string {
  const [slow, fast] = r.slower === 'a' ? [r.a.durationMs, r.b.durationMs] : [r.b.durationMs, r.a.durationMs];
  return fast > 0 ? ` (${(slow / fast).toFixed(1)}×)` : '';
}
</script>

<template>
  <div class="compare stack">
    <p class="muted small">
      Runs this collection against two environments at the same time and shows only what differs: status codes, body
      fields, headers, errors and much slower responses. No baseline needed.
    </p>
    <CalloutBox v-if="envs.list.length < 2" small>
      Create at least two environments (for example <strong>Production</strong> and <strong>Staging</strong>, or the old
      and the new build) to compare them.
    </CalloutBox>
    <template v-else-if="cfg">
      <div class="row envs">
        <SelectField
          v-model="cfg.envA"
          class="grow"
          label="A — reference (e.g. Production, old build)"
          :options="envOptions"
        />
        <BaseButton class="swap" title="Swap A and B" aria-label="Swap A and B" @click="swap">⇄</BaseButton>
        <SelectField
          v-model="cfg.envB"
          class="grow"
          label="B — to check (e.g. Staging, new build)"
          :options="envOptions"
        />
      </div>
      <CheckboxField v-model="cfg.detectChanging" class="small">
        Detect changing fields — send each read-only request a second time to each environment and leave out fields
        whose value changes between calls (IDs, timestamps, tokens)
      </CheckboxField>
      <CheckboxField v-model="cfg.includeModifying" class="small">
        Include requests that may modify data ({{ modifying.length }}: POST, PUT, PATCH, DELETE) — sent once to each
        environment
      </CheckboxField>
      <FormField label="Ignored paths (one per line; header:Name ignores a header). Baseline ignored paths apply too.">
        <BaseInput v-model="ignored" multiline rows="3" :placeholder="IGNORED_PLACEHOLDER" />
      </FormField>
      <div class="row">
        <BaseButton v-if="!opId" variant="primary" @click="compare">
          Compare {{ compared.length }} request{{ compared.length === 1 ? '' : 's' }}
        </BaseButton>
        <template v-else>
          <BaseButton variant="danger" @click="cancel">Cancel</BaseButton>
          <span class="muted small">Comparing… {{ progress.done }} / {{ progress.total }}</span>
        </template>
        <span class="muted small">Settings are saved with the collection; the result is not stored.</span>
      </div>
    </template>

    <template v-if="report">
      <h3 class="section-title">
        {{ report.a.name }} <span class="muted">vs</span> {{ report.b.name }}
        <span v-if="report.cancelled" class="fail small">— cancelled</span>
      </h3>
      <div class="tiles">
        <StatTile label="Same" :value="report.same" tone="pass" />
        <StatTile label="Different" :value="report.different" :tone="report.different ? 'fail' : undefined" />
        <StatTile label="Errors" :value="report.errors" :tone="report.errors ? 'fail' : undefined" />
        <StatTile label="Slower" :value="report.slower" :tone="report.slower ? 'warn' : undefined" />
        <StatTile label="Skipped" :value="report.skipped" />
      </div>
      <div class="row small">
        <CheckboxField v-model="showSame" label="Show identical and skipped requests" />
        <span class="grow" />
        <BaseButton small variant="ghost" @click="ui.showRun(report.runIdA)">Open A in history</BaseButton>
        <BaseButton small variant="ghost" @click="ui.showRun(report.runIdB)">Open B in history</BaseButton>
      </div>
      <DataTable
        :columns="COLUMNS"
        :rows="rows"
        :row-key="(r) => r.requestId"
        :row-class="(r) => ({ active: expanded === r.requestId })"
        clickable
        :empty="
          report.requests.length ? 'No differences — every compared request behaves the same.' : 'Nothing to compare.'
        "
        @row-click="toggle"
      >
        <template #cell-name="{ row }">
          <span class="row name">
            <MethodBadge :method="row.method" />
            <span>{{ row.name }}</span>
          </span>
        </template>
        <template #cell-verdict="{ row }">
          <span :class="VERDICT_TONE[row.verdict]">{{ VERDICT_LABEL[row.verdict] }}</span>
          <span v-if="row.slower" class="warn small"> · slower in {{ row.slower.toUpperCase() }}</span>
        </template>
        <template #cell-a="{ row }">
          <span class="mono small" :class="{ warn: row.slower === 'a' }">{{ sideText(row.a, row.verdict) }}</span>
        </template>
        <template #cell-b="{ row }">
          <span class="mono small" :class="{ warn: row.slower === 'b' }">{{ sideText(row.b, row.verdict) }}</span>
        </template>
        <template #cell-diffs="{ row }">{{ row.verdict === 'different' ? diffCount(row) : '' }}</template>
        <template #after-row="{ row }">
          <div v-if="expanded === row.requestId" class="detail stack small">
            <p v-if="row.skipReason" class="muted">Skipped: {{ row.skipReason }}.</p>
            <template v-if="row.verdict === 'error'">
              <p v-if="!row.a.statusCode" class="fail">A ({{ report.a.name }}): {{ row.a.error }}</p>
              <p v-if="!row.b.statusCode" class="fail">B ({{ report.b.name }}): {{ row.b.error }}</p>
            </template>
            <p v-if="row.statusChanged">
              <strong>Status</strong> <code>{{ row.a.statusCode }}</code> → <code>{{ row.b.statusCode }}</code>
            </p>
            <p v-if="row.slower">
              <strong>Timing</strong> {{ formatDuration(row.a.durationMs) }} vs {{ formatDuration(row.b.durationMs) }} —
              slower in {{ row.slower === 'a' ? report.a.name : report.b.name }}{{ ratio(row) }}
            </p>
            <template v-if="row.headers.length">
              <strong>Headers</strong>
              <DiffTable :diffs="row.headers" before-label="A" after-label="B" />
            </template>
            <p v-if="row.bodyMessage"><strong>Body</strong> {{ row.bodyMessage }}</p>
            <template v-if="row.body.length">
              <strong>Body</strong>
              <DiffTable :diffs="row.body" before-label="A" after-label="B" />
            </template>
            <p v-if="row.changing.length" class="muted">
              Left out as changing between calls:
              <code v-for="p in row.changing" :key="p" class="chip">{{ p }}</code>
            </p>
            <div class="row">
              <BaseButton v-if="requestOf(row)" small @click="emit('open', requestOf(row)!)">Open request</BaseButton>
            </div>
          </div>
        </template>
      </DataTable>
    </template>
  </div>
</template>

<style scoped>
.envs {
  align-items: flex-end;
}
.swap {
  margin-bottom: 2px;
}
.name {
  gap: 6px;
}
.detail {
  padding: 8px 12px 12px;
  background: var(--bg-subtle);
  border-bottom: 1px solid var(--line);
}
.chip {
  margin-right: 4px;
}
</style>

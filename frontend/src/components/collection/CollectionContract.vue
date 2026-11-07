<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';

import { api } from '@/api';
import BaseButton from '@/components/base/BaseButton.vue';
import CalloutBox from '@/components/base/CalloutBox.vue';
import CheckboxField from '@/components/base/CheckboxField.vue';
import DataTable from '@/components/base/DataTable.vue';
import EmptyState from '@/components/base/EmptyState.vue';
import MethodBadge from '@/components/base/MethodBadge.vue';
import SelectField from '@/components/base/SelectField.vue';
import { useAsyncAction } from '@/composables/useAsyncAction';
import { useEnvironments } from '@/stores/environments';
import { useUi } from '@/stores/ui';
import type { CollectionTree, ContractCoverage, ContractInfo, Request } from '@/types';
import { formatTime } from '@/utils/format';

// The OpenAPI spec attached to a collection and the requests it covers (docs/openapi-contract.md).
const props = defineProps<{ tree: CollectionTree }>();
const emit = defineEmits<{ open: [request: Request] }>();
const ui = useUi();
const envs = useEnvironments();
const { busy, run } = useAsyncAction();

const id = computed(() => props.tree.collection.id);
const info = ref<ContractInfo | null>(null);
const coverage = ref<ContractCoverage | null>(null);
const envId = ref(envs.selectedId);
const envOptions = computed(() => [
  { value: '', label: 'No environment' },
  ...envs.list.map((e) => ({ value: e.id, label: e.name })),
]);

async function loadCoverage() {
  coverage.value = info.value?.attached ? await api.getContractCoverage(id.value, envId.value) : null;
}

async function load() {
  await run(async () => {
    info.value = await api.getContract(id.value);
    await loadCoverage();
  });
}
onMounted(load);
watch(envId, () => run(loadCoverage));

async function attach() {
  await run(async () => {
    const res = await api.attachContract(id.value);
    if (!res) return;
    const replaced = info.value?.attached;
    info.value = res;
    await loadCoverage();
    ui.toast(`${replaced ? 'Replaced' : 'Attached'} the spec: ${res.operations} operations`, 'success');
  });
}

async function setOptions(enabled: boolean, allowUndocumented: boolean) {
  await run(async () => {
    info.value = await api.setContractOptions(id.value, enabled, allowUndocumented);
  });
}

async function remove() {
  const ok = await ui.confirm({
    title: 'Remove the OpenAPI spec?',
    message: 'Responses will no longer be checked against it. Requests are not changed.',
    confirmLabel: 'Remove',
    danger: true,
  });
  if (!ok) return;
  await run(async () => {
    await api.removeContract(id.value);
    info.value = await api.getContract(id.value);
    coverage.value = null;
  });
}

const ATTACH_HINT =
  'Attach the OpenAPI 3 or Swagger 2 file (JSON or YAML) that documents this API. ' +
  'Importing an OpenAPI file attaches it automatically.';
const UNDOCUMENTED_HINT =
  'Accept response fields the spec does not mention. Wrong types and missing required fields are still reported.';

const requestsById = computed(() => new Map(props.tree.requests.map((r) => [r.id, r])));
const uncovered = computed(() => coverage.value?.requests.filter((r) => !r.operation) ?? []);
const untested = computed(() => coverage.value?.operations.filter((o) => o.requests === 0) ?? []);

const REQUEST_COLUMNS = [
  { key: 'name', label: 'Request' },
  { key: 'operation', label: 'Operation in the spec', class: 'mono' },
];
const OPERATION_COLUMNS = [
  { key: 'operation', label: 'Operation', class: 'mono' },
  { key: 'summary', label: 'Summary' },
  { key: 'requests', label: 'Requests', class: 'num' },
];

function openRow(row: { requestId: string }) {
  const r = requestsById.value.get(row.requestId);
  if (r) emit('open', r);
}
</script>

<template>
  <p class="muted small">
    Every response of this collection's requests is checked against the OpenAPI spec: field types, required and
    undocumented fields, enums and formats, status codes, Content-Type and required headers. Problems appear in the
    request's Tests as <strong>Contract</strong> checks and fail the run.
  </p>

  <p v-if="!info" class="muted small">Loading…</p>

  <EmptyState v-else-if="!info.attached" small title="No spec attached" :message="ATTACH_HINT">
    <BaseButton variant="primary" :disabled="busy" @click="attach">Attach spec…</BaseButton>
  </EmptyState>

  <template v-else>
    <div class="spec row">
      <div class="grow">
        <strong>{{ info.title || 'Untitled spec' }}</strong>
        <span v-if="info.version" class="muted"> {{ info.version }}</span>
        <div class="muted small">
          {{ info.operations }} operation{{ info.operations === 1 ? '' : 's' }}
          <template v-if="info.sourceName"> · from {{ info.sourceName }}</template>
          · attached {{ formatTime(info.updatedAt) }}
        </div>
      </div>
      <BaseButton :disabled="busy" @click="attach">Replace spec…</BaseButton>
      <BaseButton variant="danger" :disabled="busy" @click="remove">Remove</BaseButton>
    </div>

    <CheckboxField
      :model-value="info.enabled"
      label="Check responses against the spec"
      hint="Turning this off keeps the spec."
      :disabled="busy"
      @update:model-value="setOptions($event === true, info.allowUndocumented)"
    />
    <CheckboxField
      :model-value="info.allowUndocumented"
      label="Allow undocumented fields"
      :hint="UNDOCUMENTED_HINT"
      :disabled="busy || !info.enabled"
      @update:model-value="setOptions(info.enabled, $event === true)"
    />

    <div class="row coverage-head">
      <h3 class="section-title grow">Coverage</h3>
      <SelectField v-model="envId" :options="envOptions" aria-label="Environment for resolving URLs" />
    </div>
    <p class="muted small">
      Requests are matched by method and URL path, with variables from the selected environment. Nothing is sent.
    </p>

    <template v-if="coverage">
      <CalloutBox v-if="uncovered.length" tone="warn" small>
        {{ uncovered.length }} request{{ uncovered.length === 1 ? ' is' : 's are' }} not in the spec and will not be
        checked.
      </CalloutBox>
      <DataTable
        :columns="REQUEST_COLUMNS"
        :rows="coverage.requests"
        :row-key="(r) => r.requestId"
        clickable
        empty="This collection has no HTTP requests."
        @row-click="openRow"
      >
        <template #cell-name="{ row }"><MethodBadge :method="row.method" /> {{ row.name }}</template>
        <template #cell-operation="{ row }">
          <span v-if="row.operation">{{ row.operation }}</span>
          <span v-else class="warn" :title="row.problem">{{
            row.problem ? 'URL not resolved' : 'not in the spec'
          }}</span>
        </template>
      </DataTable>

      <h3 class="section-title">Operations without a request ({{ untested.length }})</h3>
      <DataTable
        :columns="OPERATION_COLUMNS"
        :rows="untested"
        :row-key="(o) => o.operation"
        empty="Every operation of the spec has a request."
      />
    </template>
  </template>
</template>

<style scoped>
.spec {
  gap: 8px;
  align-items: center;
  margin: 8px 0 12px;
}
.coverage-head {
  align-items: center;
  gap: 8px;
}
</style>

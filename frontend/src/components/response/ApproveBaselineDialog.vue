<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';

import BaseButton from '@/components/base/BaseButton.vue';
import BaseInput from '@/components/base/BaseInput.vue';
import CalloutBox from '@/components/base/CalloutBox.vue';
import CheckboxField from '@/components/base/CheckboxField.vue';
import FormField from '@/components/base/FormField.vue';
import ModalFrame from '@/components/base/ModalFrame.vue';
import RadioGroup from '@/components/base/RadioGroup.vue';
import TextField from '@/components/base/TextField.vue';
import { useAsyncAction } from '@/composables/useAsyncAction';
import { api } from '@/services/api';
import { jsonPaths } from '@/services/model';
import { useEnvironments } from '@/stores/environments';
import { useUi } from '@/stores/ui';
import { useWorkspace } from '@/stores/workspace';
import type { RunResult } from '@/types';
import { lines, mergeLines } from '@/utils/object';

import ChangingFieldsDetector from './ChangingFieldsDetector.vue';

const props = defineProps<{ result: RunResult; requestId: string; existingIgnored?: string[] }>();
const emit = defineEmits<{ close: []; approved: [] }>();
const envs = useEnvironments();
const ui = useUi();
const ws = useWorkspace();
/** The request the response belongs to, as open in the editor (detection sends it again). */
const request = computed(() => (ws.draft?.id === props.requestId ? ws.draft : null));

type Mode = 'all' | 'selected' | 'schema';
const mode = ref<Mode>('all');
const MODES: { value: Mode; label: string }[] = [
  { value: 'all', label: 'Compare the whole response' },
  { value: 'selected', label: 'Compare selected stable fields only' },
  { value: 'schema', label: 'Compare structure only (field names and types)' },
];
const IGNORED_PLACEHOLDER = 'meta.request_id\nitems[*].updated_at\n**.timestamp';
const filter = ref('');
const selected = ref<string[]>([]);
const ignored = ref((props.existingIgnored ?? []).join('\n'));
const tolerance = ref(0);
/** Shown in sign-off reports; defaults to Settings → Your name. */
const approvedBy = ref('');
onMounted(async () => {
  approvedBy.value = (await api.getSettings().catch(() => null))?.reviewerName ?? '';
});
const { busy, run: runBusy } = useAsyncAction();

const doc = computed(() => {
  try {
    return JSON.parse(props.result.response?.body ?? '');
  } catch {
    return undefined;
  }
});
const paths = computed(() => (doc.value === undefined ? [] : jsonPaths(doc.value)));
const visiblePaths = computed(() =>
  paths.value.filter((p) => p.toLowerCase().includes(filter.value.toLowerCase())).slice(0, 300),
);
const suggestions = computed(() =>
  paths.value.filter((p) => /(time|date|_at$|At$|request_?id|trace|nonce|token|uuid)/i.test(p)),
);

function addSuggestions() {
  ignored.value = mergeLines(
    ignored.value,
    suggestions.value.map((s) => s.replace(/\[\d+\]/g, '[*]')),
  );
}

async function approve() {
  if (mode.value === 'selected' && selected.value.length === 0) {
    ui.toast('Select at least one path, or compare the whole response.', 'error');
    return;
  }
  await runBusy(async () => {
    await api.approveBaseline({
      requestId: props.requestId,
      environmentId: envs.selectedId,
      body: props.result.response?.body ?? '',
      truncated: props.result.response?.truncated ?? false,
      selectedPaths: mode.value === 'selected' ? selected.value : [],
      ignoredPaths: ignored.value.split('\n'),
      tolerance: mode.value === 'schema' ? 0 : tolerance.value || 0,
      mode: mode.value === 'schema' ? 'schema' : 'values',
      approvedBy: approvedBy.value,
    });
    ui.toast('Baseline approved', 'success');
    emit('approved');
    emit('close');
  });
}
</script>

<template>
  <ModalFrame title="Approve response as baseline" wide @close="emit('close')">
    <CalloutBox small>
      A baseline is not automatically correct. Only approve this response after verifying it is right for
      <strong>{{ envs.selected?.name ?? 'No environment' }}</strong
      >. Future runs in this environment will be compared against it.
    </CalloutBox>
    <CalloutBox v-if="doc === undefined" tone="error">
      The response body is not valid JSON, so it cannot be used as a baseline.
    </CalloutBox>
    <template v-else>
      <RadioGroup v-model="mode" name="baseline-mode" label="What to compare" :options="MODES" />
      <p v-if="mode === 'schema'" class="muted small">
        Values are not compared. A run fails when a field appears or disappears, or its type changes (for example a
        number becomes a string, or an object becomes <code>null</code>). Array items are checked against all items seen
        now; fields missing from some items are optional. Fields that are <code>null</code> now, and empty arrays,
        accept any value later.
      </p>
      <div v-if="mode === 'selected'" class="stack">
        <BaseInput v-model="filter" placeholder="Filter paths…" aria-label="Filter paths" />
        <div class="paths">
          <CheckboxField
            v-for="p in visiblePaths"
            :key="p"
            v-model="selected"
            class="small mono"
            :value="p"
            :label="p"
          />
        </div>
        <span class="muted small">{{ selected.length }} selected</span>
      </div>
      <FormField>
        <template #label>
          <span class="row"
            >Ignored paths (one per line)
            <BaseButton v-if="suggestions.length" variant="ghost" small @click="addSuggestions">
              Add likely dynamic fields ({{ suggestions.length }})
            </BaseButton></span
          >
        </template>
        <BaseInput v-model="ignored" multiline rows="4" :placeholder="IGNORED_PLACEHOLDER" />
      </FormField>
      <ChangingFieldsDetector
        :request="request"
        :ignored="lines(ignored)"
        @add="ignored = mergeLines(ignored, $event)"
      />
      <TextField
        v-if="mode !== 'schema'"
        v-model="tolerance"
        class="tol"
        label="Numeric tolerance (absolute)"
        type="number"
        min="0"
        step="any"
      />
      <TextField
        v-model="approvedBy"
        label="Approved by — shown in sign-off reports (default: Settings → Your name)"
        maxlength="200"
        placeholder="Your name"
      />
    </template>
    <template #footer>
      <BaseButton @click="emit('close')">Cancel</BaseButton>
      <BaseButton variant="primary" :disabled="busy || doc === undefined" @click="approve">Approve baseline</BaseButton>
    </template>
  </ModalFrame>
</template>

<style scoped>
.paths {
  max-height: 220px;
  overflow: auto;
  border: 1px solid var(--line);
  border-radius: var(--radius);
  padding: 6px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.tol :deep(input) {
  width: 140px;
}
</style>

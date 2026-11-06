<script setup lang="ts">
import { computed, ref, watch } from 'vue';

import BaseButton from '@/components/base/BaseButton.vue';
import CalloutBox from '@/components/base/CalloutBox.vue';
import TextField from '@/components/base/TextField.vue';
import { useAsyncAction } from '@/composables/useAsyncAction';
import { api } from '@/services/api';
import { useEnvironments } from '@/stores/environments';
import { useUi } from '@/stores/ui';
import { useWorkspace } from '@/stores/workspace';
import type { Baseline } from '@/types';
import { formatTime } from '@/utils/format';
import { lines, mergeLines } from '@/utils/object';

import ChangingFieldsDetector from './ChangingFieldsDetector.vue';

const ws = useWorkspace();
const envs = useEnvironments();
const ui = useUi();

const baseline = ref<Baseline | null>(null);
const ignored = ref('');
const tolerance = ref(0);
const { busy: loading, run: runLoad } = useAsyncAction();
const { run } = useAsyncAction();
const IGNORED_PLACEHOLDER = 'meta.request_id\nitems[*].updated_at\n**.timestamp';

const requestId = computed(() => ws.draft?.id ?? '');

async function load() {
  baseline.value = null;
  if (!requestId.value) return;
  const id = requestId.value;
  await runLoad(async () => {
    baseline.value = await api.getBaseline(id, envs.selectedId);
    ignored.value = baseline.value?.ignoredPaths.join('\n') ?? '';
    tolerance.value = baseline.value?.tolerance ?? 0;
  });
}
watch([requestId, () => envs.selectedId, () => ws.currentResult?.regression?.baselineId], load, { immediate: true });
defineExpose({ load });

async function saveRules() {
  const b = baseline.value;
  if (!b) return;
  const saved = await run(async () => {
    await api.updateBaselineRules(b.id, ignored.value.split('\n'), tolerance.value || 0);
    ui.toast('Baseline rules saved. They apply from the next run.', 'success');
    return true;
  });
  if (saved) await load();
}

async function remove() {
  if (!baseline.value) return;
  const ok = await ui.confirm({
    title: 'Delete baseline?',
    message: 'Future runs will not be compared until you approve a new baseline.',
    confirmLabel: 'Delete',
    danger: true,
  });
  const b = baseline.value;
  if (!ok || !b) return;
  await run(async () => {
    await api.deleteBaseline(b.id);
    baseline.value = null;
  });
}
</script>

<template>
  <div class="stack baseline">
    <CalloutBox v-if="!requestId" tone="info">Save the request to use baselines.</CalloutBox>
    <template v-else-if="loading" />
    <CalloutBox v-else-if="!baseline" tone="info">
      No baseline for this request in <strong>{{ envs.selected?.name ?? 'No environment' }}</strong
      >. Send the request against a known-good environment, review the response, then choose
      <strong>Approve as baseline</strong> in the response panel. Baselines are per environment and are only changed
      when you approve them.
    </CalloutBox>
    <template v-else>
      <p class="small muted">
        Approved {{ formatTime(baseline.updatedAt) }} for <strong>{{ envs.selected?.name ?? 'No environment' }}</strong
        >. Compares
        {{
          baseline.mode === 'schema'
            ? 'the structure of the response (field names and types, not values)'
            : baseline.selectedPaths.length
              ? `${baseline.selectedPaths.length} selected path(s)`
              : 'the whole response body'
        }}.
      </p>
      <div v-if="baseline.selectedPaths.length" class="small">
        <span class="muted">Selected paths:</span>
        <code v-for="p in baseline.selectedPaths" :key="p" class="chip">{{ p }}</code>
      </div>
      <TextField
        v-model="ignored"
        label="Ignored paths (one per line) — dynamic values such as timestamps and request IDs"
        multiline
        rows="4"
        :placeholder="IGNORED_PLACEHOLDER"
      />
      <ChangingFieldsDetector
        :request="ws.draft"
        :ignored="lines(ignored)"
        @add="ignored = mergeLines(ignored, $event)"
      />
      <TextField
        v-if="baseline.mode !== 'schema'"
        v-model="tolerance"
        class="tol"
        label="Numeric tolerance (absolute)"
        type="number"
        min="0"
        step="any"
      />
      <div class="row">
        <BaseButton variant="primary" @click="saveRules">Save rules</BaseButton>
        <BaseButton variant="danger" @click="remove">Delete baseline</BaseButton>
      </div>
      <details>
        <summary class="small muted">
          {{ baseline.mode === 'schema' ? 'Approved schema' : 'Approved snapshot' }}
        </summary>
        <pre class="code snap">{{ JSON.stringify(baseline.snapshot, null, 2) }}</pre>
      </details>
    </template>
  </div>
</template>

<style scoped>
.baseline {
  max-width: 720px;
}
.tol :deep(input) {
  width: 140px;
}
.chip {
  display: inline-block;
  margin: 2px 4px 2px 0;
  padding: 0 6px;
  border: 1px solid var(--line);
  border-radius: 4px;
}
.snap {
  max-height: 280px;
  margin-top: 6px;
}
</style>

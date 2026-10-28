<script setup lang="ts">
import { computed, ref, watch } from 'vue';

import { api } from '../services/api';
import { formatTime } from '../services/model';
import { useEnvironments } from '../stores/environments';
import { useUi } from '../stores/ui';
import { useWorkspace } from '../stores/workspace';
import type { Baseline } from '../types';

const ws = useWorkspace();
const envs = useEnvironments();
const ui = useUi();

const baseline = ref<Baseline | null>(null);
const ignored = ref('');
const tolerance = ref(0);
const loading = ref(false);

const requestId = computed(() => ws.draft?.id ?? '');

async function load() {
  baseline.value = null;
  if (!requestId.value) return;
  loading.value = true;
  try {
    baseline.value = await api.getBaseline(requestId.value, envs.selectedId);
    ignored.value = baseline.value?.ignoredPaths.join('\n') ?? '';
    tolerance.value = baseline.value?.tolerance ?? 0;
  } catch (e) {
    ui.error(e);
  } finally {
    loading.value = false;
  }
}
watch([requestId, () => envs.selectedId, () => ws.currentResult?.regression?.baselineId], load, { immediate: true });
defineExpose({ load });

async function saveRules() {
  if (!baseline.value) return;
  try {
    await api.updateBaselineRules(baseline.value.id, ignored.value.split('\n'), tolerance.value || 0);
    ui.toast('Baseline rules saved. They apply from the next run.', 'success');
    await load();
  } catch (e) {
    ui.error(e);
  }
}

async function remove() {
  if (!baseline.value) return;
  const ok = await ui.confirm({
    title: 'Delete baseline?',
    message: 'Future runs will not be compared until you approve a new baseline.',
    confirmLabel: 'Delete',
    danger: true,
  });
  if (!ok) return;
  try {
    await api.deleteBaseline(baseline.value.id);
    baseline.value = null;
  } catch (e) {
    ui.error(e);
  }
}
</script>

<template>
  <div class="stack baseline">
    <div v-if="!requestId" class="callout info">Save the request to use baselines.</div>
    <template v-else-if="loading" />
    <div v-else-if="!baseline" class="callout info">
      No baseline for this request in <strong>{{ envs.selected?.name ?? 'No environment' }}</strong
      >. Send the request against a known-good environment, review the response, then choose
      <strong>Approve as baseline</strong> in the response panel. Baselines are per environment and are only changed
      when you approve them.
    </div>
    <template v-else>
      <p class="small muted">
        Approved {{ formatTime(baseline.updatedAt) }} for <strong>{{ envs.selected?.name ?? 'No environment' }}</strong
        >. Compares
        {{
          baseline.selectedPaths.length
            ? `${baseline.selectedPaths.length} selected path(s)`
            : 'the whole response body'
        }}.
      </p>
      <div v-if="baseline.selectedPaths.length" class="small">
        <span class="muted">Selected paths:</span>
        <code v-for="p in baseline.selectedPaths" :key="p" class="chip">{{ p }}</code>
      </div>
      <label class="field">
        <span>Ignored paths (one per line) — dynamic values such as timestamps and request IDs</span>
        <textarea v-model="ignored" rows="4" placeholder="meta.request_id&#10;items[*].updated_at&#10;**.timestamp" />
      </label>
      <label class="field tol">
        <span>Numeric tolerance (absolute)</span>
        <input v-model.number="tolerance" type="number" min="0" step="any" />
      </label>
      <div class="row">
        <button class="primary" @click="saveRules">Save rules</button>
        <button class="danger" @click="remove">Delete baseline</button>
      </div>
      <details>
        <summary class="small muted">Approved snapshot</summary>
        <pre class="code snap">{{ JSON.stringify(baseline.snapshot, null, 2) }}</pre>
      </details>
    </template>
  </div>
</template>

<style scoped>
.baseline {
  max-width: 720px;
}
.tol input {
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

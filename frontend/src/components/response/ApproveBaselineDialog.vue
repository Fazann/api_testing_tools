<script setup lang="ts">
import { computed, ref } from 'vue';

import { api } from '../services/api';
import { jsonPaths } from '../services/model';
import { useEnvironments } from '../stores/environments';
import { useUi } from '../stores/ui';
import type { RunResult } from '../types';
import ModalFrame from './ModalFrame.vue';

const props = defineProps<{ result: RunResult; requestId: string; existingIgnored?: string[] }>();
const emit = defineEmits<{ close: []; approved: [] }>();
const envs = useEnvironments();
const ui = useUi();

const mode = ref<'all' | 'selected' | 'schema'>('all');
const filter = ref('');
const selected = ref<string[]>([]);
const ignored = ref((props.existingIgnored ?? []).join('\n'));
const tolerance = ref(0);
const busy = ref(false);

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
  const cur = new Set(
    ignored.value
      .split('\n')
      .map((s) => s.trim())
      .filter(Boolean),
  );
  for (const s of suggestions.value) cur.add(s.replace(/\[\d+\]/g, '[*]'));
  ignored.value = [...cur].join('\n');
}

async function approve() {
  if (mode.value === 'selected' && selected.value.length === 0) {
    ui.toast('Select at least one path, or compare the whole response.', 'error');
    return;
  }
  busy.value = true;
  try {
    await api.approveBaseline({
      requestId: props.requestId,
      environmentId: envs.selectedId,
      body: props.result.response?.body ?? '',
      truncated: props.result.response?.truncated ?? false,
      selectedPaths: mode.value === 'selected' ? selected.value : [],
      ignoredPaths: ignored.value.split('\n'),
      tolerance: mode.value === 'schema' ? 0 : tolerance.value || 0,
      mode: mode.value === 'schema' ? 'schema' : 'values',
    });
    ui.toast('Baseline approved', 'success');
    emit('approved');
    emit('close');
  } catch (e) {
    ui.error(e);
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <ModalFrame title="Approve response as baseline" wide @close="emit('close')">
    <p class="callout small">
      A baseline is not automatically correct. Only approve this response after verifying it is right for
      <strong>{{ envs.selected?.name ?? 'No environment' }}</strong
      >. Future runs in this environment will be compared against it.
    </p>
    <div v-if="doc === undefined" class="callout error">
      The response body is not valid JSON, so it cannot be used as a baseline.
    </div>
    <template v-else>
      <div class="row">
        <label class="row"><input v-model="mode" type="radio" value="all" /> Compare the whole response</label>
        <label class="row"
          ><input v-model="mode" type="radio" value="selected" /> Compare selected stable fields only</label
        >
        <label class="row"
          ><input v-model="mode" type="radio" value="schema" /> Compare structure only (field names and types)</label
        >
      </div>
      <p v-if="mode === 'schema'" class="muted small">
        Values are not compared. A run fails when a field appears or disappears, or its type changes (for example a
        number becomes a string, or an object becomes <code>null</code>). Array items are checked against all items seen
        now; fields missing from some items are optional. Fields that are <code>null</code> now, and empty arrays,
        accept any value later.
      </p>
      <div v-if="mode === 'selected'" class="stack">
        <input v-model="filter" placeholder="Filter paths…" aria-label="Filter paths" />
        <div class="paths">
          <label v-for="p in visiblePaths" :key="p" class="row small mono"
            ><input v-model="selected" type="checkbox" :value="p" /> {{ p }}</label
          >
        </div>
        <span class="muted small">{{ selected.length }} selected</span>
      </div>
      <label class="field">
        <span class="row"
          >Ignored paths (one per line)
          <button v-if="suggestions.length" type="button" class="ghost small" @click="addSuggestions">
            Add likely dynamic fields ({{ suggestions.length }})
          </button></span
        >
        <textarea v-model="ignored" rows="4" placeholder="meta.request_id&#10;items[*].updated_at&#10;**.timestamp" />
      </label>
      <label v-if="mode !== 'schema'" class="field tol"
        ><span>Numeric tolerance (absolute)</span><input v-model.number="tolerance" type="number" min="0" step="any"
      /></label>
    </template>
    <template #footer>
      <button @click="emit('close')">Cancel</button>
      <button class="primary" :disabled="busy || doc === undefined" @click="approve">Approve baseline</button>
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
.tol input {
  width: 140px;
}
</style>

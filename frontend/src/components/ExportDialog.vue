<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';

import { api } from '../services/api';
import { useEnvironments } from '../stores/environments';
import { useUi } from '../stores/ui';
import type { Collection, SecretFinding } from '../types';
import ModalFrame from './ModalFrame.vue';

const props = defineProps<{ collection: Collection }>();
const emit = defineEmits<{ close: [] }>();
const envs = useEnvironments();
const ui = useUi();

const envIds = ref<string[]>([]);
const findings = ref<SecretFinding[]>([]);
const includeSecrets = ref(false);
const busy = ref(false);

const hasSecrets = computed(() => findings.value.length > 0);

async function scan() {
  try {
    findings.value = await api.exportPreview(props.collection.id, envIds.value);
  } catch (e) {
    ui.error(e);
  }
}
onMounted(scan);
watch(envIds, scan, { deep: true });

async function doExport() {
  if (includeSecrets.value && hasSecrets.value) {
    const ok = await ui.confirm({
      title: 'Export secrets in plain text?',
      message: 'Anyone with the file will be able to read these values. Do not commit it to source control.',
      details: findings.value.map((f) => `${f.location}: ${f.detail}`),
      confirmLabel: 'Include secrets',
      danger: true,
    });
    if (!ok) return;
  }
  busy.value = true;
  try {
    const path = await api.exportCollection(props.collection.id, envIds.value, includeSecrets.value);
    if (path) {
      ui.toast(`Exported to ${path}`, 'success');
      emit('close');
    }
  } catch (e) {
    ui.error(e);
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <ModalFrame :title="`Export “${collection.name}”`" @close="emit('close')">
    <p class="muted">
      Exports use the documented JSON collection format (version 1) and can be imported on another machine.
    </p>
    <div class="field">
      <span>Include environments</span>
      <label v-for="e in envs.list" :key="e.id" class="row"
        ><input v-model="envIds" type="checkbox" :value="e.id" /> {{ e.name }}</label
      >
      <span v-if="envs.list.length === 0" class="muted small">No environments defined.</span>
    </div>
    <div v-if="hasSecrets" class="callout">
      <strong>This export contains secrets</strong>
      <ul class="small">
        <li v-for="(f, i) in findings" :key="i">{{ f.location }}: {{ f.detail }}</li>
      </ul>
      <label class="row"
        ><input v-model="includeSecrets" type="checkbox" /> Include secret values (otherwise they are left blank)</label
      >
    </div>
    <p v-else class="muted small">
      No secret values detected. Credentials that use <code v-pre>{{ variables }}</code> are exported as references
      only.
    </p>
    <template #footer>
      <button @click="emit('close')">Cancel</button>
      <button class="primary" :disabled="busy" @click="doExport">Export…</button>
    </template>
  </ModalFrame>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';

import BaseButton from '@/components/base/BaseButton.vue';
import CalloutBox from '@/components/base/CalloutBox.vue';
import CheckboxField from '@/components/base/CheckboxField.vue';
import ModalFrame from '@/components/base/ModalFrame.vue';
import { useAsyncAction } from '@/composables/useAsyncAction';
import { api } from '@/services/api';
import { useEnvironments } from '@/stores/environments';
import { useUi } from '@/stores/ui';
import type { Collection, SecretFinding } from '@/types';

const props = defineProps<{ collection: Collection }>();
const emit = defineEmits<{ close: [] }>();
const envs = useEnvironments();
const ui = useUi();

const envIds = ref<string[]>([]);
const findings = ref<SecretFinding[]>([]);
const includeSecrets = ref(false);
const includeBaselines = ref(false);
const { busy, run } = useAsyncAction();

const hasSecrets = computed(() => findings.value.length > 0);

async function scan() {
  findings.value = (await run(() => api.exportPreview(props.collection.id, envIds.value))) ?? findings.value;
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
  const path = await run(() =>
    api.exportCollection(props.collection.id, envIds.value, includeSecrets.value, includeBaselines.value),
  );
  if (path) {
    ui.toast(`Exported to ${path}`, 'success');
    emit('close');
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
      <CheckboxField v-for="e in envs.list" :key="e.id" v-model="envIds" :value="e.id" :label="e.name" />
      <span v-if="envs.list.length === 0" class="muted small">No environments defined.</span>
    </div>
    <CheckboxField v-model="includeBaselines">
      <strong>Include baselines</strong> — approved baselines of every environment, so the regression check also runs on
      another machine or in CI with <code>api-test-kits-cli</code>. Baselines hold response bodies (already redacted
      when approved).
    </CheckboxField>
    <CalloutBox v-if="hasSecrets">
      <strong>This export contains secrets</strong>
      <ul class="small">
        <li v-for="(f, i) in findings" :key="i">{{ f.location }}: {{ f.detail }}</li>
      </ul>
      <CheckboxField v-model="includeSecrets" label="Include secret values (otherwise they are left blank)" />
    </CalloutBox>
    <p v-else class="muted small">
      No secret values detected. Credentials that use <code v-pre>{{ variables }}</code> are exported as references
      only.
    </p>
    <template #footer>
      <BaseButton @click="emit('close')">Cancel</BaseButton>
      <BaseButton variant="primary" :disabled="busy" @click="doExport">Export…</BaseButton>
    </template>
  </ModalFrame>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';

import BaseButton from '@/components/base/BaseButton.vue';
import BaseInput from '@/components/base/BaseInput.vue';
import CheckboxField from '@/components/base/CheckboxField.vue';
import FormField from '@/components/base/FormField.vue';
import SelectField from '@/components/base/SelectField.vue';
import TextField from '@/components/base/TextField.vue';
import { useAsyncAction } from '@/composables/useAsyncAction';
import { api } from '@/services/api';
import { useEnvironments } from '@/stores/environments';
import { useUi } from '@/stores/ui';
import { useUpdates } from '@/stores/updates';
import { useWorkspace } from '@/stores/workspace';
import type { AppInfo, DataInfo, Settings } from '@/types';
import { formatBytes } from '@/utils/format';
import { clone } from '@/utils/object';

const ui = useUi();
const { run } = useAsyncAction();
const ws = useWorkspace();
const envs = useEnvironments();
const updates = useUpdates();

const form = ref<Settings | null>(null);
const snapshot = ref('');
const info = ref<DataInfo | null>(null);
const app = ref<AppInfo | null>(null);
const dirty = computed(() => form.value !== null && JSON.stringify(form.value) !== snapshot.value);

// Sizes are edited in KB/MB for readability.
const maxResponseMb = computed({
  get: () => (form.value ? form.value.maxResponseBytes / (1 << 20) : 0),
  set: (v: number) => form.value && (form.value.maxResponseBytes = Math.round(v * (1 << 20))),
});
const historyBodyKb = computed({
  get: () => (form.value ? form.value.historyBodyLimitBytes / 1024 : 0),
  set: (v: number) => form.value && (form.value.historyBodyLimitBytes = Math.round(v * 1024)),
});

async function load() {
  await run(async () => {
    const [s, d, a] = await Promise.all([api.getSettings(), api.dataInfo(), api.appInfo()]);
    form.value = s;
    snapshot.value = JSON.stringify(s);
    info.value = d;
    app.value = a;
  });
}
onMounted(load);

async function save() {
  const settings = form.value;
  if (!settings) return;
  await run(async () => {
    await api.saveSettings(clone(settings));
    snapshot.value = JSON.stringify(settings);
    ui.toast('Settings saved', 'success');
  });
}

async function chooseCA() {
  if (!form.value) return;
  const path = await api.chooseCACertFile().catch(ui.error);
  if (path) form.value.caCertFile = path;
}

async function compact() {
  await run(async () => {
    await api.compactDatabase();
    info.value = await api.dataInfo();
    ui.toast('Database compacted', 'success');
  });
}

async function deleteAll() {
  const ok = await ui.confirm({
    title: 'Delete all local data?',
    message:
      'Every collection, request, environment, baseline and run will be permanently deleted from this computer. ' +
      'Export anything you want to keep first.',
    confirmLabel: 'Delete everything',
    danger: true,
  });
  if (!ok) return;
  await run(async () => {
    await api.deleteAllData();
    ws.closeAll();
    await Promise.all([ws.load(), envs.load(), load()]);
    ui.toast('All local data deleted', 'success');
  });
}

const PROXY_MODES: { value: Settings['proxyMode']; label: string }[] = [
  { value: 'system', label: 'Use system proxy variables (HTTP_PROXY, HTTPS_PROXY, NO_PROXY)' },
  { value: 'none', label: 'No proxy — connect directly' },
  { value: 'custom', label: 'Custom proxy URL' },
];
</script>

<template>
  <div class="panel settings">
    <template v-if="form">
      <div class="row head">
        <h2 class="grow">Settings</h2>
        <BaseButton variant="primary" :disabled="!dirty" @click="save">Save settings</BaseButton>
      </div>

      <h3 class="section-title">Requests</h3>
      <TextField
        v-model="maxResponseMb"
        label="Response size limit (MB) — larger bodies are truncated"
        type="number"
        min="0.001"
        max="512"
        step="1"
      />
      <TextField
        v-model="form.concurrency"
        label="Parallel requests in collection runs (1 = strictly in order)"
        type="number"
        min="1"
        max="8"
      />

      <h3 class="section-title">Network &amp; TLS</h3>
      <SelectField v-model="form.proxyMode" label="Proxy" :options="PROXY_MODES" />
      <TextField
        v-if="form.proxyMode === 'custom'"
        v-model="form.proxyUrl"
        label="Proxy URL"
        input-class="mono"
        placeholder="http://proxy.local:3128"
      />
      <FormField label="Extra trusted CA certificates (PEM file, optional)">
        <span class="row">
          <BaseInput v-model="form.caCertFile" class="mono grow" placeholder="/path/to/ca.pem" />
          <BaseButton @click="chooseCA">Browse…</BaseButton>
        </span>
      </FormField>
      <CheckboxField v-model="form.insecureSkipVerify" class="check">
        <strong class="fail">Disable TLS certificate verification</strong> — only for local test servers with
        self-signed certificates. Reports record when this is off.
      </CheckboxField>
      <CheckboxField v-model="form.allowSensitiveDestinations" class="check">
        Allow requests to link-local and cloud metadata addresses (169.254.0.0/16, metadata.google.internal). Blocked by
        default to protect against untrusted imported collections.
      </CheckboxField>

      <h3 class="section-title">History</h3>
      <TextField
        v-model="form.historyRetentionRuns"
        label="Runs to keep (older runs are deleted automatically)"
        type="number"
        min="1"
        max="100000"
      />
      <TextField
        v-model="historyBodyKb"
        label="Response body stored per result (KB)"
        type="number"
        min="0"
        max="10240"
      />

      <h3 class="section-title">Updates</h3>
      <CheckboxField v-model="form.checkForUpdates" class="check">
        Check for a new version when the app starts. Updates install in place — no uninstall needed, and your data is
        kept.
      </CheckboxField>
      <div class="row small">
        <BaseButton :disabled="updates.checking || updates.installing" @click="updates.check()">
          {{ updates.checking ? 'Checking…' : 'Check for updates now' }}
        </BaseButton>
        <span v-if="updates.error" class="fail">{{ updates.error }}</span>
        <span v-else-if="updates.info && !updates.info.enabled" class="muted">
          This build has no update feed (development build).
        </span>
        <span v-else-if="updates.info?.available">
          Version {{ updates.info.latest }} is available.
          <BaseButton small variant="primary" :disabled="updates.installing" @click="updates.install()">
            {{ updates.info.method === 'manual' ? 'Download…' : 'Update and restart' }}
          </BaseButton>
        </span>
        <span v-else-if="updates.info" class="muted">You have the latest version ({{ updates.info.current }}).</span>
      </div>

      <h3 class="section-title">Local data</h3>
      <div v-if="info" class="stack small">
        <div>
          Database: <code>{{ info.path }}</code> ({{ formatBytes(info.sizeBytes) }})
        </div>
        <div class="muted">
          {{ info.collections }} collections · {{ info.requests }} requests · {{ info.environments }} environments ·
          {{ info.baselines }} baselines · {{ info.runs }} runs
        </div>
        <div class="muted">
          Back up your data by copying this file while the app is closed, or by exporting collections.
        </div>
        <div class="row">
          <BaseButton @click="compact">Compact database</BaseButton>
          <BaseButton variant="danger" @click="deleteAll">Delete all local data…</BaseButton>
        </div>
      </div>
      <p v-if="app" class="muted small about">
        {{ app.name }} {{ app.version }} · Works offline. No account, no telemetry — the only automatic network call is
        the optional update check.
      </p>
    </template>
  </div>
</template>

<style scoped>
.settings {
  overflow: auto;
  flex: 1;
  max-width: 760px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.head h2 {
  margin: 0;
  font-size: 17px;
}
:deep(.field input[type='number']) {
  width: 160px;
}
.about {
  margin-top: 16px;
}
</style>

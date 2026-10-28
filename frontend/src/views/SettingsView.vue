<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';

import { api } from '../services/api';
import { clone, formatBytes } from '../services/model';
import { useEnvironments } from '../stores/environments';
import { useUi } from '../stores/ui';
import { useWorkspace } from '../stores/workspace';
import type { AppInfo, DataInfo, Settings } from '../types';

const ui = useUi();
const ws = useWorkspace();
const envs = useEnvironments();

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
  try {
    const [s, d, a] = await Promise.all([api.getSettings(), api.dataInfo(), api.appInfo()]);
    form.value = s;
    snapshot.value = JSON.stringify(s);
    info.value = d;
    app.value = a;
  } catch (e) {
    ui.error(e);
  }
}
onMounted(load);

async function save() {
  if (!form.value) return;
  try {
    await api.saveSettings(clone(form.value));
    snapshot.value = JSON.stringify(form.value);
    ui.toast('Settings saved', 'success');
  } catch (e) {
    ui.error(e);
  }
}

async function chooseCA() {
  if (!form.value) return;
  const path = await api.chooseCACertFile().catch(ui.error);
  if (path) form.value.caCertFile = path;
}

async function compact() {
  try {
    await api.compactDatabase();
    info.value = await api.dataInfo();
    ui.toast('Database compacted', 'success');
  } catch (e) {
    ui.error(e);
  }
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
  try {
    await api.deleteAllData();
    ws.closeDraft();
    await Promise.all([ws.load(), envs.load(), load()]);
    ui.toast('All local data deleted', 'success');
  } catch (e) {
    ui.error(e);
  }
}
</script>

<template>
  <div class="panel settings">
    <template v-if="form">
      <div class="row head">
        <h2 class="grow">Settings</h2>
        <button class="primary" :disabled="!dirty" @click="save">Save settings</button>
      </div>

      <h3 class="section-title">Requests</h3>
      <label class="field">
        <span>Response size limit (MB) — larger bodies are truncated</span>
        <input v-model.number="maxResponseMb" type="number" min="0.001" max="512" step="1" />
      </label>
      <label class="field">
        <span>Parallel requests in collection runs (1 = strictly in order)</span>
        <input v-model.number="form.concurrency" type="number" min="1" max="8" />
      </label>

      <h3 class="section-title">Network &amp; TLS</h3>
      <label class="field">
        <span>Proxy</span>
        <select v-model="form.proxyMode">
          <option value="system">Use system proxy variables (HTTP_PROXY, HTTPS_PROXY, NO_PROXY)</option>
          <option value="none">No proxy — connect directly</option>
          <option value="custom">Custom proxy URL</option>
        </select>
      </label>
      <label v-if="form.proxyMode === 'custom'" class="field">
        <span>Proxy URL</span>
        <input v-model="form.proxyUrl" class="mono" placeholder="http://proxy.local:3128" />
      </label>
      <label class="field">
        <span>Extra trusted CA certificates (PEM file, optional)</span>
        <span class="row">
          <input v-model="form.caCertFile" class="mono grow" placeholder="/path/to/ca.pem" />
          <button @click="chooseCA">Browse…</button>
        </span>
      </label>
      <label class="row check">
        <input v-model="form.insecureSkipVerify" type="checkbox" />
        <span>
          <strong class="fail">Disable TLS certificate verification</strong> — only for local test servers with
          self-signed certificates. Reports record when this is off.
        </span>
      </label>
      <label class="row check">
        <input v-model="form.allowSensitiveDestinations" type="checkbox" />
        <span>
          Allow requests to link-local and cloud metadata addresses (169.254.0.0/16, metadata.google.internal). Blocked
          by default to protect against untrusted imported collections.
        </span>
      </label>

      <h3 class="section-title">History</h3>
      <label class="field">
        <span>Runs to keep (older runs are deleted automatically)</span>
        <input v-model.number="form.historyRetentionRuns" type="number" min="1" max="100000" />
      </label>
      <label class="field">
        <span>Response body stored per result (KB)</span>
        <input v-model.number="historyBodyKb" type="number" min="0" max="10240" />
      </label>

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
          <button @click="compact">Compact database</button>
          <button class="danger" @click="deleteAll">Delete all local data…</button>
        </div>
      </div>
      <p v-if="app" class="muted small about">
        {{ app.name }} {{ app.version }} · Works offline. No account, no telemetry.
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
.field input[type='number'] {
  width: 160px;
}
.check {
  align-items: flex-start;
}
.about {
  margin-top: 16px;
}
</style>

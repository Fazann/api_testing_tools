<script setup lang="ts">
import { computed, ref } from 'vue';

import { isDestructive } from '../services/model';
import { useEnvironments } from '../stores/environments';
import { useRunner } from '../stores/runner';
import { useUi } from '../stores/ui';
import { useWorkspace } from '../stores/workspace';
import { METHODS, type Collection } from '../types';
import AssertionEditor from './AssertionEditor.vue';
import AuthEditor from './AuthEditor.vue';
import BaselinePanel from './BaselinePanel.vue';
import CollectionSettingsDialog from './CollectionSettingsDialog.vue';
import KeyValueTable from './KeyValueTable.vue';
import VariableInput from './VariableInput.vue';

const ws = useWorkspace();
const runner = useRunner();
const envs = useEnvironments();
const ui = useUi();

type Tab = 'params' | 'headers' | 'body' | 'auth' | 'tests' | 'baseline' | 'options';
const tab = ref<Tab>('tests');
const editingCollection = ref<Collection | null>(null);

const req = computed(() => ws.draft!);
const count = (rows: { key: string; enabled: boolean }[]) => rows.filter((r) => r.enabled && r.key).length;
const bodyAllowed = computed(() => req.value.method !== 'GET' && req.value.method !== 'HEAD');
const jsonError = computed(() => {
  if (req.value.bodyType !== 'json' || !req.value.body.trim()) return '';
  // Variables may make the template invalid JSON until resolved; only check without them.
  if (req.value.body.includes('{{')) return '';
  try {
    JSON.parse(req.value.body);
    return '';
  } catch (e) {
    return (e as Error).message;
  }
});

async function save() {
  try {
    await ws.saveDraft();
    ui.toast('Saved', 'success');
  } catch (e) {
    ui.error(e);
  }
}

function formatBody() {
  try {
    req.value.body = JSON.stringify(JSON.parse(req.value.body), null, 2);
  } catch {
    /* leave invalid JSON untouched */
  }
}

const tabs = computed<{ id: Tab; label: string; count?: number }[]>(() => [
  { id: 'params', label: 'Params', count: count(req.value.query) },
  { id: 'headers', label: 'Headers', count: count(req.value.headers) },
  { id: 'body', label: 'Body' },
  { id: 'auth', label: req.value.auth.type === 'inherit' ? 'Auth (inherited)' : 'Auth' },
  { id: 'tests', label: 'Tests', count: req.value.assertions.filter((a) => a.enabled).length },
  { id: 'baseline', label: 'Baseline' },
  { id: 'options', label: 'Options' },
]);
</script>

<template>
  <section class="editor">
    <div class="row title">
      <input v-model="req.name" class="name grow" aria-label="Request name" maxlength="200" />
      <span v-if="ws.dirty" class="muted small">Unsaved</span>
      <span
        v-if="isDestructive(req.method)"
        class="small"
        :class="envs.selected?.protected ? 'fail' : 'warn'"
        :title="envs.selected?.protected ? 'Requires confirmation in protected environments' : ''"
      >
        May modify data
      </span>
    </div>
    <form class="row urlbar" @submit.prevent="runner.send()">
      <select v-model="req.method" class="method" :class="req.method" aria-label="HTTP method">
        <option v-for="m in METHODS" :key="m" :value="m">{{ m }}</option>
      </select>
      <VariableInput v-model="req.url" class="grow" placeholder="{{base_url}}/path" aria-label="URL" />
      <button v-if="runner.sendingOp" type="button" class="danger" @click="runner.cancelSend()">Cancel</button>
      <button v-else type="submit" class="primary" title="Send (Ctrl+Enter)">Send</button>
      <button type="button" :disabled="!ws.dirty" title="Save (Ctrl+S)" @click="save">Save</button>
    </form>
    <nav class="tabs" role="tablist">
      <button
        v-for="t in tabs"
        :key="t.id"
        role="tab"
        :aria-selected="tab === t.id"
        :class="{ active: tab === t.id }"
        @click="tab = t.id"
      >
        {{ t.label }}<span v-if="t.count" class="count">{{ t.count }}</span>
      </button>
    </nav>
    <div class="tab-body">
      <template v-if="tab === 'params'">
        <KeyValueTable :rows="req.query" key-placeholder="parameter" />
        <p class="muted small">
          Parameters are URL-encoded and appended to the URL. Values support <code v-pre>{{ variables }}</code
          >.
        </p>
      </template>
      <KeyValueTable v-else-if="tab === 'headers'" :rows="req.headers" key-placeholder="Header-Name" />
      <div v-else-if="tab === 'body'" class="stack">
        <div class="row">
          <label v-for="t in ['none', 'json', 'text']" :key="t" class="row small"
            ><input v-model="req.bodyType" type="radio" :value="t" />
            {{ t === 'none' ? 'No body' : t.toUpperCase() }}</label
          >
          <span class="spacer" />
          <button v-if="req.bodyType === 'json'" class="ghost small" @click="formatBody">Format JSON</button>
        </div>
        <p v-if="!bodyAllowed && req.bodyType !== 'none'" class="callout small">
          {{ req.method }} requests are sent without a body.
        </p>
        <textarea
          v-if="req.bodyType !== 'none'"
          v-model="req.body"
          rows="12"
          spellcheck="false"
          aria-label="Request body"
          :placeholder="req.bodyType === 'json' ? '{&quot;amount&quot;: 10}' : ''"
        />
        <p v-if="jsonError" class="fail small">Invalid JSON: {{ jsonError }}</p>
      </div>
      <AuthEditor
        v-else-if="tab === 'auth'"
        :auth="req.auth"
        :inherit-from="ws.currentCollection"
        @edit-collection="editingCollection = ws.currentCollection"
      />
      <AssertionEditor v-else-if="tab === 'tests'" :assertions="req.assertions" />
      <BaselinePanel v-else-if="tab === 'baseline'" />
      <div v-else-if="tab === 'options'" class="stack options">
        <label class="field"
          ><span>Timeout (ms)</span><input v-model.number="req.timeoutMs" type="number" min="1" max="600000"
        /></label>
        <label class="row"
          ><input v-model="req.followRedirects" type="checkbox" /> Follow redirects (credentials are never forwarded to
          a different origin)</label
        >
      </div>
    </div>
    <CollectionSettingsDialog
      v-if="editingCollection"
      :collection="editingCollection"
      @close="editingCollection = null"
    />
  </section>
</template>

<style scoped>
.editor {
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.title {
  padding: 10px 16px 0;
}
.name {
  font-size: 15px;
  font-weight: 600;
  border-color: transparent;
  background: transparent;
}
.name:hover,
.name:focus {
  border-color: var(--line);
}
.urlbar {
  padding: 8px 16px;
}
.urlbar .method {
  width: 105px;
  font-weight: 700;
  font-family: var(--mono);
}
.tabs {
  padding: 0 12px;
}
.tab-body {
  padding: 12px 16px;
  overflow: auto;
  flex: 1;
  min-height: 0;
}
.options {
  max-width: 520px;
}
.options input[type='number'] {
  width: 160px;
}
</style>

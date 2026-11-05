<script setup lang="ts">
import { computed, ref } from 'vue';

import { looksLikeCurl, parseCurl } from '../services/curl';
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
import CurlExportDialog from './CurlExportDialog.vue';
import KeyValueTable from './KeyValueTable.vue';
import RaceTestPanel from './RaceTestPanel.vue';
import ScriptEditor from './ScriptEditor.vue';
import VariableInput from './VariableInput.vue';

const ws = useWorkspace();
const runner = useRunner();
const envs = useEnvironments();
const ui = useUi();

type Tab = 'params' | 'headers' | 'body' | 'auth' | 'scripts' | 'tests' | 'baseline' | 'race' | 'options';
const tab = ref<Tab>('tests');
const editingCollection = ref<Collection | null>(null);
const copyingCurl = ref(false);

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

// Pasting a cURL command into the URL bar fills in the whole request, like "New request from cURL".
function onUrlPaste(e: ClipboardEvent) {
  const pasted = e.clipboardData?.getData('text/plain') ?? '';
  if (!looksLikeCurl(pasted)) return;
  e.preventDefault();
  try {
    const { request, warnings } = parseCurl(pasted);
    const r = req.value;
    if (r.name === 'New request') r.name = request.name;
    Object.assign(r, {
      method: request.method,
      url: request.url,
      // The URL already carries the query string.
      query: [],
      headers: request.headers,
      body: request.body,
      bodyType: request.bodyType,
      auth: request.auth,
      timeoutMs: request.timeoutMs,
    });
    ui.toast(
      'Filled in the request from cURL. Save to keep it.' + (warnings.length ? ` Note: ${warnings.join('; ')}.` : ''),
      warnings.length ? 'info' : 'success',
    );
  } catch (err) {
    ui.toast(`Could not read the cURL command: ${(err as Error).message}`, 'error');
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
  {
    id: 'scripts',
    label: 'Scripts',
    count: [req.value.preRequestScript, req.value.postResponseScript].filter((s) => s?.trim()).length,
  },
  { id: 'tests', label: 'Tests', count: req.value.assertions.filter((a) => a.enabled).length },
  { id: 'baseline', label: 'Baseline' },
  { id: 'race', label: 'Race test' },
  { id: 'options', label: 'Options' },
]);
</script>

<template>
  <section class="editor">
    <div class="row title">
      <input v-model="req.name" class="name grow" aria-label="Request name" maxlength="200" />
      <span v-if="ws.dirty" class="muted small">Unsaved</span>
      <button class="ghost small" title="Copy this request as a cURL command" @click="copyingCurl = true">
        Copy as cURL
      </button>
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
      <VariableInput
        v-model="req.url"
        class="grow"
        placeholder="{{base_url}}/path or paste a cURL command"
        aria-label="URL"
        @paste="onUrlPaste"
      />
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
      <ScriptEditor
        v-else-if="tab === 'scripts'"
        v-model:pre="req.preRequestScript"
        v-model:post="req.postResponseScript"
        scope="request"
      />
      <AssertionEditor v-else-if="tab === 'tests'" :assertions="req.assertions" />
      <BaselinePanel v-else-if="tab === 'baseline'" />
      <RaceTestPanel v-else-if="tab === 'race'" />
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
    <CurlExportDialog v-if="copyingCurl" :request="req" @close="copyingCurl = false" />
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

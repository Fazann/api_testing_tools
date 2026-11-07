<script setup lang="ts">
import { computed, ref } from 'vue';

import BaseButton from '@/components/base/BaseButton.vue';
import BaseInput from '@/components/base/BaseInput.vue';
import CalloutBox from '@/components/base/CalloutBox.vue';
import CheckboxField from '@/components/base/CheckboxField.vue';
import CodeEditor from '@/components/base/CodeEditor.vue';
import RadioGroup from '@/components/base/RadioGroup.vue';
import SelectField from '@/components/base/SelectField.vue';
import TabBar, { type Tab as TabItem } from '@/components/base/TabBar.vue';
import TextField from '@/components/base/TextField.vue';
import CodeExportDialog from '@/components/collection/CodeExportDialog.vue';
import CollectionSettingsDialog from '@/components/collection/CollectionSettingsDialog.vue';
import PerformancePanel from '@/components/performance/PerformancePanel.vue';
import BaselinePanel from '@/components/response/BaselinePanel.vue';
import RaceTestPanel from '@/components/security/RaceTestPanel.vue';
import SocketMessagesEditor from '@/components/socket/SocketMessagesEditor.vue';
import SocketSettings from '@/components/socket/SocketSettings.vue';
import SocketStepsEditor from '@/components/socket/SocketStepsEditor.vue';
import VariableInput from '@/components/variables/VariableInput.vue';
import { useAsyncAction } from '@/composables/useAsyncAction';
import { looksLikeCurl, parseCurl } from '@/services/curl';
import { isSocket, sendsMessages, setRequestKind } from '@/services/socket';
import { useEnvironments } from '@/stores/environments';
import { useRunner } from '@/stores/runner';
import { useUi } from '@/stores/ui';
import { useWorkspace } from '@/stores/workspace';
import { METHODS, REQUEST_KINDS, type Collection, type Request, type RequestKind } from '@/types';
import { isDestructive } from '@/utils/http';

import AssertionEditor from './AssertionEditor.vue';
import AuthEditor from './AuthEditor.vue';
import KeyValueTable from './KeyValueTable.vue';
import ScriptEditor from './ScriptEditor.vue';

const ws = useWorkspace();
const runner = useRunner();
const envs = useEnvironments();
const ui = useUi();

type Tab =
  | 'params'
  | 'headers'
  | 'body'
  | 'auth'
  | 'scripts'
  | 'tests'
  | 'baseline'
  | 'performance'
  | 'race'
  | 'messages'
  | 'options';
const tab = ref<Tab>('tests');
const editingCollection = ref<Collection | null>(null);
const copyingCurl = ref(false);

const req = computed(() => ws.draft!);
const socket = computed(() => isSocket(req.value.kind));
const mayModify = computed(() => (socket.value ? sendsMessages(req.value.socket) : isDestructive(req.value.method)));
function changeKind(kind: RequestKind | undefined) {
  if (!kind) return;
  setRequestKind(req.value, kind);
  const hidden: Tab[] = socket.value ? ['body', 'baseline', 'performance', 'race'] : ['messages'];
  if (hidden.includes(tab.value)) tab.value = 'tests';
}
const count = (rows: { key: string; enabled: boolean }[]) => rows.filter((r) => r.enabled && r.key).length || '';
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

const { run } = useAsyncAction();
const save = () =>
  run(async () => {
    await ws.saveDraft();
    ui.toast('Saved', 'success');
  });

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

const METHOD_OPTIONS = METHODS.map((m) => ({ value: m, label: m }));
const BODY_TYPES: { value: Request['bodyType']; label: string }[] = [
  { value: 'none', label: 'No body' },
  { value: 'json', label: 'JSON' },
  { value: 'text', label: 'TEXT' },
];

// Tabs show a count only when there is something to count. Socket requests have no body, baseline,
// response-time check or race test, but saved messages and test steps.
const tabs = computed<TabItem<Tab>[]>(() =>
  socket.value
    ? [
        { id: 'params', label: 'Params', count: count(req.value.query) },
        { id: 'headers', label: 'Headers', count: count(req.value.headers) },
        { id: 'auth', label: req.value.auth.type === 'inherit' ? 'Auth (inherited)' : 'Auth' },
        { id: 'messages', label: 'Messages', count: req.value.socket?.messages.length || '' },
        { id: 'scripts', label: 'Scripts', count: req.value.preRequestScript?.trim() ? 1 : '' },
        { id: 'tests', label: 'Tests', count: req.value.socket?.steps.filter((s) => s.enabled).length || '' },
        { id: 'options', label: 'Options' },
      ]
    : httpTabs.value,
);
const httpTabs = computed<TabItem<Tab>[]>(() => [
  { id: 'params', label: 'Params', count: count(req.value.query) },
  { id: 'headers', label: 'Headers', count: count(req.value.headers) },
  { id: 'body', label: 'Body' },
  { id: 'auth', label: req.value.auth.type === 'inherit' ? 'Auth (inherited)' : 'Auth' },
  {
    id: 'scripts',
    label: 'Scripts',
    count: [req.value.preRequestScript, req.value.postResponseScript].filter((s) => s?.trim()).length || '',
  },
  { id: 'tests', label: 'Tests', count: req.value.assertions.filter((a) => a.enabled).length || '' },
  { id: 'baseline', label: 'Baseline' },
  { id: 'performance', label: 'Performance' },
  { id: 'race', label: 'Race test' },
  { id: 'options', label: 'Options' },
]);
</script>

<template>
  <section class="editor">
    <div class="row title">
      <BaseInput v-model="req.name" class="name grow" aria-label="Request name" maxlength="200" />
      <span v-if="ws.dirty" class="muted small">Unsaved</span>
      <BaseButton
        v-if="!socket"
        variant="ghost"
        small
        title="Copy this request as cURL, fetch, Python, Go, Java and other code"
        @click="copyingCurl = true"
      >
        Copy as code
      </BaseButton>
      <span
        v-if="mayModify"
        class="small"
        :class="envs.selected?.protected ? 'fail' : 'warn'"
        :title="envs.selected?.protected ? 'Requires confirmation in protected environments' : ''"
      >
        May modify data
      </span>
    </div>
    <form class="row urlbar" @submit.prevent="runner.send()">
      <SelectField
        :model-value="req.kind ?? 'http'"
        class="kind"
        :options="REQUEST_KINDS"
        aria-label="Request type"
        @update:model-value="changeKind"
      />
      <SelectField
        v-if="!socket"
        v-model="req.method"
        class="method"
        :class="req.method"
        :options="METHOD_OPTIONS"
        aria-label="HTTP method"
      />
      <VariableInput
        v-model="req.url"
        class="grow"
        :placeholder="
          socket
            ? req.kind === 'socketio'
              ? 'https://{{host}}/namespace'
              : 'wss://{{host}}/path'
            : '{{base_url}}/path or paste a cURL command'
        "
        aria-label="URL"
        @paste="!socket && onUrlPaste($event)"
      />
      <BaseButton
        v-if="runner.sendingOp && runner.sendingRequestId === req.id"
        variant="danger"
        @click="runner.cancelSend()"
        >Cancel</BaseButton
      >
      <BaseButton
        v-else
        type="submit"
        variant="primary"
        :disabled="!!runner.sendingOp"
        :title="
          runner.sendingOp
            ? 'Another tab is sending a request; wait for it or cancel it there'
            : socket
              ? 'Connect, run the test steps and record the result (Ctrl+Enter)'
              : 'Send (Ctrl+Enter)'
        "
        >{{ socket ? 'Run test' : 'Send' }}</BaseButton
      >
      <BaseButton :disabled="!ws.dirty" title="Save (Ctrl+S)" @click="save">Save</BaseButton>
    </form>
    <TabBar v-model="tab" :tabs="tabs" />
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
          <RadioGroup v-model="req.bodyType" class="small" name="body-type" label="Body type" :options="BODY_TYPES" />
          <span class="spacer" />
          <BaseButton v-if="req.bodyType === 'json'" variant="ghost" small @click="formatBody">Format JSON</BaseButton>
        </div>
        <CalloutBox v-if="!bodyAllowed && req.bodyType !== 'none'" small>
          {{ req.method }} requests are sent without a body.
        </CalloutBox>
        <CodeEditor
          v-if="req.bodyType === 'json'"
          v-model="req.body"
          language="json"
          theme="app"
          label="Request body"
          :min-lines="12"
          placeholder='{"amount": 10}'
        />
        <BaseInput
          v-else-if="req.bodyType !== 'none'"
          multiline
          v-model="req.body"
          rows="12"
          spellcheck="false"
          aria-label="Request body"
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
        :pre-only="socket"
      />
      <SocketStepsEditor v-else-if="tab === 'tests' && socket && req.socket" :socket="req.socket" :kind="req.kind" />
      <AssertionEditor v-else-if="tab === 'tests'" :assertions="req.assertions" />
      <SocketMessagesEditor v-else-if="tab === 'messages' && req.socket" :socket="req.socket" :kind="req.kind" />
      <SocketSettings v-else-if="tab === 'options' && socket" :request="req" />
      <BaselinePanel v-else-if="tab === 'baseline'" />
      <PerformancePanel v-else-if="tab === 'performance'" />
      <RaceTestPanel v-else-if="tab === 'race'" />
      <div v-else-if="tab === 'options'" class="stack options">
        <TextField v-model="req.timeoutMs" label="Timeout (ms)" type="number" min="1" max="600000" />
        <CheckboxField
          v-model="req.followRedirects"
          label="Follow redirects (credentials are never forwarded to a different origin)"
        />
      </div>
    </div>
    <CodeExportDialog v-if="copyingCurl" :request="req" @close="copyingCurl = false" />
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
.urlbar .kind {
  width: 120px;
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

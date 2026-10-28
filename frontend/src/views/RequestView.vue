<script setup lang="ts">
import RequestEditor from '../components/RequestEditor.vue';
import ResponseViewer from '../components/ResponseViewer.vue';
import { useRunner } from '../stores/runner';
import { useUi } from '../stores/ui';
import { useWorkspace } from '../stores/workspace';

const ws = useWorkspace();
const ui = useUi();
const runner = useRunner();

async function quickStart() {
  const name = await ui.prompt({ title: 'New collection', value: 'My API tests', confirmLabel: 'Create' });
  if (!name) return;
  try {
    const c = await ws.createCollection(name);
    await ws.createRequest(c.id);
  } catch (e) {
    ui.error(e);
  }
}
</script>

<template>
  <div v-if="!ws.draft" class="empty welcome">
    <h2>{{ ws.collections.length ? 'Select a request' : 'Welcome to API Regression Guard' }}</h2>
    <p v-if="!ws.collections.length">
      Test HTTP APIs locally, check business rules — not just status codes — and catch regressions by comparing
      responses with an approved baseline. Everything is stored on this computer. No account needed.
    </p>
    <p v-else>Choose a request in the sidebar, or create a new one from a collection's ⋯ menu.</p>
    <ol v-if="!ws.collections.length" class="steps">
      <li>Create an <strong>environment</strong> with <code>base_url</code> and a secret <code>token</code>.</li>
      <li>Create a <strong>collection</strong> and add requests with tests.</li>
      <li>Send against a known-good environment and <strong>approve a baseline</strong>.</li>
      <li>After code or config changes, <strong>run the collection</strong> and review differences.</li>
    </ol>
    <div class="row">
      <button class="primary" @click="quickStart">Create collection</button>
      <button @click="ui.view = 'environments'">Set up environments</button>
    </div>
  </div>
  <div v-else class="split">
    <RequestEditor class="top" />
    <div class="bottom">
      <div v-if="runner.sendingOp" class="empty">
        <p>Sending… <button class="small" @click="runner.cancelSend()">Cancel (Esc)</button></p>
      </div>
      <ResponseViewer v-else-if="ws.currentResult" :result="ws.currentResult" :request-id="ws.draft.id" allow-approve />
      <div v-else class="empty small">
        <p>Press <strong>Send</strong> (Ctrl+Enter) to run this request and its tests.</p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.welcome {
  flex: 1;
}
.steps {
  text-align: left;
  max-width: 460px;
  margin: 0;
  padding-left: 20px;
}
.steps li {
  margin: 4px 0;
}
.split {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}
.top {
  flex: 1 1 50%;
  min-height: 200px;
  border-bottom: 1px solid var(--line);
}
.bottom {
  flex: 1 1 50%;
  min-height: 160px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
</style>

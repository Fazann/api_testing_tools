<script setup lang="ts">
import { computed, ref, watch } from 'vue';

import BaseButton from '@/components/base/BaseButton.vue';
import EmptyState from '@/components/base/EmptyState.vue';
import TabBar from '@/components/base/TabBar.vue';
import RequestEditor from '@/components/request/RequestEditor.vue';
import RequestTabs from '@/components/request/RequestTabs.vue';
import ResponseViewer from '@/components/response/ResponseViewer.vue';
import SocketConsole from '@/components/socket/SocketConsole.vue';
import { useAsyncAction } from '@/composables/useAsyncAction';
import { isSocket } from '@/services/socket';
import { useRunner } from '@/stores/runner';
import { useUi } from '@/stores/ui';
import { useWorkspace } from '@/stores/workspace';

const ws = useWorkspace();
const ui = useUi();
const { run } = useAsyncAction();
const runner = useRunner();

// Socket requests show their live console and the result of the last test run in tabs.
const socket = computed(() => !!ws.draft && isSocket(ws.draft.kind));
const socketTab = ref<'console' | 'result'>('console');
const socketTabs = computed(() => [
  { id: 'console' as const, label: 'Console' },
  { id: 'result' as const, label: 'Test result', disabled: !ws.currentResult },
]);
watch(
  () => ws.currentResult?.id,
  (id) => (socketTab.value = id ? 'result' : 'console'),
);
watch(
  () => ws.draft?.id,
  () => (socketTab.value = 'console'),
);

async function quickStart() {
  const name = await ui.prompt({ title: 'New collection', value: 'My API tests', confirmLabel: 'Create' });
  if (!name) return;
  await run(async () => {
    const c = await ws.createCollection(name);
    await ws.createRequest(c.id);
  });
}
</script>

<template>
  <EmptyState
    v-if="!ws.draft"
    class="welcome"
    :title="ws.collections.length ? 'Select a request' : 'Welcome to API test kits'"
  >
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
    <template #actions>
      <BaseButton variant="primary" @click="quickStart">Create collection</BaseButton>
      <BaseButton @click="ui.view = 'environments'">Set up environments</BaseButton>
    </template>
  </EmptyState>
  <div v-else class="split">
    <RequestTabs />
    <RequestEditor class="top" />
    <div class="bottom">
      <TabBar v-if="socket" v-model="socketTab" class="socket-tabs" :tabs="socketTabs" />
      <EmptyState v-if="runner.sendingOp && runner.sendingRequestId === ws.draft.id">
        <p>
          {{ socket ? 'Running the test…' : 'Sending…' }}
          <BaseButton small @click="runner.cancelSend()">Cancel (Esc)</BaseButton>
        </p>
      </EmptyState>
      <SocketConsole v-else-if="socket && socketTab === 'console'" :request="ws.draft" />
      <ResponseViewer v-else-if="ws.currentResult" :result="ws.currentResult" :request-id="ws.draft.id" allow-approve />
      <EmptyState v-else small>
        <p v-if="socket">Press <strong>Run test</strong> (Ctrl+Enter) to connect and run the test steps.</p>
        <p v-else>Press <strong>Send</strong> (Ctrl+Enter) to run this request and its tests.</p>
      </EmptyState>
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
.socket-tabs {
  padding: 0 12px;
}
.bottom {
  flex: 1 1 50%;
  min-height: 160px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
</style>

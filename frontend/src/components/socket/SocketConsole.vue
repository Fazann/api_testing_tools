<script setup lang="ts">
import { computed, ref, watch } from 'vue';

import BaseButton from '@/components/base/BaseButton.vue';
import SelectField from '@/components/base/SelectField.vue';
import { messageProblem, newSocketMessage } from '@/services/socket';
import { useSockets } from '@/stores/sockets';
import { useUi } from '@/stores/ui';
import type { Request, SocketMessage } from '@/types';
import { clone, uid } from '@/utils/object';

import SocketFrameList from './SocketFrameList.vue';
import SocketMessageFields from './SocketMessageFields.vue';

// Live console of a WebSocket or Socket.IO request: connect, send messages and watch what arrives.
const props = defineProps<{ request: Request }>();
const sockets = useSockets();
const ui = useUi();

const key = computed(() => props.request.id);
const c = computed(() => sockets.consoleFor(key.value));
const composer = ref<SocketMessage>(newSocketMessage(props.request.kind));
const savedId = ref<string | undefined>(undefined);
watch(
  () => [props.request.id, props.request.kind],
  () => {
    composer.value = newSocketMessage(props.request.kind);
    savedId.value = undefined;
  },
);

const saved = computed(() => props.request.socket?.messages ?? []);
const savedOptions = computed(() => saved.value.map((m) => ({ value: m.id, label: m.name || m.event || m.body })));
const canSend = computed(() => c.value.status === 'open' && !messageProblem(props.request.kind, composer.value));
const STATUS_LABEL = { idle: 'Not connected', connecting: 'Connecting…', open: 'Connected', closed: 'Disconnected' };

function load(id: string | undefined) {
  const m = saved.value.find((x) => x.id === id);
  if (m) composer.value = { ...clone(m), id: uid() };
}

async function send() {
  if (!canSend.value) return;
  await sockets.send(key.value, composer.value);
}

async function saveMessage() {
  const name = await ui.prompt({
    title: 'Save message',
    message: 'Saved messages are kept with the request and can be sent again from here or used in test steps.',
    value: composer.value.event || 'Message',
  });
  if (!name || !props.request.socket) return;
  props.request.socket.messages.push({ ...clone(composer.value), id: uid(), name });
  ui.toast('Message added. Save the request to keep it.', 'success');
}
</script>

<template>
  <section class="console">
    <div class="row bar">
      <span class="dot" :class="c.status" aria-hidden="true" />
      <strong>{{ STATUS_LABEL[c.status] }}</strong>
      <span v-if="c.status === 'closed' && c.error" class="fail small clip" :title="c.error">{{ c.error }}</span>
      <span class="spacer" />
      <BaseButton v-if="c.status === 'connecting'" small @click="sockets.cancelConnect(key)">Cancel</BaseButton>
      <BaseButton v-else-if="c.status === 'open'" small variant="danger" @click="sockets.disconnect(key)"
        >Disconnect</BaseButton
      >
      <BaseButton v-else small variant="primary" @click="sockets.connect(key, request)">Connect</BaseButton>
      <BaseButton small variant="ghost" :disabled="!c.frames.length" @click="sockets.clear(key)">Clear</BaseButton>
    </div>
    <div class="body">
      <div class="stack composer">
        <div class="row">
          <SelectField
            v-if="saved.length"
            v-model="savedId"
            :options="savedOptions"
            aria-label="Saved messages"
            @update:model-value="load"
          />
          <span class="spacer" />
          <BaseButton small variant="ghost" @click="saveMessage">Save message…</BaseButton>
        </div>
        <SocketMessageFields v-model="composer" :kind="request.kind" id-prefix="console" :rows="3" @submit="send" />
        <div class="row">
          <BaseButton variant="primary" :disabled="!canSend" title="Send (Ctrl+Enter)" @click="send">Send</BaseButton>
          <span v-if="c.status !== 'open'" class="muted small">Connect to send messages.</span>
          <span v-else class="muted small"
            >Values support <code v-pre>{{ variables }}</code
            >.</span
          >
        </div>
      </div>
      <SocketFrameList
        class="log"
        :frames="c.frames"
        :kind="request.kind"
        :empty="c.status === 'open' ? 'Waiting for messages…' : 'Connect to see messages.'"
      />
    </div>
  </section>
</template>

<style scoped>
.console {
  display: flex;
  flex-direction: column;
  min-height: 0;
  flex: 1;
}
.bar {
  padding: 8px 16px;
  border-bottom: 1px solid var(--line);
}
.dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: var(--fg-muted);
}
.dot.open {
  background: var(--pass);
}
.dot.connecting {
  background: var(--warn);
}
.dot.closed {
  background: var(--fail);
}
.clip {
  max-width: 50%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.body {
  display: grid;
  grid-template-columns: minmax(260px, 2fr) 3fr;
  gap: 16px;
  padding: 12px 16px;
  overflow: auto;
  flex: 1;
  min-height: 0;
}
.log {
  min-width: 0;
}
</style>

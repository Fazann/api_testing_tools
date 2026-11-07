<script setup lang="ts">
import { computed, ref, watch } from 'vue';

import BaseButton from '@/components/base/BaseButton.vue';
import BaseInput from '@/components/base/BaseInput.vue';
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

// Heartbeat: the composer's message, sent by the backend every N seconds (presence / keep-alive testing).
const MIN_INTERVAL_S = 0.1;
const MAX_INTERVAL_S = 600;
const intervalSeconds = ref(5);
const beat = computed(() => c.value.heartbeat);
const beating = computed(() => !!beat.value?.running && c.value.status === 'open');
const intervalValid = computed(
  () => intervalSeconds.value >= MIN_INTERVAL_S && intervalSeconds.value <= MAX_INTERVAL_S,
);
const beatStatus = computed(() => {
  const b = beat.value;
  if (!b) return '';
  const every = `every ${+(b.intervalMs / 1000).toFixed(3)} s`;
  if (b.running) return `♥ ${b.sent} sent, ${every}`;
  return `Stopped after ${b.sent} heartbeat${b.sent === 1 ? '' : 's'}${b.error ? `: ${b.error}` : ''}`;
});

async function startHeartbeat() {
  if (!canSend.value || !intervalValid.value) return;
  await sockets.startHeartbeat(key.value, composer.value, Math.round(intervalSeconds.value * 1000));
}

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
        <div class="row heartbeat" role="group" aria-label="Heartbeat">
          <span class="small">Send every</span>
          <BaseInput
            v-model.number="intervalSeconds"
            class="interval"
            type="number"
            :min="MIN_INTERVAL_S"
            :max="MAX_INTERVAL_S"
            step="0.1"
            aria-label="Heartbeat interval in seconds"
            :disabled="beating"
          />
          <span class="small">s</span>
          <BaseButton v-if="beating" small variant="danger" @click="sockets.stopHeartbeat(key)"
            >Stop heartbeat</BaseButton
          >
          <BaseButton
            v-else
            small
            :disabled="!canSend || !intervalValid"
            :title="
              intervalValid
                ? 'Send this message now and then on every interval, e.g. a presence heartbeat'
                : `The interval must be between ${MIN_INTERVAL_S} and ${MAX_INTERVAL_S} seconds`
            "
            @click="startHeartbeat"
            >Start heartbeat</BaseButton
          >
          <span v-if="beatStatus" class="small" :class="beating ? 'beat' : 'muted'">{{ beatStatus }}</span>
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
.heartbeat {
  flex-wrap: wrap;
  gap: 6px;
  padding-top: 8px;
  border-top: 1px dashed var(--line);
}
.interval {
  width: 80px;
}
.beat {
  color: var(--fail);
}
</style>

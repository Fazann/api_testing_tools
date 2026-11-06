<script setup lang="ts">
import { computed } from 'vue';

import BaseInput from '@/components/base/BaseInput.vue';
import FormField from '@/components/base/FormField.vue';
import TextField from '@/components/base/TextField.vue';
import type { Request } from '@/types';

// Options of a WebSocket or Socket.IO request.
const props = defineProps<{ request: Request }>();
const socket = computed(() => props.request.socket!);

const subprotocols = computed({
  get: () => socket.value.subprotocols.join(', '),
  set: (v: string) => {
    socket.value.subprotocols = v
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
  },
});
const authError = computed(() => {
  const text = socket.value.authPayload.trim();
  if (!text || text.includes('{{')) return '';
  try {
    const v = JSON.parse(text);
    return v && typeof v === 'object' && !Array.isArray(v) ? '' : 'The auth payload must be a JSON object.';
  } catch (e) {
    return `Invalid JSON: ${(e as Error).message}`;
  }
});
</script>

<template>
  <div class="stack options">
    <TextField
      v-model="request.timeoutMs"
      label="Test timeout (ms)"
      hint="Bounds connecting and the whole test run, including every wait."
      type="number"
      min="1"
      max="600000"
    />
    <template v-if="request.kind === 'socketio'">
      <TextField
        v-model="socket.path"
        label="Socket.IO path"
        input-class="mono"
        hint="Where the server is mounted. The URL's own path is the namespace, as in io('https://host/admin')."
        placeholder="/socket.io/"
      />
      <FormField label="Auth payload (JSON object)">
        <template #hint>
          Sent with the connection request and readable on the server as <code>socket.handshake.auth</code>. Use
          <code v-pre>{"token": "{{ token }}"}</code> with a secret variable.
        </template>
        <BaseInput
          v-model="socket.authPayload"
          multiline
          rows="4"
          class="mono"
          spellcheck="false"
          placeholder='{"token": "{{token}}"}'
        />
      </FormField>
      <p v-if="authError" class="fail small">{{ authError }}</p>
      <p class="muted small">Socket.IO v3 and v4 servers are supported, over the WebSocket transport only.</p>
    </template>
    <TextField
      v-else
      v-model="subprotocols"
      label="Subprotocols"
      input-class="mono"
      hint="Offered in Sec-WebSocket-Protocol, comma-separated; the server picks one."
      placeholder="graphql-transport-ws, chat.v1"
    />
  </div>
</template>

<style scoped>
.options {
  max-width: 620px;
}
</style>

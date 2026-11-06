<script setup lang="ts">
import BaseButton from '@/components/base/BaseButton.vue';
import BaseInput from '@/components/base/BaseInput.vue';
import CalloutBox from '@/components/base/CalloutBox.vue';
import { newSocketMessage } from '@/services/socket';
import type { RequestKind, SocketConfig } from '@/types';

import SocketMessageFields from './SocketMessageFields.vue';

// Saved messages of a socket request, for sending from the console and adding as test steps.
const props = defineProps<{ socket: SocketConfig; kind: RequestKind }>();

function add() {
  props.socket.messages.push({ ...newSocketMessage(props.kind), name: `Message ${props.socket.messages.length + 1}` });
}
function remove(i: number) {
  props.socket.messages.splice(i, 1);
}
</script>

<template>
  <div class="stack">
    <CalloutBox v-if="socket.messages.length === 0" tone="info">
      No saved messages. Save messages you send often, then pick them in the console or add them as test steps. Bodies
      support <code v-pre>{{ variables }}</code
      >.
    </CalloutBox>
    <div v-for="(m, i) in socket.messages" :key="m.id" class="saved">
      <div class="row">
        <BaseInput v-model="m.name" class="grow name" :aria-label="`Name of message ${i + 1}`" maxlength="200" />
        <BaseButton variant="ghost" :aria-label="`Remove message ${i + 1}`" @click="remove(i)">✕</BaseButton>
      </div>
      <SocketMessageFields v-model="socket.messages[i]" :kind="kind" :id-prefix="`saved-${m.id}`" :rows="3" />
    </div>
    <div class="row">
      <BaseButton @click="add">Add message</BaseButton>
    </div>
  </div>
</template>

<style scoped>
.saved {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 10px;
  border: 1px solid var(--line);
  border-radius: var(--radius);
  background: var(--bg-subtle);
}
.name {
  font-weight: 600;
}
</style>

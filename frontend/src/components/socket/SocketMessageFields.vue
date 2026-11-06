<script setup lang="ts">
import { computed } from 'vue';

import BaseButton from '@/components/base/BaseButton.vue';
import BaseInput from '@/components/base/BaseInput.vue';
import CheckboxField from '@/components/base/CheckboxField.vue';
import RadioGroup from '@/components/base/RadioGroup.vue';
import { messageProblem } from '@/services/socket';
import type { RequestKind, SocketBodyType, SocketMessage } from '@/types';

// Event name (Socket.IO), body, body type and acknowledgement of one message: used by the console composer,
// saved messages and send steps.
const message = defineModel<SocketMessage>({ required: true });
const props = defineProps<{ kind: RequestKind; rows?: number; idPrefix: string }>();
const emit = defineEmits<{ submit: [] }>();

const BODY_TYPES: { value: SocketBodyType; label: string }[] = [
  { value: 'json', label: 'JSON' },
  { value: 'text', label: 'Text' },
];
const problem = computed(() => messageProblem(props.kind, message.value));

function format() {
  try {
    message.value.body = JSON.stringify(JSON.parse(message.value.body), null, 2);
  } catch {
    /* leave invalid JSON untouched */
  }
}
</script>

<template>
  <div class="stack message">
    <div class="row">
      <BaseInput
        v-if="kind === 'socketio'"
        v-model="message.event"
        class="mono event"
        placeholder="event name"
        aria-label="Event name"
        maxlength="200"
      />
      <RadioGroup v-model="message.bodyType" class="small" :name="`${idPrefix}-type`" :options="BODY_TYPES" />
      <CheckboxField
        v-if="kind === 'socketio'"
        v-model="message.ack"
        class="small"
        label="Ask for acknowledgement"
        hint="The server's callback reply arrives as an (ack) message."
      />
      <span class="spacer" />
      <BaseButton v-if="message.bodyType === 'json'" variant="ghost" small @click="format">Format JSON</BaseButton>
    </div>
    <BaseInput
      v-model="message.body"
      multiline
      class="mono"
      :rows="rows ?? 4"
      spellcheck="false"
      aria-label="Message body"
      :placeholder="message.bodyType === 'json' ? '{&quot;type&quot;: &quot;ping&quot;}' : 'Message text'"
      @keydown.ctrl.enter.prevent="emit('submit')"
      @keydown.meta.enter.prevent="emit('submit')"
    />
    <p v-if="problem" class="fail small">{{ problem }}</p>
  </div>
</template>

<style scoped>
.event {
  width: 200px;
}
</style>

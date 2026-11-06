<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue';

import { parseCurl } from '../services/curl';
import { useUi } from '../stores/ui';
import { useWorkspace } from '../stores/workspace';
import ModalFrame from './ModalFrame.vue';

const props = defineProps<{ collectionId: string; parentId?: string }>();
const emit = defineEmits<{ close: [] }>();
const ws = useWorkspace();
const ui = useUi();

const placeholder = [
  `curl 'https://api.example.com/orders'`,
  `-H 'Content-Type: application/json'`,
  `--data-raw '{"amount": 10}'`,
].join(' ');
const text = ref('');
const busy = ref(false);
const area = ref<HTMLTextAreaElement | null>(null);
onMounted(() => nextTick(() => area.value?.focus()));

const parsed = computed(() => {
  if (!text.value.trim()) return null;
  try {
    return { ok: true as const, ...parseCurl(text.value) };
  } catch (e) {
    return { ok: false as const, error: (e as Error).message };
  }
});

const authLabel = computed(() => {
  if (!parsed.value?.ok) return '';
  const a = parsed.value.request.auth;
  return a.type === 'inherit' ? 'inherited from collection' : a.type === 'none' ? 'none' : a.type;
});

async function create() {
  if (!parsed.value?.ok || busy.value) return;
  busy.value = true;
  try {
    await ws.createRequest(props.collectionId, props.parentId ?? '', parsed.value.request);
    ui.view = 'requests';
    ui.toast(`Created "${parsed.value.request.name}" from cURL`, 'success');
    emit('close');
  } catch (e) {
    ui.error(e);
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <ModalFrame title="New request from cURL" wide @close="emit('close')">
    <p class="muted small">
      Paste a command, e.g. from your browser's developer tools (Network → right-click → Copy as cURL (bash)) or from
      API documentation.
    </p>
    <textarea
      ref="area"
      v-model="text"
      class="mono"
      rows="10"
      spellcheck="false"
      aria-label="cURL command"
      :placeholder="placeholder"
      @keydown.ctrl.enter.prevent="create"
      @keydown.meta.enter.prevent="create"
    />
    <p v-if="parsed && !parsed.ok" class="fail small">{{ parsed.error }}</p>
    <div v-else-if="parsed?.ok" class="preview small">
      <div class="row">
        <span class="method" :class="parsed.request.method">{{ parsed.request.method }}</span>
        <code class="url">{{ parsed.request.url }}</code>
      </div>
      <div class="muted">
        {{ parsed.request.headers.length }} header{{ parsed.request.headers.length === 1 ? '' : 's' }} · body:
        {{ parsed.request.bodyType === 'none' ? 'none' : parsed.request.bodyType.toUpperCase() }} · auth:
        {{ authLabel }}
      </div>
      <ul v-if="parsed.warnings.length" class="warnings">
        <li v-for="w in parsed.warnings" :key="w" class="warn">{{ w }}</li>
      </ul>
      <p v-if="parsed.request.auth.type === 'bearer' || parsed.request.auth.type === 'basic'" class="muted">
        Tip: after creating, replace pasted credentials with a secret <code v-pre>{{ variable }}</code> so they are not
        stored in the collection or exports.
      </p>
    </div>
    <template #footer>
      <button @click="emit('close')">Cancel</button>
      <button class="primary" :disabled="!parsed?.ok || busy" title="Create (Ctrl+Enter)" @click="create">
        Create request
      </button>
    </template>
  </ModalFrame>
</template>

<style scoped>
textarea {
  width: 100%;
  resize: vertical;
}
.preview {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.url {
  overflow-wrap: anywhere;
}
.warnings {
  margin: 0;
  padding-left: 18px;
}
.preview p {
  margin: 0;
}
</style>

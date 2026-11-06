<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue';

import BaseButton from '@/components/base/BaseButton.vue';
import BaseInput from '@/components/base/BaseInput.vue';
import MethodBadge from '@/components/base/MethodBadge.vue';
import ModalFrame from '@/components/base/ModalFrame.vue';
import { useAsyncAction } from '@/composables/useAsyncAction';
import { parseCurl } from '@/services/curl';
import { useUi } from '@/stores/ui';
import { useWorkspace } from '@/stores/workspace';

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
const { busy: busy, run: runBusy } = useAsyncAction();
const area = ref<InstanceType<typeof BaseInput> | null>(null);
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
  const p = parsed.value;
  if (!p?.ok || busy.value) return;
  await runBusy(async () => {
    await ws.createRequest(props.collectionId, props.parentId ?? '', p.request);
    ui.view = 'requests';
    ui.toast(`Created "${p.request.name}" from cURL`, 'success');
    emit('close');
  });
}
</script>

<template>
  <ModalFrame title="New request from cURL" wide @close="emit('close')">
    <p class="muted small">
      Paste a command, e.g. from your browser's developer tools (Network → right-click → Copy as cURL (bash)) or from
      API documentation.
    </p>
    <BaseInput
      ref="area"
      multiline
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
        <MethodBadge :method="parsed.request.method" />
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
      <BaseButton @click="emit('close')">Cancel</BaseButton>
      <BaseButton variant="primary" :disabled="!parsed?.ok || busy" title="Create (Ctrl+Enter)" @click="create">
        Create request
      </BaseButton>
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

<script setup lang="ts">
import { computed, ref, watch } from 'vue';

import BaseButton from '@/components/base/BaseButton.vue';
import CalloutBox from '@/components/base/CalloutBox.vue';
import CheckboxField from '@/components/base/CheckboxField.vue';
import ModalFrame from '@/components/base/ModalFrame.vue';
import SelectField from '@/components/base/SelectField.vue';
import { useAsyncAction } from '@/composables/useAsyncAction';
import { CODE_LANGUAGES, generateCode, type CodeLanguage } from '@/services/codegen';
import { hasVariables } from '@/services/variables';
import { useEnvironments } from '@/stores/environments';
import { useUi } from '@/stores/ui';
import { useWorkspace } from '@/stores/workspace';
import type { Request } from '@/types';

const props = defineProps<{ request: Request }>();
const emit = defineEmits<{ close: [] }>();
const ws = useWorkspace();
const envs = useEnvironments();
const ui = useUi();
const { run } = useAsyncAction();

const collection = computed(() => ws.collections.find((c) => c.id === props.request.collectionId) ?? null);
// The last language used is remembered on this machine.
const LANGUAGE_KEY = 'arg.codeLanguage';
function readLanguage(): CodeLanguage {
  try {
    const v = localStorage.getItem(LANGUAGE_KEY);
    return CODE_LANGUAGES.some((l) => l.value === v) ? (v as CodeLanguage) : 'curl';
  } catch {
    return 'curl';
  }
}
const language = ref<CodeLanguage>(readLanguage());
watch(language, (v) => {
  try {
    localStorage.setItem(LANGUAGE_KEY, v);
  } catch {
    /* a convenience only */
  }
});
const languageLabel = computed(() => CODE_LANGUAGES.find((l) => l.value === language.value)?.label ?? '');

const fill = ref(true);
const includeSecrets = ref(false);

const usesVariables = computed(() => {
  const r = props.request;
  const auth = r.auth.type === 'inherit' ? collection.value?.auth : r.auth;
  return [
    r.url,
    r.body,
    ...r.headers.flatMap((h) => [h.key, h.value]),
    ...r.query.flatMap((q) => [q.key, q.value]),
    auth?.token,
    auth?.username,
    auth?.password,
    auth?.keyName,
    auth?.keyValue,
  ].some(hasVariables);
});

const command = computed(() =>
  generateCode(language.value, props.request, {
    collectionAuth: collection.value?.auth,
    resolve:
      fill.value && usesVariables.value
        ? {
            environment: envs.selected,
            collectionVariables: collection.value?.variables ?? [],
            includeSecrets: includeSecrets.value,
          }
        : undefined,
  }),
);

const hasScripts = computed(
  () =>
    !!(props.request.preRequestScript?.trim() || collection.value?.preRequestScript?.trim()) ||
    !!props.request.postResponseScript?.trim(),
);

async function copy() {
  await run(async () => {
    await navigator.clipboard.writeText(command.value);
    ui.toast(`Copied ${languageLabel.value} code`, 'success');
    emit('close');
  });
}
</script>

<template>
  <ModalFrame :title="`Copy “${request.name}” as code`" wide @close="emit('close')">
    <SelectField v-model="language" label="Language" :options="CODE_LANGUAGES" class="language" />
    <div v-if="usesVariables" class="stack options">
      <CheckboxField v-model="fill" class="small">
        Fill in variables from
        {{ envs.selected ? `“${envs.selected.name}”` : 'the collection (no environment selected)' }}
      </CheckboxField>
      <CheckboxField v-model="includeSecrets" class="small" :class="{ muted: !fill }" :disabled="!fill">
        Include secret values (otherwise they stay as <code v-pre>{{ name }}</code
        >)
      </CheckboxField>
    </div>
    <pre class="mono command">{{ command }}</pre>
    <CalloutBox v-if="fill && includeSecrets" small>
      The command contains secret values. Avoid pasting it into chats, tickets or shell history you share.
    </CalloutBox>
    <p v-if="hasScripts" class="muted small">Scripts are not included; the code sends the request as written.</p>
    <template #footer>
      <BaseButton @click="emit('close')">Close</BaseButton>
      <BaseButton variant="primary" @click="copy">Copy to clipboard</BaseButton>
    </template>
  </ModalFrame>
</template>

<style scoped>
.language {
  max-width: 280px;
}
.options {
  gap: 4px;
}
.command {
  margin: 0;
  padding: 10px 12px;
  background: var(--bg-subtle);
  border: 1px solid var(--line);
  border-radius: var(--radius);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  max-height: 50vh;
  overflow: auto;
}
</style>

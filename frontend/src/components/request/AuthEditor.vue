<script setup lang="ts">
import { computed } from 'vue';

import BaseButton from '@/components/base/BaseButton.vue';
import CalloutBox from '@/components/base/CalloutBox.vue';
import FormField from '@/components/base/FormField.vue';
import SelectField from '@/components/base/SelectField.vue';
import SecretInput from '@/components/variables/SecretInput.vue';
import VariableInput from '@/components/variables/VariableInput.vue';
import type { Auth, Collection } from '@/types';

// inheritFrom is set for requests: it enables "Inherit auth from collection"
// and shows what the collection provides. Collections themselves omit it.
const props = defineProps<{ auth: Auth; inheritFrom?: Collection | null }>();
const emit = defineEmits<{ editCollection: [] }>();

const TYPE_LABELS: Record<string, string> = {
  none: 'No auth',
  bearer: 'Bearer token',
  basic: 'Basic auth',
  apikey: 'API key',
};

const typeOptions = computed(() => [
  ...(props.inheritFrom !== undefined ? [{ value: 'inherit' as const, label: 'Inherit auth from collection' }] : []),
  ...(['none', 'bearer', 'basic', 'apikey'] as const).map((value) => ({ value, label: TYPE_LABELS[value] })),
]);
const KEY_IN = [
  { value: 'header' as const, label: 'Header' },
  { value: 'query' as const, label: 'Query parameter' },
];
const inherited = computed(() => props.inheritFrom?.auth ?? { type: 'none' as const });
const inheritedSummary = computed(() => {
  const a = inherited.value;
  switch (a.type) {
    case 'bearer':
      return `Bearer ${a.token || '(empty token)'}`;
    case 'basic':
      return `Basic ${a.username || '(no username)'}`;
    case 'apikey':
      return `API key ${a.keyName} in ${a.keyIn === 'query' ? 'query' : 'header'}`;
  }
  return 'No auth';
});
</script>

<template>
  <div class="stack auth">
    <SelectField v-model="auth.type" label="Type" :options="typeOptions" />

    <template v-if="auth.type === 'inherit'">
      <CalloutBox v-if="inheritFrom" tone="info">
        <div>
          Uses the auth of collection <strong>{{ inheritFrom.name }}</strong
          >:
          <span class="mono">{{ inheritedSummary }}</span>
        </div>
        <p v-if="inherited.type === 'none'" class="muted small">
          The collection has no auth, so this request is sent without credentials.
        </p>
        <BaseButton small class="edit" @click="emit('editCollection')">Edit collection auth…</BaseButton>
      </CalloutBox>
      <CalloutBox v-else small>
        This request is not in a collection, so there is nothing to inherit; it is sent without credentials.
      </CalloutBox>
    </template>
    <template v-else-if="auth.type === 'bearer'">
      <FormField label="Token"><SecretInput v-model="auth.token" placeholder="{{token}}" label="Token" /></FormField>
    </template>
    <template v-else-if="auth.type === 'basic'">
      <FormField label="Username"><VariableInput v-model="auth.username" placeholder="{{username}}" /></FormField>
      <FormField label="Password">
        <SecretInput v-model="auth.password" placeholder="{{password}}" label="Password" />
      </FormField>
    </template>
    <template v-else-if="auth.type === 'apikey'">
      <FormField label="Key name"><VariableInput v-model="auth.keyName" placeholder="X-API-Key" /></FormField>
      <FormField label="Key value">
        <SecretInput v-model="auth.keyValue" placeholder="{{api_key}}" label="Key value" />
      </FormField>
      <SelectField v-model="auth.keyIn" label="Send in" :options="KEY_IN" />
    </template>

    <CalloutBox v-if="auth.type !== 'none' && auth.type !== 'inherit'" tone="info" small>
      Tip: reference a <strong>secret</strong> environment variable such as <code v-pre>{{ token }}</code> instead of
      pasting credentials here. Secret variables are masked in the UI, redacted from history and reports, and left out
      of exports.
    </CalloutBox>
    <p v-if="inheritFrom === undefined && auth.type !== 'none'" class="muted small">
      Requests set to “Inherit auth from collection” use this ({{ TYPE_LABELS[auth.type] }}).
    </p>
  </div>
</template>

<style scoped>
.auth {
  max-width: 560px;
}
.edit {
  margin-top: 8px;
}
</style>

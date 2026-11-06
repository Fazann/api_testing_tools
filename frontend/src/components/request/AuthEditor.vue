<script setup lang="ts">
import { computed } from 'vue';

import type { Auth, Collection } from '../types';
import SecretInput from './SecretInput.vue';
import VariableInput from './VariableInput.vue';

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
    <label class="field">
      <span>Type</span>
      <select v-model="auth.type">
        <option v-if="inheritFrom !== undefined" value="inherit">Inherit auth from collection</option>
        <option value="none">No auth</option>
        <option value="bearer">Bearer token</option>
        <option value="basic">Basic auth</option>
        <option value="apikey">API key</option>
      </select>
    </label>

    <template v-if="auth.type === 'inherit'">
      <div v-if="inheritFrom" class="callout info">
        <div>
          Uses the auth of collection <strong>{{ inheritFrom.name }}</strong
          >:
          <span class="mono">{{ inheritedSummary }}</span>
        </div>
        <p v-if="inherited.type === 'none'" class="muted small">
          The collection has no auth, so this request is sent without credentials.
        </p>
        <button class="small edit" type="button" @click="emit('editCollection')">Edit collection auth…</button>
      </div>
      <p v-else class="callout small">
        This request is not in a collection, so there is nothing to inherit; it is sent without credentials.
      </p>
    </template>
    <template v-else-if="auth.type === 'bearer'">
      <label class="field"
        ><span>Token</span><SecretInput v-model="auth.token" placeholder="{{token}}" label="Token"
      /></label>
    </template>
    <template v-else-if="auth.type === 'basic'">
      <label class="field"
        ><span>Username</span><VariableInput v-model="auth.username" placeholder="{{username}}"
      /></label>
      <label class="field"
        ><span>Password</span><SecretInput v-model="auth.password" placeholder="{{password}}" label="Password"
      /></label>
    </template>
    <template v-else-if="auth.type === 'apikey'">
      <label class="field"><span>Key name</span><VariableInput v-model="auth.keyName" placeholder="X-API-Key" /></label>
      <label class="field"
        ><span>Key value</span><SecretInput v-model="auth.keyValue" placeholder="{{api_key}}" label="Key value"
      /></label>
      <label class="field">
        <span>Send in</span>
        <select v-model="auth.keyIn">
          <option value="header">Header</option>
          <option value="query">Query parameter</option>
        </select>
      </label>
    </template>

    <p v-if="auth.type !== 'none' && auth.type !== 'inherit'" class="callout info small">
      Tip: reference a <strong>secret</strong> environment variable such as <code v-pre>{{ token }}</code> instead of
      pasting credentials here. Secret variables are masked in the UI, redacted from history and reports, and left out
      of exports.
    </p>
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

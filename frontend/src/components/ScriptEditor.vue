<script setup lang="ts">
import { computed, ref } from 'vue';

import { SCRIPT_PLACEHOLDERS, SCRIPT_SNIPPETS, type ScriptPhase } from '../services/scripts';
import CodeEditor from './CodeEditor.vue';

const pre = defineModel<string>('pre', { required: true });
const post = defineModel<string>('post', { required: true });
defineProps<{ scope?: 'request' | 'collection' }>();

const phase = ref<ScriptPhase>('pre');
const snippet = ref('');

const code = computed({
  get: () => (phase.value === 'pre' ? pre.value : post.value),
  set: (v: string) => {
    if (phase.value === 'pre') pre.value = v;
    else post.value = v;
  },
});
const snippets = computed(() => SCRIPT_SNIPPETS.filter((s) => s.phase === phase.value));

function insertSnippet() {
  const s = snippets.value.find((x) => x.label === snippet.value);
  snippet.value = '';
  if (!s) return;
  const current = code.value.trimEnd();
  code.value = (current ? current + '\n\n' : '') + s.code + '\n';
}
</script>

<template>
  <div class="stack scripts">
    <div class="row">
      <nav class="tabs inner" role="tablist">
        <button role="tab" :aria-selected="phase === 'pre'" :class="{ active: phase === 'pre' }" @click="phase = 'pre'">
          Pre-request<span v-if="pre.trim()" class="count">✓</span>
        </button>
        <button
          role="tab"
          :aria-selected="phase === 'post'"
          :class="{ active: phase === 'post' }"
          @click="phase = 'post'"
        >
          Post-response<span v-if="post.trim()" class="count">✓</span>
        </button>
      </nav>
      <span class="spacer" />
      <select v-model="snippet" class="small" aria-label="Insert snippet" @change="insertSnippet">
        <option value="" disabled>Insert snippet…</option>
        <option v-for="s in snippets" :key="s.label" :value="s.label">{{ s.label }}</option>
      </select>
    </div>
    <p class="muted small">
      <template v-if="phase === 'pre'">
        Runs before {{ scope === 'collection' ? 'every request in this collection' : 'the request is sent' }}, before
        <code v-pre>{{ variables }}</code> are resolved. Set values with
        <code>pm.variables.set('signature', …)</code> and use them as <code v-pre>{{ signature }}</code
        >, or change the request with <code>pm.request.headers.upsert(…)</code>.
      </template>
      <template v-else>
        Runs after the response arrives{{ scope === 'collection' ? ', for every request in this collection' : '' }}. Add
        checks with <code>pm.test(…)</code> and save values with <code>pm.environment.set(…)</code>.
      </template>
      <template v-if="scope === 'collection'"> Runs before the request's own script.</template>
    </p>
    <CodeEditor
      v-model="code"
      language="javascript"
      :label="phase === 'pre' ? 'Pre-request script' : 'Post-response script'"
      :placeholder="SCRIPT_PLACEHOLDERS[phase]"
    />
    <details class="small help">
      <summary>Available APIs and libraries</summary>
      <ul>
        <li>
          <code>pm.environment</code>, <code>pm.collectionVariables</code>: <code>.get .set .unset .has</code> — changes
          are saved after the run. <code>pm.variables</code>: <code>.get .set .replaceIn</code> — only for this request.
        </li>
        <li>
          <code>pm.request</code>: <code>.method .url .headers .body.raw</code> (editable in pre-request scripts);
          <code>pm.response</code>: <code>.code .status .responseTime .headers .json() .text()</code>
        </li>
        <li>
          <code>pm.test(name, fn)</code>,
          <code>pm.expect(value).to.equal / eql / include / have.property / be.a / be.below …</code>,
          <code>pm.response.to.have.status(200)</code>, <code>console.log(…)</code> (shown under Console, secrets
          redacted)
        </li>
        <li>
          <code>CryptoJS</code> (<code>require('crypto-js')</code>: MD5, SHA1/256/512, HmacSHA256, AES, DES, Base64,
          Hex…), <code>_</code> (lodash), <code>moment</code>, <code>require('uuid').v4()</code>,
          <code>atob</code>/<code>btoa</code>
        </li>
        <li>
          <code>pm.crypto.rsaEncrypt(publicKeyPem, text, {padding: 'pkcs1' | 'oaep', hash: 'sha256'})</code>,
          <code>pm.crypto.rsaSign(privateKeyPem, text, {padding: 'pkcs1' | 'pss', hash: 'sha256'})</code> → base64
        </li>
      </ul>
      <p class="muted">Scripts cannot access files or the network, and stop after 10 seconds.</p>
    </details>
  </div>
</template>

<style scoped>
.inner {
  padding: 0;
  border-bottom: none;
}
.help ul {
  margin: 6px 0;
  padding-left: 18px;
}
.help li {
  margin: 3px 0;
}
</style>

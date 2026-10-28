<script setup lang="ts">
import { computed, ref, watch } from 'vue';

import VariableTable from '../components/VariableTable.vue';
import { clone } from '../services/model';
import { useEnvironments } from '../stores/environments';
import { useUi } from '../stores/ui';
import type { Environment } from '../types';

const envs = useEnvironments();
const ui = useUi();

const editingId = ref(envs.selectedId || envs.list[0]?.id || '');
const form = ref<Environment | null>(null);
const snapshot = ref('');
const dirty = computed(() => form.value !== null && JSON.stringify(form.value) !== snapshot.value);

function loadForm() {
  const e = envs.list.find((x) => x.id === editingId.value);
  form.value = e ? clone(e) : null;
  snapshot.value = JSON.stringify(form.value);
}
watch(() => [editingId.value, envs.list], loadForm, { immediate: true });

async function select(id: string) {
  if (id === editingId.value) return;
  if (dirty.value && !(await ui.confirm({ title: 'Discard unsaved changes?', confirmLabel: 'Discard', danger: true })))
    return;
  editingId.value = id;
}

async function create() {
  const name = await ui.prompt({ title: 'New environment', value: 'Local', confirmLabel: 'Create' });
  if (!name) return;
  try {
    const e = await envs.save({
      name,
      protected: false,
      variables: [{ key: 'base_url', value: 'http://localhost:8080', secret: false, enabled: true }],
    });
    editingId.value = e.id;
    if (!envs.selectedId) envs.selectedId = e.id;
  } catch (err) {
    ui.error(err);
  }
}

async function save() {
  if (!form.value) return;
  try {
    await envs.save(clone(form.value));
    ui.toast('Environment saved', 'success');
  } catch (e) {
    ui.error(e);
  }
}

async function duplicate() {
  if (!form.value) return;
  try {
    const e = await envs.duplicate(form.value.id);
    editingId.value = e.id;
  } catch (err) {
    ui.error(err);
  }
}

async function remove() {
  if (!form.value) return;
  const ok = await ui.confirm({
    title: `Delete environment "${form.value.name}"?`,
    message: 'Its variables and the baselines approved for it will be deleted.',
    confirmLabel: 'Delete',
    danger: true,
  });
  if (!ok) return;
  try {
    await envs.remove(form.value.id);
    editingId.value = envs.list[0]?.id ?? '';
  } catch (e) {
    ui.error(e);
  }
}
</script>

<template>
  <div class="envs">
    <aside class="list">
      <div class="row head">
        <strong class="grow">Environments</strong
        ><button class="ghost" aria-label="New environment" @click="create">＋</button>
      </div>
      <button
        v-for="e in envs.list"
        :key="e.id"
        class="ghost item"
        :class="{ active: e.id === editingId }"
        @click="select(e.id)"
      >
        <span class="grow name">{{ e.name }}</span>
        <span v-if="e.protected" class="badge failed">protected</span>
        <span v-if="e.id === envs.selectedId" class="badge passed">active</span>
      </button>
      <div v-if="envs.list.length === 0" class="empty small">
        <p>No environments yet.</p>
        <button class="primary" @click="create">Create environment</button>
      </div>
    </aside>
    <section class="panel editor">
      <div v-if="!form" class="empty">
        <h2>Environments</h2>
        <p>
          Environments hold variables such as <code>base_url</code> and credentials. Reference them as
          <code v-pre>{{ name }}</code> in URLs, headers, query parameters, bodies and auth. They override collection
          variables with the same name.
        </p>
      </div>
      <template v-else>
        <div class="row">
          <input v-model="form.name" class="name grow" aria-label="Environment name" maxlength="200" />
          <button v-if="form.id !== envs.selectedId" @click="envs.selectedId = form.id">Use this environment</button>
          <button @click="duplicate">Duplicate</button>
          <button class="danger" @click="remove">Delete</button>
          <button class="primary" :disabled="!dirty" @click="save">Save</button>
        </div>
        <label class="row prot">
          <input v-model="form.protected" type="checkbox" />
          <span
            ><strong>Protected (production-like)</strong> — POST, PUT, PATCH and DELETE requests require explicit
            confirmation.</span
          >
        </label>
        <VariableTable :variables="form.variables" />
        <p class="callout small">
          Secret values are masked here, redacted from run history and reports, and left out of collection exports
          unless you explicitly include them. They are stored in the local database file with owner-only file
          permissions, <strong>not encrypted</strong>. See docs/security.md.
        </p>
      </template>
    </section>
  </div>
</template>

<style scoped>
.envs {
  display: flex;
  flex: 1;
  min-height: 0;
}
.list {
  width: 240px;
  flex-shrink: 0;
  border-right: 1px solid var(--line);
  background: var(--bg-subtle);
  padding: 6px;
  overflow: auto;
}
.head {
  padding: 4px 4px 8px 6px;
}
.item {
  width: 100%;
  display: flex;
  gap: 6px;
  align-items: center;
  text-align: left;
}
.item.active {
  background: var(--bg-muted);
}
.name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.editor {
  flex: 1;
  overflow: auto;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.editor .name {
  font-size: 15px;
  font-weight: 600;
}
.prot {
  align-items: flex-start;
}
</style>

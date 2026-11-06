<script setup lang="ts">
import { computed, ref, watch } from 'vue';

import BaseButton from '@/components/base/BaseButton.vue';
import BaseInput from '@/components/base/BaseInput.vue';
import CalloutBox from '@/components/base/CalloutBox.vue';
import CheckboxField from '@/components/base/CheckboxField.vue';
import EmptyState from '@/components/base/EmptyState.vue';
import SelectableList from '@/components/base/SelectableList.vue';
import StatusBadge from '@/components/base/StatusBadge.vue';
import VariableTable from '@/components/variables/VariableTable.vue';
import { useAsyncAction } from '@/composables/useAsyncAction';
import { useEnvironments } from '@/stores/environments';
import { useUi } from '@/stores/ui';
import type { Environment } from '@/types';
import { clone } from '@/utils/object';

const envs = useEnvironments();
const ui = useUi();
const { run } = useAsyncAction();

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
  await run(async () => {
    const e = await envs.save({
      name,
      protected: false,
      variables: [{ key: 'base_url', value: 'http://localhost:8080', secret: false, enabled: true }],
    });
    editingId.value = e.id;
    if (!envs.selectedId) envs.selectedId = e.id;
  });
}

async function save() {
  const env = form.value;
  if (!env) return;
  await run(async () => {
    await envs.save(clone(env));
    ui.toast('Environment saved', 'success');
  });
}

async function duplicate() {
  const id = form.value?.id;
  if (!id) return;
  await run(async () => {
    const e = await envs.duplicate(id);
    editingId.value = e.id;
  });
}

async function remove() {
  const env = form.value;
  if (!env) return;
  const ok = await ui.confirm({
    title: `Delete environment "${env.name}"?`,
    message: 'Its variables and the baselines approved for it will be deleted.',
    confirmLabel: 'Delete',
    danger: true,
  });
  if (!ok) return;
  await run(async () => {
    await envs.remove(env.id);
    editingId.value = envs.list[0]?.id ?? '';
  });
}
</script>

<template>
  <div class="envs">
    <aside class="list">
      <div class="row head">
        <strong class="grow">Environments</strong
        ><BaseButton variant="ghost" aria-label="New environment" @click="create">＋</BaseButton>
      </div>
      <SelectableList
        :items="envs.list"
        :item-key="(e) => e.id"
        :active-key="editingId"
        label="Environments"
        @select="(e) => select(e.id)"
      >
        <template #default="{ item: e }">
          <span class="grow name">{{ e.name }}</span>
          <StatusBadge v-if="e.protected" status="failed" label="protected" />
          <StatusBadge v-if="e.id === envs.selectedId" status="passed" label="active" />
        </template>
        <template #empty>
          <EmptyState small message="No environments yet.">
            <template #actions>
              <BaseButton variant="primary" @click="create">Create environment</BaseButton>
            </template>
          </EmptyState>
        </template>
      </SelectableList>
    </aside>
    <section class="panel editor">
      <EmptyState v-if="!form" title="Environments">
        <p>
          Environments hold variables such as <code>base_url</code> and credentials. Reference them as
          <code v-pre>{{ name }}</code> in URLs, headers, query parameters, bodies and auth. They override collection
          variables with the same name.
        </p>
      </EmptyState>
      <template v-else>
        <div class="row">
          <BaseInput v-model="form.name" class="name grow" aria-label="Environment name" maxlength="200" />
          <BaseButton v-if="form.id !== envs.selectedId" @click="envs.selectedId = form.id"
            >Use this environment</BaseButton
          >
          <BaseButton @click="duplicate">Duplicate</BaseButton>
          <BaseButton variant="danger" @click="remove">Delete</BaseButton>
          <BaseButton variant="primary" :disabled="!dirty" @click="save">Save</BaseButton>
        </div>
        <CheckboxField v-model="form.protected">
          <strong>Protected (production-like)</strong> — POST, PUT, PATCH and DELETE requests require explicit
          confirmation.
        </CheckboxField>
        <VariableTable :variables="form.variables" />
        <CalloutBox small>
          Secret values are masked here, redacted from run history and reports, and left out of collection exports
          unless you explicitly include them. They are stored in the local database file with owner-only file
          permissions, <strong>not encrypted</strong>. See docs/security.md.
        </CalloutBox>
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
</style>

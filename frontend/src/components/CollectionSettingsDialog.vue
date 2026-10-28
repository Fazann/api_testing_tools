<script setup lang="ts">
import { computed, onMounted, ref, toRef } from 'vue';

import { provideCollectionVariables } from '../services/collectionVariables';
import { clone } from '../services/model';
import { useUi } from '../stores/ui';
import { useWorkspace } from '../stores/workspace';
import type { Collection } from '../types';
import AuthEditor from './AuthEditor.vue';
import ModalFrame from './ModalFrame.vue';
import VariableTable from './VariableTable.vue';

const props = defineProps<{ collection: Collection }>();
const emit = defineEmits<{ close: [] }>();
const ws = useWorkspace();
const ui = useUi();

const form = ref(clone(props.collection));
if (!form.value.auth) form.value.auth = { type: 'none' };
if (!form.value.variables) form.value.variables = [];
provideCollectionVariables(toRef(() => form.value.variables));
const required = ref((props.collection.requiredVariables ?? []).join(', '));
const applyToAll = ref(false);
const saving = ref(false);

onMounted(() => {
  if (!ws.trees[props.collection.id]) ws.loadTree(props.collection.id).catch(ui.error);
});

/** Requests that use their own auth instead of the collection's. */
const ownAuth = computed(() =>
  (ws.trees[props.collection.id]?.requests ?? []).filter((r) => r.auth.type !== 'inherit'),
);

async function save() {
  form.value.requiredVariables = required.value
    .split(/[\s,]+/)
    .map((s) => s.trim())
    .filter(Boolean);
  form.value.variables = form.value.variables.filter((v) => v.key.trim() !== '' || v.value !== '');
  saving.value = true;
  try {
    await ws.updateCollection(form.value);
    let message = 'Collection saved';
    if (applyToAll.value) {
      const n = await ws.inheritCollectionAuth(form.value.id);
      message += `; ${n} request${n === 1 ? '' : 's'} now use the collection auth`;
    }
    ui.toast(message, 'success');
    emit('close');
  } catch (e) {
    ui.error(e);
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <ModalFrame title="Collection settings" wide @close="emit('close')">
    <label class="field"><span>Name</span><input v-model="form.name" maxlength="200" /></label>
    <label class="field"><span>Description</span><textarea v-model="form.description" rows="3" /></label>

    <h3 class="section-title">Variables</h3>
    <p class="muted small">
      Every request in this collection can use these, for example <code v-pre>{{ base_url }}</code> or
      <code v-pre>{{ token }}</code
      >. Change a value once here and all requests follow. A variable with the same name in the selected environment
      takes precedence.
    </p>
    <VariableTable :variables="form.variables" />

    <label class="field">
      <span>Required variables</span>
      <input v-model="required" class="mono" placeholder="base_url, token" />
      <span class="muted small"
        >The collection will not run unless these are set and non-empty, here or in the selected environment.</span
      >
    </label>

    <h3 class="section-title">Authorization</h3>
    <p class="muted small">
      Set auth once here, for example Bearer <code v-pre>{{ token }}</code
      >. Requests whose Auth is “Inherit auth from collection” use it; any request can override it.
    </p>
    <AuthEditor :auth="form.auth" />
    <label v-if="ownAuth.length > 0" class="row apply">
      <input v-model="applyToAll" type="checkbox" />
      <span>
        <strong>Use this auth for all requests</strong> — {{ ownAuth.length }} request{{
          ownAuth.length === 1 ? ' has' : 's have'
        }}
        their own auth ({{
          ownAuth
            .slice(0, 3)
            .map((r) => r.name)
            .join(', ')
        }}{{ ownAuth.length > 3 ? ', …' : '' }}) and will be switched to inherit.
      </span>
    </label>
    <p v-else-if="ws.trees[collection.id]" class="muted small">All requests in this collection use this auth.</p>

    <template #footer>
      <button @click="emit('close')">Cancel</button>
      <button class="primary" :disabled="saving" @click="save">Save</button>
    </template>
  </ModalFrame>
</template>

<style scoped>
.apply {
  align-items: flex-start;
}
</style>

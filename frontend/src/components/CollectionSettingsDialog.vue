<script setup lang="ts">
import { computed, onMounted, ref, toRef } from 'vue';

import { provideCollectionVariables } from '../services/collectionVariables';
import { clone } from '../services/model';
import { useOpenRequest } from '../services/navigation';
import { useUi } from '../stores/ui';
import { useWorkspace } from '../stores/workspace';
import type { Collection, Request } from '../types';
import AuthEditor from './AuthEditor.vue';
import CollectionEndpoints from './CollectionEndpoints.vue';
import CollectionResponseTimes from './CollectionResponseTimes.vue';
import ModalFrame from './ModalFrame.vue';
import PermissionMatrix from './PermissionMatrix.vue';
import ScriptEditor from './ScriptEditor.vue';
import VariableTable from './VariableTable.vue';

const props = defineProps<{
  collection: Collection;
  initialTab?: 'endpoints' | 'times' | 'permissions' | 'settings';
}>();
const emit = defineEmits<{ close: [] }>();
const ws = useWorkspace();
const ui = useUi();
const openRequest = useOpenRequest();
const tab = ref(props.initialTab ?? 'settings');
const matrixDirty = ref(false);

/** Closes the dialog, asking first when the permission matrix has unsaved changes. */
async function requestClose() {
  if (matrixDirty.value) {
    const discard = await ui.confirm({
      title: 'Discard permission matrix changes?',
      message: 'You changed roles, ticked requests or expectations without saving.',
      confirmLabel: 'Discard',
      danger: true,
    });
    if (!discard) return;
  }
  emit('close');
}

const form = ref(clone(props.collection));
if (!form.value.auth) form.value.auth = { type: 'none' };
if (!form.value.variables) form.value.variables = [];
form.value.preRequestScript ??= '';
form.value.postResponseScript ??= '';
provideCollectionVariables(toRef(() => form.value.variables));
const required = ref((props.collection.requiredVariables ?? []).join(', '));
const applyToAll = ref(false);
const saving = ref(false);
const original = JSON.stringify(form.value);
const settingsChanged = computed(() => JSON.stringify(form.value) !== original);
const tree = computed(() => ws.trees[props.collection.id]);

/** Opens an endpoint from the Endpoints tab, then closes the dialog. */
async function openEndpoint(r: Request) {
  if (matrixDirty.value) {
    const discard = await ui.confirm({
      title: 'Discard permission matrix changes?',
      message: 'You changed the permission matrix without saving.',
      confirmLabel: 'Discard',
      danger: true,
    });
    if (!discard) return;
  }
  if (settingsChanged.value) {
    const discard = await ui.confirm({
      title: 'Discard collection settings changes?',
      message: "You changed this collection's settings without saving.",
      confirmLabel: 'Discard',
      danger: true,
    });
    if (!discard) return;
  }
  if (await openRequest(r)) emit('close');
}

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
  <ModalFrame :title="collection.name" wide :xwide="tab === 'permissions'" @close="requestClose">
    <nav class="tabs" role="tablist">
      <button :class="{ active: tab === 'endpoints' }" role="tab" @click="tab = 'endpoints'">
        Endpoints<span v-if="tree" class="count">{{ tree.requests.length }}</span>
      </button>
      <button :class="{ active: tab === 'times' }" role="tab" @click="tab = 'times'">Response times</button>
      <button :class="{ active: tab === 'permissions' }" role="tab" @click="tab = 'permissions'">Permissions</button>
      <button :class="{ active: tab === 'settings' }" role="tab" @click="tab = 'settings'">Settings &amp; auth</button>
    </nav>

    <template v-if="tab === 'endpoints'">
      <CollectionEndpoints v-if="tree" :tree="tree" @open="openEndpoint" />
      <p v-else class="muted small">Loading…</p>
    </template>

    <template v-else-if="tab === 'times'">
      <CollectionResponseTimes v-if="tree" :tree="tree" @open="openEndpoint" />
      <p v-else class="muted small">Loading…</p>
    </template>

    <template v-else-if="tab === 'permissions'">
      <PermissionMatrix v-if="tree" :tree="tree" @dirty="matrixDirty = $event" @open="openEndpoint" />
      <p v-else class="muted small">Loading…</p>
    </template>

    <template v-else>
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

      <h3 class="section-title">Scripts</h3>
      <ScriptEditor v-model:pre="form.preRequestScript!" v-model:post="form.postResponseScript!" scope="collection" />
    </template>

    <template #footer>
      <span v-if="tab !== 'settings' && settingsChanged" class="muted small grow">Settings have unsaved changes.</span>
      <button @click="requestClose">{{ tab === 'settings' || settingsChanged ? 'Cancel' : 'Close' }}</button>
      <button v-if="tab === 'settings' || settingsChanged" class="primary" :disabled="saving" @click="save">
        Save
      </button>
    </template>
  </ModalFrame>
</template>

<style scoped>
.apply {
  align-items: flex-start;
}
</style>

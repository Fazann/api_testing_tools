<script setup lang="ts">
import { computed, onMounted, ref, toRef } from 'vue';

import BaseButton from '@/components/base/BaseButton.vue';
import CheckboxField from '@/components/base/CheckboxField.vue';
import ModalFrame from '@/components/base/ModalFrame.vue';
import TabBar from '@/components/base/TabBar.vue';
import TextField from '@/components/base/TextField.vue';
import CollectionResponseTimes from '@/components/performance/CollectionResponseTimes.vue';
import AuthEditor from '@/components/request/AuthEditor.vue';
import ScriptEditor from '@/components/request/ScriptEditor.vue';
import PermissionMatrix from '@/components/security/PermissionMatrix.vue';
import VariableTable from '@/components/variables/VariableTable.vue';
import { useAsyncAction } from '@/composables/useAsyncAction';
import { provideCollectionVariables } from '@/services/collectionVariables';
import { useOpenRequest } from '@/services/navigation';
import { useUi } from '@/stores/ui';
import { useWorkspace } from '@/stores/workspace';
import type { Collection, Request } from '@/types';
import { clone } from '@/utils/object';

import CollectionEndpoints from './CollectionEndpoints.vue';
import CollectionRules from './CollectionRules.vue';
import EnvironmentCompare from './EnvironmentCompare.vue';

type Tab = 'endpoints' | 'times' | 'compare' | 'rules' | 'permissions' | 'settings';
const props = defineProps<{ collection: Collection; initialTab?: Tab }>();
const emit = defineEmits<{ close: [] }>();
const ws = useWorkspace();
const ui = useUi();
const openRequest = useOpenRequest();
const tab = ref<Tab>(props.initialTab ?? 'settings');
const matrixDirty = ref(false);
const rulesDirty = ref(false);

/** Asks before discarding unsaved changes in the permission matrix or the rules; true to go ahead. */
async function discardUnsaved(): Promise<boolean> {
  const pending = [
    matrixDirty.value && 'the permission matrix (roles, ticked requests or expectations)',
    rulesDirty.value && 'the rules',
  ].filter(Boolean);
  if (pending.length === 0) return true;
  return ui.confirm({
    title: 'Discard unsaved changes?',
    message: `You changed ${pending.join(' and ')} without saving.`,
    confirmLabel: 'Discard',
    danger: true,
  });
}

/** Closes the dialog, asking first when there are unsaved changes. */
async function requestClose() {
  if (await discardUnsaved()) emit('close');
}

const form = ref(clone(props.collection));
if (!form.value.auth) form.value.auth = { type: 'none' };
if (!form.value.variables) form.value.variables = [];
form.value.preRequestScript ??= '';
form.value.postResponseScript ??= '';
provideCollectionVariables(toRef(() => form.value.variables));
const required = ref((props.collection.requiredVariables ?? []).join(', '));
const applyToAll = ref(false);
const { busy: saving, run: runSaving } = useAsyncAction();
const original = JSON.stringify(form.value);
const settingsChanged = computed(() => JSON.stringify(form.value) !== original);
const tree = computed(() => ws.trees[props.collection.id]);

/** Opens an endpoint from the Endpoints tab, then closes the dialog. */
async function openEndpoint(r: Request) {
  if (!(await discardUnsaved())) return;
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
  await runSaving(async () => {
    await ws.updateCollection(form.value);
    let message = 'Collection saved';
    if (applyToAll.value) {
      const n = await ws.inheritCollectionAuth(form.value.id);
      message += `; ${n} request${n === 1 ? '' : 's'} now use the collection auth`;
    }
    ui.toast(message, 'success');
    emit('close');
  });
}

const tabs = computed(() => [
  { id: 'endpoints' as const, label: 'Endpoints', count: tree.value?.requests.length },
  { id: 'times' as const, label: 'Response times' },
  { id: 'compare' as const, label: 'Compare environments' },
  { id: 'rules' as const, label: 'Rules', count: tree.value?.collection.rules?.length || undefined },
  { id: 'permissions' as const, label: 'Permissions' },
  { id: 'settings' as const, label: 'Settings & auth' },
]);
</script>

<template>
  <ModalFrame
    :title="collection.name"
    wide
    :xwide="tab === 'permissions' || tab === 'compare' || tab === 'rules'"
    @close="requestClose"
  >
    <TabBar v-model="tab" :tabs="tabs" />

    <template v-if="tab === 'endpoints'">
      <CollectionEndpoints v-if="tree" :tree="tree" @open="openEndpoint" />
      <p v-else class="muted small">Loading…</p>
    </template>

    <template v-else-if="tab === 'times'">
      <CollectionResponseTimes v-if="tree" :tree="tree" @open="openEndpoint" />
      <p v-else class="muted small">Loading…</p>
    </template>

    <template v-else-if="tab === 'compare'">
      <EnvironmentCompare v-if="tree" :tree="tree" @open="openEndpoint" />
      <p v-else class="muted small">Loading…</p>
    </template>

    <template v-else-if="tab === 'rules'">
      <CollectionRules v-if="tree" :tree="tree" @dirty="rulesDirty = $event" />
      <p v-else class="muted small">Loading…</p>
    </template>

    <template v-else-if="tab === 'permissions'">
      <PermissionMatrix v-if="tree" :tree="tree" @dirty="matrixDirty = $event" @open="openEndpoint" />
      <p v-else class="muted small">Loading…</p>
    </template>

    <template v-else>
      <TextField v-model="form.name" label="Name" maxlength="200" />
      <TextField v-model="form.description" label="Description" multiline rows="3" />

      <h3 class="section-title">Variables</h3>
      <p class="muted small">
        Every request in this collection can use these, for example <code v-pre>{{ base_url }}</code> or
        <code v-pre>{{ token }}</code
        >. Change a value once here and all requests follow. A variable with the same name in the selected environment
        takes precedence.
      </p>
      <VariableTable :variables="form.variables" />

      <TextField
        v-model="required"
        label="Required variables"
        input-class="mono"
        placeholder="base_url, token"
        hint="The collection will not run unless these are set and non-empty, here or in the selected environment."
      />

      <h3 class="section-title">Authorization</h3>
      <p class="muted small">
        Set auth once here, for example Bearer <code v-pre>{{ token }}</code
        >. Requests whose Auth is “Inherit auth from collection” use it; any request can override it.
      </p>
      <AuthEditor :auth="form.auth" />
      <CheckboxField v-if="ownAuth.length > 0" v-model="applyToAll" class="apply">
        <strong>Use this auth for all requests</strong> — {{ ownAuth.length }} request{{
          ownAuth.length === 1 ? ' has' : 's have'
        }}
        their own auth ({{
          ownAuth
            .slice(0, 3)
            .map((r) => r.name)
            .join(', ')
        }}{{ ownAuth.length > 3 ? ', …' : '' }}) and will be switched to inherit.
      </CheckboxField>
      <p v-else-if="ws.trees[collection.id]" class="muted small">All requests in this collection use this auth.</p>

      <h3 class="section-title">Scripts</h3>
      <ScriptEditor v-model:pre="form.preRequestScript!" v-model:post="form.postResponseScript!" scope="collection" />
    </template>

    <template #footer>
      <span v-if="tab !== 'settings' && settingsChanged" class="muted small grow">Settings have unsaved changes.</span>
      <BaseButton @click="requestClose">{{ tab === 'settings' || settingsChanged ? 'Cancel' : 'Close' }}</BaseButton>
      <BaseButton v-if="tab === 'settings' || settingsChanged" variant="primary" :disabled="saving" @click="save">
        Save
      </BaseButton>
    </template>
  </ModalFrame>
</template>

<style scoped>
.apply {
  align-items: flex-start;
}
</style>

<script setup lang="ts">
import { ref } from 'vue';

import { clone } from '../services/model';
import { useUi } from '../stores/ui';
import { useWorkspace } from '../stores/workspace';
import type { Collection } from '../types';
import AuthEditor from './AuthEditor.vue';
import ModalFrame from './ModalFrame.vue';

const props = defineProps<{ collection: Collection }>();
const emit = defineEmits<{ close: [] }>();
const ws = useWorkspace();
const ui = useUi();

const form = ref(clone(props.collection));
if (!form.value.auth) form.value.auth = { type: 'none' };
const required = ref((props.collection.requiredVariables ?? []).join(', '));

async function save() {
  form.value.requiredVariables = required.value
    .split(/[\s,]+/)
    .map((s) => s.trim())
    .filter(Boolean);
  try {
    await ws.updateCollection(form.value);
    ui.toast('Collection saved', 'success');
    emit('close');
  } catch (e) {
    ui.error(e);
  }
}
</script>

<template>
  <ModalFrame title="Collection settings" wide @close="emit('close')">
    <label class="field"><span>Name</span><input v-model="form.name" maxlength="200" /></label>
    <label class="field"><span>Description</span><textarea v-model="form.description" rows="3" /></label>
    <label class="field">
      <span>Required environment variables</span>
      <input v-model="required" class="mono" placeholder="base_url, token" />
      <span class="muted small"
        >The collection will not run unless these variables are set and non-empty in the selected environment.</span
      >
    </label>
    <h3 class="section-title">Authorization</h3>
    <p class="muted small">
      Set auth once here, for example Bearer <code v-pre>{{ token }}</code
      >. Requests whose Auth is “Inherit auth from collection” use it; any request can override it.
    </p>
    <AuthEditor :auth="form.auth" />
    <template #footer>
      <button @click="emit('close')">Cancel</button>
      <button class="primary" @click="save">Save</button>
    </template>
  </ModalFrame>
</template>

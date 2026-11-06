<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';

import BaseButton from '@/components/base/BaseButton.vue';
import BaseInput from '@/components/base/BaseInput.vue';
import CheckboxField from '@/components/base/CheckboxField.vue';
import { useAsyncAction } from '@/composables/useAsyncAction';
import { useCollectionVariables } from '@/services/collectionVariables';
import { BUILTINS, describeVariable } from '@/services/variables';
import { useEnvironments } from '@/stores/environments';
import { useUi } from '@/stores/ui';
import { useWorkspace } from '@/stores/workspace';
import type { Variable } from '@/types';
import { clone } from '@/utils/object';

type Target = 'environment' | 'collection';

const props = defineProps<{ name: string; x: number; y: number }>();
const emit = defineEmits<{ enter: []; leave: []; close: [] }>();
const envs = useEnvironments();
const ws = useWorkspace();
const ui = useUi();
const scope = useCollectionVariables();

const info = computed(() => describeVariable(props.name, envs.selected, scope.variables.value));
/** The collection a new or changed collection variable is saved to. */
const collection = computed(() => (scope.editing ? null : ws.currentCollection));
const canUseCollection = computed(() => scope.editing !== null || collection.value !== null);
const sourceLabel = computed(() =>
  info.value.source === 'collection' ? 'Collection' : (envs.selected?.name ?? 'Environment'),
);
const reveal = ref(false);
const editing = ref<Target | null>(null);
const draft = ref('');
const secret = ref(false);
const { busy: saving, run: runSaving } = useAsyncAction();
const valueInput = ref<InstanceType<typeof BaseInput> | null>(null);

watch(
  () => props.name,
  () => {
    reveal.value = false;
    editing.value = null;
  },
);

const displayValue = computed(() => {
  const v = info.value.variable;
  if (!v) return '';
  if (v.value === '') return '(empty)';
  return v.secret && !reveal.value ? '•'.repeat(Math.min(v.value.length, 12)) : v.value;
});

function startEdit(target: Target) {
  draft.value = info.value.variable?.value ?? '';
  secret.value = info.value.variable?.secret ?? /token|secret|password|key/i.test(props.name);
  editing.value = target;
  nextTick(() => valueInput.value?.focus());
}

/** Sets the variable in list, adding it when absent. */
function upsert(list: Variable[]) {
  const existing = list.find((v) => v.key === props.name);
  if (existing) {
    existing.value = draft.value;
    existing.secret = secret.value;
    existing.enabled = true;
  } else {
    list.push({ key: props.name, value: draft.value, secret: secret.value, enabled: true });
  }
}

async function save() {
  if (editing.value === 'collection' && scope.editing) {
    // The collection settings form is saved by its own Save button.
    upsert(scope.editing.value);
    editing.value = null;
    return;
  }
  await runSaving(async () => {
    if (editing.value === 'collection' && collection.value) {
      const updated = clone(collection.value);
      updated.variables = updated.variables ?? [];
      upsert(updated.variables);
      await ws.updateCollection(updated);
      ui.toast(`Saved ${props.name} in collection ${updated.name}`, 'success');
    } else if (editing.value === 'environment' && envs.selected) {
      const updated = clone(envs.selected);
      upsert(updated.variables);
      await envs.save(updated);
      ui.toast(`Saved ${props.name} in ${updated.name}`, 'success');
    }
    editing.value = null;
  });
}

// Keep the popover inside the window.
const style = computed(() => ({
  left: `${Math.max(8, Math.min(props.x, window.innerWidth - 328))}px`,
  top: `${Math.min(props.y, window.innerHeight - 220)}px`,
}));
</script>

<template>
  <Teleport to="body">
    <div
      class="var-popover"
      :style="style"
      role="dialog"
      :aria-label="`Variable ${name}`"
      @mouseenter="emit('enter')"
      @mouseleave="emit('leave')"
      @keydown.esc.stop="emit('close')"
    >
      <div class="row head">
        <code class="name">{{ name }}</code>
        <span class="spacer" />
        <span class="state" :class="info.state">
          {{
            {
              builtin: 'Built-in',
              defined: sourceLabel,
              empty: sourceLabel,
              disabled: 'Disabled',
              missing: 'Unresolved',
            }[info.state]
          }}
        </span>
      </div>

      <p v-if="info.state === 'builtin'" class="muted small">{{ BUILTINS[name] }}</p>

      <template v-else-if="editing">
        <p class="muted small">
          {{ editing === 'collection' ? 'Collection variable, used by every request' : `In ${envs.selected?.name}` }}
        </p>
        <BaseInput
          v-model="draft"
          class="mono"
          :type="secret && !reveal ? 'password' : 'text'"
          :aria-label="`Value of ${name}`"
          autocomplete="off"
          spellcheck="false"
          ref="valueInput"
          @keydown.enter.prevent="save"
        />
        <div class="row small">
          <CheckboxField v-model="secret" label="Secret" />
          <BaseButton v-if="secret" variant="ghost" small @click="reveal = !reveal">
            {{ reveal ? 'Hide' : 'Show' }}
          </BaseButton>
          <span class="spacer" />
          <BaseButton small @click="editing = null">Cancel</BaseButton>
          <BaseButton variant="primary" small :disabled="saving" @click="save">Save</BaseButton>
        </div>
      </template>

      <template v-else-if="info.variable">
        <div class="row value-row">
          <code class="value grow" :class="{ muted: info.variable.value === '' }">{{ displayValue }}</code>
          <BaseButton
            v-if="info.variable.secret && info.variable.value"
            variant="ghost"
            small
            @click="reveal = !reveal"
          >
            {{ reveal ? 'Hide' : 'Show' }}
          </BaseButton>
        </div>
        <p v-if="info.source === 'collection'" class="muted small">
          Collection variable: every request in the collection uses it unless the environment overrides it.
        </p>
        <p v-if="info.state === 'disabled'" class="warn small">
          This variable is disabled, so the reference will not be replaced.
        </p>
        <p v-else-if="info.state === 'empty'" class="warn small">The value is empty.</p>
        <p v-if="info.variable.secret" class="muted small">Secret: redacted from history, reports and exports.</p>
        <div class="row">
          <span class="spacer" />
          <BaseButton
            v-if="info.source === 'environment' || canUseCollection"
            small
            @click="startEdit(info.source ?? 'environment')"
          >
            {{ info.state === 'disabled' ? 'Enable and edit' : 'Edit' }}
          </BaseButton>
        </div>
      </template>

      <template v-else>
        <p class="small">
          <strong class="fail">{{ name }}</strong> is not defined
          <template v-if="envs.selected">
            in <strong>{{ envs.selected.name }}</strong></template
          >
          <template v-if="canUseCollection"> or in the collection</template>. The request will fail until it is.
        </p>
        <p v-if="!envs.selected && !canUseCollection" class="muted small">
          Choose an environment in the top bar, or create one under Environments.
        </p>
        <div class="row">
          <span class="spacer" />
          <BaseButton v-if="canUseCollection" small @click="startEdit('collection')">Add to collection</BaseButton>
          <BaseButton v-if="envs.selected" variant="primary" small @click="startEdit('environment')">
            Add to {{ envs.selected.name }}
          </BaseButton>
        </div>
      </template>
    </div>
  </Teleport>
</template>

<style scoped>
.var-popover {
  position: fixed;
  z-index: 60;
  width: 320px;
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  background: var(--bg);
  border: 1px solid var(--line);
  border-radius: 8px;
  box-shadow: var(--shadow-md);
}
.var-popover p {
  margin: 0;
}
.name {
  font-weight: 700;
  word-break: break-all;
}
.state {
  font-size: 11px;
  font-weight: 600;
  white-space: nowrap;
  max-width: 150px;
  overflow: hidden;
  text-overflow: ellipsis;
}
.state.defined,
.state.builtin {
  color: var(--pass);
}
.state.empty,
.state.disabled {
  color: var(--warn);
}
.state.missing {
  color: var(--fail);
}
.value {
  font-family: var(--mono);
  padding: 4px 6px;
  background: var(--bg-subtle);
  border: 1px solid var(--line);
  border-radius: 4px;
  word-break: break-all;
  max-height: 120px;
  overflow: auto;
}
</style>

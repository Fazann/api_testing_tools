<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';

import { clone } from '../services/model';
import { BUILTINS, describeVariable } from '../services/variables';
import { useEnvironments } from '../stores/environments';
import { useUi } from '../stores/ui';

const props = defineProps<{ name: string; x: number; y: number }>();
const emit = defineEmits<{ enter: []; leave: []; close: [] }>();
const envs = useEnvironments();
const ui = useUi();

const info = computed(() => describeVariable(props.name, envs.selected));
const reveal = ref(false);
const editing = ref(false);
const draft = ref('');
const secret = ref(false);
const saving = ref(false);
const valueInput = ref<HTMLInputElement | null>(null);

watch(
  () => props.name,
  () => {
    reveal.value = false;
    editing.value = false;
  },
);

const displayValue = computed(() => {
  const v = info.value.variable;
  if (!v) return '';
  if (v.value === '') return '(empty)';
  return v.secret && !reveal.value ? '•'.repeat(Math.min(v.value.length, 12)) : v.value;
});

function startEdit() {
  draft.value = info.value.variable?.value ?? '';
  secret.value = info.value.variable?.secret ?? /token|secret|password|key/i.test(props.name);
  editing.value = true;
  nextTick(() => valueInput.value?.focus());
}

async function save() {
  const env = envs.selected;
  if (!env) return;
  const updated = clone(env);
  const existing = updated.variables.find((v) => v.key === props.name);
  if (existing) {
    existing.value = draft.value;
    existing.secret = secret.value;
    existing.enabled = true;
  } else {
    updated.variables.push({ key: props.name, value: draft.value, secret: secret.value, enabled: true });
  }
  saving.value = true;
  try {
    await envs.save(updated);
    editing.value = false;
    ui.toast(`Saved ${props.name} in ${env.name}`, 'success');
  } catch (e) {
    ui.error(e);
  } finally {
    saving.value = false;
  }
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
              defined: envs.selected?.name,
              empty: envs.selected?.name,
              disabled: 'Disabled',
              missing: 'Unresolved',
            }[info.state]
          }}
        </span>
      </div>

      <p v-if="info.state === 'builtin'" class="muted small">{{ BUILTINS[name] }}</p>

      <template v-else-if="!envs.selected">
        <p class="small">No environment is selected, so this variable cannot be resolved.</p>
        <p class="muted small">Choose an environment in the top bar, or create one under Environments.</p>
      </template>

      <template v-else-if="editing">
        <input
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
          <label class="row"><input v-model="secret" type="checkbox" /> Secret</label>
          <button v-if="secret" class="ghost small" type="button" @click="reveal = !reveal">
            {{ reveal ? 'Hide' : 'Show' }}
          </button>
          <span class="spacer" />
          <button class="small" type="button" @click="editing = false">Cancel</button>
          <button class="primary small" type="button" :disabled="saving" @click="save">Save</button>
        </div>
      </template>

      <template v-else-if="info.variable">
        <div class="row value-row">
          <code class="value grow" :class="{ muted: info.variable.value === '' }">{{ displayValue }}</code>
          <button v-if="info.variable.secret && info.variable.value" class="ghost small" @click="reveal = !reveal">
            {{ reveal ? 'Hide' : 'Show' }}
          </button>
        </div>
        <p v-if="info.state === 'disabled'" class="warn small">
          This variable is disabled in {{ envs.selected.name }}, so the reference will not be replaced.
        </p>
        <p v-else-if="info.state === 'empty'" class="warn small">The value is empty.</p>
        <p v-if="info.variable.secret" class="muted small">Secret: redacted from history, reports and exports.</p>
        <div class="row">
          <span class="spacer" />
          <button class="small" @click="startEdit">{{ info.state === 'disabled' ? 'Enable and edit' : 'Edit' }}</button>
        </div>
      </template>

      <template v-else>
        <p class="small">
          <strong class="fail">{{ name }}</strong> is not defined in <strong>{{ envs.selected.name }}</strong
          >. The request will fail until it is.
        </p>
        <div class="row">
          <span class="spacer" />
          <button class="primary small" @click="startEdit">Add to {{ envs.selected.name }}</button>
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
  box-shadow: 0 8px 28px #0003;
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

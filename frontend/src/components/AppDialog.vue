<script setup lang="ts">
import { nextTick, ref, watch } from 'vue';

import { useUi } from '../stores/ui';

const ui = useUi();
const input = ref<HTMLInputElement | null>(null);
const confirmBtn = ref<HTMLButtonElement | null>(null);

watch(
  () => ui.dialog,
  async (d) => {
    if (!d) return;
    await nextTick();
    if (d.type === 'prompt') {
      input.value?.focus();
      input.value?.select();
    } else if (!d.danger) {
      confirmBtn.value?.focus();
    }
  },
);

function submit() {
  const d = ui.dialog;
  if (!d) return;
  if (d.type === 'prompt') {
    const v = d.value.trim();
    if (!v) return;
    ui.closeDialog(v);
  } else {
    ui.closeDialog(true);
  }
}

function cancel() {
  ui.closeDialog(ui.dialog?.type === 'prompt' ? null : false);
}
</script>

<template>
  <div v-if="ui.dialog" class="backdrop" @mousedown.self="cancel" @keydown.esc="cancel">
    <form class="dialog" role="dialog" aria-modal="true" :aria-label="ui.dialog.title" @submit.prevent="submit">
      <h2>{{ ui.dialog.title }}</h2>
      <p v-if="ui.dialog.message" class="muted">{{ ui.dialog.message }}</p>
      <ul v-if="ui.dialog.details?.length" class="details">
        <li v-for="(d, i) in ui.dialog.details" :key="i">{{ d }}</li>
      </ul>
      <input
        v-if="ui.dialog.type === 'prompt'"
        ref="input"
        v-model="ui.dialog.value"
        maxlength="200"
        @keydown.esc.stop="cancel"
      />
      <div class="row actions">
        <button type="button" @click="cancel">Cancel</button>
        <button ref="confirmBtn" type="submit" :class="ui.dialog.danger ? 'danger' : 'primary'">
          {{ ui.dialog.confirmLabel }}
        </button>
      </div>
    </form>
  </div>
</template>

<style scoped>
.backdrop {
  position: fixed;
  inset: 0;
  background: #0006;
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 40;
  padding: 16px;
}
.dialog {
  background: var(--bg);
  border: 1px solid var(--line);
  border-radius: 10px;
  padding: 18px;
  width: min(480px, 100%);
  display: flex;
  flex-direction: column;
  gap: 10px;
  box-shadow: 0 12px 40px #0004;
}
.dialog h2 {
  margin: 0;
  font-size: 16px;
}
.dialog p {
  margin: 0;
}
.details {
  margin: 0;
  padding-left: 18px;
  max-height: 240px;
  overflow: auto;
}
.details li {
  margin-bottom: 4px;
}
.actions {
  justify-content: flex-end;
  margin-top: 6px;
}
</style>

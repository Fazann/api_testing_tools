<script setup lang="ts">
import { nextTick, ref, watch } from 'vue';

import BaseButton from '@/components/base/BaseButton.vue';
import BaseInput from '@/components/base/BaseInput.vue';
import ModalFrame from '@/components/base/ModalFrame.vue';
import { useUi } from '@/stores/ui';

const FORM_ID = 'app-dialog-form';

const ui = useUi();
const input = ref<InstanceType<typeof BaseInput> | null>(null);
const confirmBtn = ref<{ focus(): void } | null>(null);

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
  <ModalFrame v-if="ui.dialog" :title="ui.dialog.title" stacked @close="cancel">
    <form :id="FORM_ID" class="stack" @submit.prevent="submit">
      <p v-if="ui.dialog.message" class="muted">{{ ui.dialog.message }}</p>
      <ul v-if="ui.dialog.details?.length" class="details">
        <li v-for="(d, i) in ui.dialog.details" :key="i">{{ d }}</li>
      </ul>
      <BaseInput
        v-if="ui.dialog.type === 'prompt'"
        ref="input"
        v-model="ui.dialog.value"
        :aria-label="ui.dialog.title"
        maxlength="200"
        @keydown.esc.stop="cancel"
      />
    </form>
    <template #footer>
      <BaseButton @click="cancel">Cancel</BaseButton>
      <BaseButton ref="confirmBtn" type="submit" :form="FORM_ID" :variant="ui.dialog.danger ? 'danger' : 'primary'">
        {{ ui.dialog.confirmLabel }}
      </BaseButton>
    </template>
  </ModalFrame>
</template>

<style scoped>
.stack {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
p {
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
</style>

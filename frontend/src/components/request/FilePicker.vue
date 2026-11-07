<script setup lang="ts">
import { computed } from 'vue';

import { api } from '@/api';
import BaseButton from '@/components/base/BaseButton.vue';
import { useUi } from '@/stores/ui';

/** A chosen upload file: shows its name (full path on hover) with Choose and Clear buttons. */
const props = defineProps<{ label?: string }>();
const path = defineModel<string | undefined>({ required: true });
const ui = useUi();

const name = computed(() => {
  const p = path.value ?? '';
  return p.split(/[\\/]/).pop() || p;
});

async function choose() {
  const chosen = await api.chooseUploadFile().catch(ui.error);
  if (chosen) path.value = chosen;
}
</script>

<template>
  <div class="picker">
    <span v-if="path" class="file mono" :title="path">{{ name }}</span>
    <span v-else class="muted small">No file chosen</span>
    <BaseButton small :aria-label="props.label ? `Choose ${props.label}` : 'Choose file'" @click="choose">
      {{ path ? 'Change…' : 'Choose file…' }}
    </BaseButton>
    <BaseButton
      v-if="path"
      variant="ghost"
      small
      :aria-label="props.label ? `Clear ${props.label}` : 'Clear file'"
      @click="path = ''"
      >✕</BaseButton
    >
  </div>
</template>

<style scoped>
.picker {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}
.file {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
  flex: 1;
}
</style>

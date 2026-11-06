<script setup lang="ts">
import BaseButton from '@/components/base/BaseButton.vue';
import { useUi } from '@/stores/ui';

const ui = useUi();
</script>

<template>
  <div class="toasts" role="status" aria-live="polite">
    <div v-for="t in ui.toasts" :key="t.id" class="toast" :class="t.kind">
      <span class="grow">{{ t.text }}</span>
      <BaseButton variant="ghost" aria-label="Dismiss" @click="ui.dismiss(t.id)">✕</BaseButton>
    </div>
  </div>
</template>

<style scoped>
.toasts {
  position: fixed;
  right: 16px;
  bottom: 16px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  z-index: 50;
  max-width: min(420px, calc(100vw - 32px));
}
.toast {
  display: flex;
  gap: 8px;
  align-items: flex-start;
  padding: 8px 10px;
  border-radius: var(--radius);
  background: var(--bg-subtle);
  border: 1px solid var(--line);
  border-left: 3px solid var(--accent);
  box-shadow: var(--shadow-sm);
  word-break: break-word;
}
.toast.success {
  border-left-color: var(--pass);
}
.toast.error {
  border-left-color: var(--fail);
}
</style>

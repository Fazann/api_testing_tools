<script setup lang="ts">
import BaseButton from './BaseButton.vue';

// `stacked` raises the dialog above other open modals (confirm and prompt dialogs).
defineProps<{ title: string; wide?: boolean; xwide?: boolean; stacked?: boolean }>();
const emit = defineEmits<{ close: [] }>();
</script>

<template>
  <div class="backdrop" :class="{ stacked }" @mousedown.self="emit('close')" @keydown.esc="emit('close')">
    <section class="modal" :class="{ wide, xwide }" role="dialog" aria-modal="true" :aria-label="title">
      <header class="row">
        <h2 class="grow">{{ title }}</h2>
        <BaseButton variant="ghost" aria-label="Close" @click="emit('close')">✕</BaseButton>
      </header>
      <div class="content"><slot /></div>
      <footer class="row"><slot name="footer" /></footer>
    </section>
  </div>
</template>

<style scoped>
.backdrop {
  position: fixed;
  inset: 0;
  background: var(--backdrop);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 35;
  padding: 16px;
}
.backdrop.stacked {
  z-index: 40;
}
.modal {
  background: var(--bg);
  border: 1px solid var(--line);
  border-radius: 10px;
  width: min(520px, 100%);
  max-height: calc(100vh - 32px);
  display: flex;
  flex-direction: column;
  box-shadow: var(--shadow-lg);
}
.modal.wide {
  width: min(760px, 100%);
}
.modal.xwide {
  width: min(1180px, 100%);
}
header {
  padding: 14px 16px 6px;
}
header h2 {
  margin: 0;
  font-size: 16px;
}
.content {
  padding: 8px 16px;
  overflow: auto;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
footer {
  padding: 10px 16px 14px;
  justify-content: flex-end;
}
</style>

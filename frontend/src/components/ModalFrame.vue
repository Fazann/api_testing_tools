<script setup lang="ts">
defineProps<{ title: string; wide?: boolean }>();
const emit = defineEmits<{ close: [] }>();
</script>

<template>
  <div class="backdrop" @mousedown.self="emit('close')" @keydown.esc="emit('close')">
    <section class="modal" :class="{ wide }" role="dialog" aria-modal="true" :aria-label="title">
      <header class="row">
        <h2 class="grow">{{ title }}</h2>
        <button class="ghost" aria-label="Close" @click="emit('close')">✕</button>
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
  background: #0006;
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 35;
  padding: 16px;
}
.modal {
  background: var(--bg);
  border: 1px solid var(--line);
  border-radius: 10px;
  width: min(520px, 100%);
  max-height: calc(100vh - 32px);
  display: flex;
  flex-direction: column;
  box-shadow: 0 12px 40px #0004;
}
.modal.wide {
  width: min(760px, 100%);
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

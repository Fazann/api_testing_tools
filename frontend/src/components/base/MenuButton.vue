<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue';

export interface MenuItem {
  label: string;
  action: () => void;
  danger?: boolean;
}

defineProps<{ items: MenuItem[]; label?: string }>();
const open = ref(false);
const root = ref<HTMLElement | null>(null);

function onDocClick(e: MouseEvent) {
  if (root.value && !root.value.contains(e.target as Node)) close();
}
function toggle() {
  open.value = !open.value;
  if (open.value) document.addEventListener('mousedown', onDocClick);
  else document.removeEventListener('mousedown', onDocClick);
}
function close() {
  open.value = false;
  document.removeEventListener('mousedown', onDocClick);
}
function pick(item: MenuItem) {
  close();
  item.action();
}
onBeforeUnmount(close);
</script>

<template>
  <span ref="root" class="menu" @keydown.esc="close">
    <button
      class="ghost"
      :aria-label="label ?? 'More actions'"
      aria-haspopup="menu"
      :aria-expanded="open"
      @click.stop="toggle"
    >
      ⋯
    </button>
    <div v-if="open" class="items" role="menu">
      <button
        v-for="item in items"
        :key="item.label"
        role="menuitem"
        :class="{ danger: item.danger }"
        @click.stop="pick(item)"
      >
        {{ item.label }}
      </button>
    </div>
  </span>
</template>

<style scoped>
.menu {
  position: relative;
  display: inline-flex;
}
.items {
  position: absolute;
  right: 0;
  top: 100%;
  z-index: 30;
  min-width: 170px;
  display: flex;
  flex-direction: column;
  padding: 4px;
  background: var(--bg);
  border: 1px solid var(--line);
  border-radius: var(--radius);
  box-shadow: var(--shadow-md);
}
.items button {
  text-align: left;
  border: none;
  background: transparent;
  padding: 6px 10px;
}
.items button:hover {
  background: var(--bg-muted);
}
</style>

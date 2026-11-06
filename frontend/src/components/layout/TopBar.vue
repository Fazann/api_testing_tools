<script setup lang="ts">
import { useEnvironments } from '../stores/environments';
import { useUi, type View } from '../stores/ui';

const ui = useUi();
const envs = useEnvironments();

const tabs: { id: View; label: string }[] = [
  { id: 'requests', label: 'Requests' },
  { id: 'run', label: 'Run' },
  { id: 'history', label: 'History' },
  { id: 'environments', label: 'Environments' },
  { id: 'settings', label: 'Settings' },
];
</script>

<template>
  <header class="topbar">
    <strong class="brand">API Regression Guard</strong>
    <nav class="nav" aria-label="Main">
      <button
        v-for="t in tabs"
        :key="t.id"
        :class="{ active: ui.view === t.id }"
        :aria-current="ui.view === t.id ? 'page' : undefined"
        @click="ui.view = t.id"
      >
        {{ t.label }}
      </button>
    </nav>
    <span class="spacer" />
    <label class="env row">
      <span class="muted small">Environment</span>
      <select v-model="envs.selectedId" aria-label="Environment">
        <option value="">No environment</option>
        <option v-for="e in envs.list" :key="e.id" :value="e.id">
          {{ e.name }}{{ e.protected ? ' (protected)' : '' }}
        </option>
      </select>
    </label>
    <span v-if="envs.selected?.protected" class="badge failed" title="Destructive requests require confirmation"
      >protected</span
    >
  </header>
</template>

<style scoped>
.topbar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 6px 12px;
  border-bottom: 1px solid var(--line);
  background: var(--bg-subtle);
}
.brand {
  font-size: 13px;
  white-space: nowrap;
}
.nav {
  display: flex;
  gap: 2px;
  overflow-x: auto;
}
.nav button {
  background: transparent;
  border-color: transparent;
  color: var(--fg-muted);
}
.nav button.active {
  color: var(--fg);
  background: var(--bg-muted);
}
.env select {
  max-width: 220px;
}
</style>

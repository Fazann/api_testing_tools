<script setup lang="ts">
import { computed } from 'vue';

import BaseButton from '@/components/base/BaseButton.vue';
import SelectField from '@/components/base/SelectField.vue';
import StatusBadge from '@/components/base/StatusBadge.vue';
import { useEnvironments } from '@/stores/environments';
import { useUi, type View } from '@/stores/ui';

const ui = useUi();
const envs = useEnvironments();
const envOptions = computed(() => [
  { value: '', label: 'No environment' },
  ...envs.list.map((e) => ({ value: e.id, label: e.name + (e.protected ? ' (protected)' : '') })),
]);

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
      <BaseButton
        v-for="t in tabs"
        :key="t.id"
        :class="{ active: ui.view === t.id }"
        :aria-current="ui.view === t.id ? 'page' : undefined"
        @click="ui.view = t.id"
      >
        {{ t.label }}
      </BaseButton>
    </nav>
    <span class="spacer" />
    <label class="env row">
      <span class="muted small">Environment</span>
      <SelectField v-model="envs.selectedId" :options="envOptions" aria-label="Environment" />
    </label>
    <StatusBadge
      v-if="envs.selected?.protected"
      status="failed"
      label="protected"
      title="Destructive requests require confirmation"
    />
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

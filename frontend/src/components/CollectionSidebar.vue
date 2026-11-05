<script setup lang="ts">
import { ref } from 'vue';

import { api } from '../services/api';
import { useEnvironments } from '../stores/environments';
import { useRunner } from '../stores/runner';
import { useUi } from '../stores/ui';
import { useWorkspace } from '../stores/workspace';
import type { Collection } from '../types';
import CollectionSettingsDialog from './CollectionSettingsDialog.vue';
import CurlImportDialog from './CurlImportDialog.vue';
import ExportDialog from './ExportDialog.vue';
import MenuButton from './MenuButton.vue';
import TreeFolder from './TreeFolder.vue';

const ws = useWorkspace();
const ui = useUi();
const runner = useRunner();
const envs = useEnvironments();
const exporting = ref<Collection | null>(null);
const curlTarget = ref<Collection | null>(null);
const editing = ref<{ collection: Collection; tab: 'endpoints' | 'permissions' | 'settings' } | null>(null);

async function newCollection() {
  const name = await ui.prompt({ title: 'New collection', value: 'My API tests', confirmLabel: 'Create' });
  if (name) await ws.createCollection(name).catch(ui.error);
}

async function importCollection() {
  try {
    const res = await api.importCollection();
    if (!res) return;
    ws.expanded[res.collection.id] = true;
    await ws.load();
    ui.toast(
      `Imported "${res.collection.name}": ${res.requests} requests, ${res.folders} folders, ` +
        `${res.environments} environments` +
        (res.baselines ? `, ${res.baselines} baselines` : '') +
        (res.baselinesSkipped
          ? ` (${res.baselinesSkipped} baselines skipped: their environment is not on this machine)`
          : ''),
      'success',
    );
    if (res.environments > 0) await envs.load();
  } catch (e) {
    ui.error(e);
  }
}

function menu(c: Collection) {
  return [
    { label: 'New request', action: () => ws.createRequest(c.id).catch(ui.error) },
    { label: 'New request from cURL…', action: () => (curlTarget.value = c) },
    { label: 'New folder', action: () => newFolder(c) },
    { label: 'Run collection', action: () => runner.runCollection(c.id, '', c.name) },
    { label: 'Endpoints summary…', action: () => (editing.value = { collection: c, tab: 'endpoints' }) },
    { label: 'Permission matrix…', action: () => (editing.value = { collection: c, tab: 'permissions' }) },
    { label: 'Settings & auth…', action: () => (editing.value = { collection: c, tab: 'settings' }) },
    { label: 'Duplicate', action: () => ws.duplicateCollection(c.id).catch(ui.error) },
    { label: 'Export…', action: () => (exporting.value = c) },
    { label: 'Delete', danger: true, action: () => remove(c) },
  ];
}

async function newFolder(c: Collection) {
  const name = await ui.prompt({ title: 'New folder', value: 'New folder', confirmLabel: 'Create' });
  if (name) await ws.createFolder(c.id, '', name).catch(ui.error);
}

async function remove(c: Collection) {
  const ok = await ui.confirm({
    title: `Delete collection "${c.name}"?`,
    message:
      'All folders, requests, assertions and baselines in this collection will be permanently deleted. ' +
      'Run history is kept.',
    confirmLabel: 'Delete collection',
    danger: true,
  });
  if (ok) await ws.deleteCollection(c.id).catch(ui.error);
}
</script>

<template>
  <aside class="sidebar" aria-label="Collections">
    <div class="head row">
      <strong class="grow">Collections</strong>
      <button class="ghost" title="Import a collection export or an OpenAPI / Swagger file" @click="importCollection">
        Import
      </button>
      <button class="ghost" title="New collection" aria-label="New collection" @click="newCollection">＋</button>
    </div>
    <div class="list">
      <div v-if="ws.loaded && ws.collections.length === 0" class="empty small">
        <p>No collections yet.</p>
        <button class="primary" @click="newCollection">Create a collection</button>
        <button @click="importCollection">Import collection or OpenAPI</button>
      </div>
      <div v-for="c in ws.collections" :key="c.id" class="collection">
        <div class="node">
          <button
            class="ghost label"
            :aria-expanded="!!ws.expanded[c.id]"
            @click="ws.toggle(c.id, c.id).catch(ui.error)"
          >
            <span class="caret">{{ ws.expanded[c.id] ? '▾' : '▸' }}</span>
            <strong>{{ c.name }}</strong>
          </button>
          <button
            class="ghost"
            title="Run collection"
            :aria-label="`Run ${c.name}`"
            @click="runner.runCollection(c.id, '', c.name)"
          >
            ▶
          </button>
          <MenuButton :items="menu(c)" :label="`Actions for ${c.name}`" />
        </div>
        <TreeFolder v-if="ws.expanded[c.id] && ws.trees[c.id]" :tree="ws.trees[c.id]" parent-id="" :depth="1" />
      </div>
    </div>
    <ExportDialog v-if="exporting" :collection="exporting" @close="exporting = null" />
    <CurlImportDialog v-if="curlTarget" :collection-id="curlTarget.id" @close="curlTarget = null" />
    <CollectionSettingsDialog
      v-if="editing"
      :collection="editing.collection"
      :initial-tab="editing.tab"
      @close="editing = null"
    />
  </aside>
</template>

<style scoped>
.sidebar {
  width: 280px;
  flex-shrink: 0;
  border-right: 1px solid var(--line);
  display: flex;
  flex-direction: column;
  background: var(--bg-subtle);
}
.head {
  padding: 8px 8px 8px 12px;
  border-bottom: 1px solid var(--line);
}
.list {
  flex: 1;
  overflow: auto;
  padding: 6px;
}
.node {
  display: flex;
  align-items: center;
  border-radius: var(--radius);
}
.node:hover {
  background: var(--bg-muted);
}
.label {
  flex: 1;
  min-width: 0;
  text-align: left;
  display: flex;
  gap: 6px;
  align-items: center;
  overflow: hidden;
}
.caret {
  width: 10px;
  color: var(--fg-muted);
}
.collection {
  margin-bottom: 4px;
}
.empty {
  padding: 24px 8px;
}
</style>

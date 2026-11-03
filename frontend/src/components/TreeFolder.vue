<script setup lang="ts">
import { computed } from 'vue';

import { useOpenRequest } from '../services/navigation';
import { useRunner } from '../stores/runner';
import { useUi } from '../stores/ui';
import { useWorkspace } from '../stores/workspace';
import type { CollectionTree, Folder, Request } from '../types';
import MenuButton from './MenuButton.vue';

const props = defineProps<{ tree: CollectionTree; parentId: string; depth: number }>();
const ws = useWorkspace();
const ui = useUi();
const runner = useRunner();
const openRequest = useOpenRequest();

const folders = computed(() => props.tree.folders.filter((f) => f.parentId === props.parentId));
const requests = computed(() => props.tree.requests.filter((r) => r.parentId === props.parentId));
const collectionId = computed(() => props.tree.collection.id);

function folderMenu(f: Folder) {
  return [
    { label: 'New request', action: () => ws.createRequest(collectionId.value, f.id).catch(ui.error) },
    { label: 'New folder', action: () => newFolder(f.id) },
    { label: 'Run folder', action: () => runner.runCollection(collectionId.value, f.id, f.name) },
    { label: 'Rename', action: () => renameFolder(f) },
    { label: 'Delete', danger: true, action: () => deleteFolder(f) },
  ];
}

function requestMenu(r: Request) {
  return [
    { label: 'Rename', action: () => renameRequest(r) },
    { label: 'Duplicate', action: () => ws.duplicateRequest(r).catch(ui.error) },
    { label: 'Delete', danger: true, action: () => deleteRequest(r) },
  ];
}

async function newFolder(parentId: string) {
  const name = await ui.prompt({ title: 'New folder', value: 'New folder', confirmLabel: 'Create' });
  if (name) await ws.createFolder(collectionId.value, parentId, name).catch(ui.error);
}

async function renameFolder(f: Folder) {
  const name = await ui.prompt({ title: 'Rename folder', value: f.name });
  if (name) await ws.renameFolder(collectionId.value, f.id, name).catch(ui.error);
}

async function deleteFolder(f: Folder) {
  const ok = await ui.confirm({
    title: `Delete folder "${f.name}"?`,
    message: 'All requests, assertions and baselines inside it will be deleted. Run history is kept.',
    confirmLabel: 'Delete',
    danger: true,
  });
  if (ok) await ws.deleteFolder(collectionId.value, f.id).catch(ui.error);
}

async function renameRequest(r: Request) {
  const name = await ui.prompt({ title: 'Rename request', value: r.name });
  if (name) await ws.renameRequest(r, name).catch(ui.error);
}

async function deleteRequest(r: Request) {
  const ok = await ui.confirm({
    title: `Delete request "${r.name}"?`,
    message: 'Its assertions and baselines will be deleted. Run history is kept.',
    confirmLabel: 'Delete',
    danger: true,
  });
  if (ok) await ws.deleteRequest(r).catch(ui.error);
}
</script>

<template>
  <ul class="tree" role="group">
    <li v-for="f in folders" :key="f.id">
      <div class="node" :style="{ paddingLeft: depth * 14 + 6 + 'px' }">
        <button class="ghost label" :aria-expanded="!!ws.expanded[f.id]" @click="ws.toggle(f.id)">
          <span class="caret">{{ ws.expanded[f.id] ? '▾' : '▸' }}</span> 📁 {{ f.name }}
        </button>
        <MenuButton :items="folderMenu(f)" :label="`Actions for folder ${f.name}`" />
      </div>
      <TreeFolder v-if="ws.expanded[f.id]" :tree="tree" :parent-id="f.id" :depth="depth + 1" />
    </li>
    <li v-for="r in requests" :key="r.id">
      <div class="node" :class="{ active: ws.draft?.id === r.id }" :style="{ paddingLeft: depth * 14 + 6 + 'px' }">
        <button class="ghost label" @click="openRequest(r)">
          <span class="method" :class="r.method">{{ r.method }}</span>
          <span class="name">{{ r.name }}</span>
          <span
            v-if="ws.results[r.id]"
            class="dot"
            :class="ws.results[r.id].status"
            :title="`Last run: ${ws.results[r.id].status}`"
          />
        </button>
        <MenuButton :items="requestMenu(r)" :label="`Actions for request ${r.name}`" />
      </div>
    </li>
    <li
      v-if="folders.length === 0 && requests.length === 0"
      class="muted small none"
      :style="{ paddingLeft: depth * 14 + 12 + 'px' }"
    >
      Empty
    </li>
  </ul>
</template>

<style scoped>
.tree {
  list-style: none;
  margin: 0;
  padding: 0;
}
.node {
  display: flex;
  align-items: center;
  border-radius: var(--radius);
  padding-right: 2px;
}
.node:hover,
.node.active {
  background: var(--bg-muted);
}
.node.active .name {
  font-weight: 600;
}
.label {
  flex: 1;
  min-width: 0;
  display: flex;
  gap: 6px;
  align-items: center;
  text-align: left;
  overflow: hidden;
}
.label .name {
  overflow: hidden;
  text-overflow: ellipsis;
}
.caret {
  width: 10px;
  display: inline-block;
  color: var(--fg-muted);
}
.method {
  width: 52px;
  flex-shrink: 0;
}
.dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  flex-shrink: 0;
  margin-left: auto;
  background: var(--warn);
}
.dot.passed {
  background: var(--pass);
}
.dot.failed {
  background: var(--fail);
}
.none {
  padding: 3px 0;
}
</style>

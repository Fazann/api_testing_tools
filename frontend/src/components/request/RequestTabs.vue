<script setup lang="ts">
import { nextTick, ref, watch } from 'vue';

import MethodBadge from '@/components/base/MethodBadge.vue';
import { useCloseTab } from '@/composables/useCloseTab';
import { requestBadge } from '@/services/socket';
import { useRunner } from '@/stores/runner';
import { useWorkspace, type OpenTab } from '@/stores/workspace';

// One tab per open request. Opening or creating a request appends a tab and
// keeps the others; every tab can be closed (middle-click and Ctrl+W work too).
// Open tabs are restored on the next start (see restoreTabs in the workspace store).
const ws = useWorkspace();
const runner = useRunner();
const strip = ref<HTMLElement | null>(null);

const closeTab = useCloseTab();
const close = (t: OpenTab) => closeTab(t.key);

function collectionName(t: OpenTab) {
  return ws.collections.find((c) => c.id === t.draft.collectionId)?.name ?? '';
}

// Keep the active tab visible when there are more tabs than fit.
watch(
  () => ws.activeKey,
  async (key) => {
    await nextTick();
    strip.value
      ?.querySelector<HTMLElement>(`[data-key="${key}"]`)
      ?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' });
  },
);
</script>

<template>
  <div ref="strip" class="request-tabs" role="tablist" aria-label="Open requests">
    <div
      v-for="t in ws.tabs"
      :key="t.key"
      class="rtab"
      :class="{ active: t.key === ws.activeKey }"
      :data-key="t.key"
      :title="[collectionName(t), t.draft.name].filter(Boolean).join(' / ')"
      @mousedown.middle.prevent="close(t)"
    >
      <button
        type="button"
        role="tab"
        class="rtab-open"
        :aria-selected="t.key === ws.activeKey"
        @click="ws.activate(t.key)"
      >
        <MethodBadge :method="requestBadge(t.draft)" />
        <span class="rtab-name">{{ t.draft.name || 'Untitled request' }}</span>
        <span v-if="runner.sendingOp && runner.sendingRequestId === t.draft.id" class="muted small">…</span>
        <span v-if="ws.tabIsDirty(t.key)" class="rtab-dirty" title="Unsaved changes" aria-label="Unsaved changes"
          >●</span
        >
      </button>
      <button
        type="button"
        class="rtab-close"
        :aria-label="`Close ${t.draft.name}`"
        title="Close tab (Ctrl+W)"
        @click="close(t)"
      >
        ×
      </button>
    </div>
  </div>
</template>

<style scoped>
.request-tabs {
  display: flex;
  overflow-x: auto;
  border-bottom: 1px solid var(--line);
  background: var(--bg-subtle);
  flex-shrink: 0;
  scrollbar-width: thin;
}
.rtab {
  display: flex;
  align-items: center;
  max-width: 220px;
  min-width: 0;
  flex-shrink: 0;
  border-right: 1px solid var(--line);
  border-top: 2px solid transparent;
}
.rtab.active {
  background: var(--bg);
  border-top-color: var(--accent);
}
.rtab-open {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  padding: 6px 4px 6px 10px;
  background: none;
  border: 0;
  color: var(--fg-muted);
  cursor: pointer;
  font: inherit;
  font-size: 12px;
}
.rtab.active .rtab-open {
  color: var(--fg);
}
.rtab-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.rtab-dirty {
  color: var(--accent);
  font-size: 10px;
}
.rtab-close {
  background: none;
  border: 0;
  color: var(--fg-muted);
  cursor: pointer;
  font-size: 15px;
  line-height: 1;
  padding: 2px 6px;
  margin-right: 4px;
  border-radius: var(--radius);
}
.rtab-close:hover,
.rtab-close:focus-visible {
  background: var(--bg-muted);
  color: var(--fg);
}
</style>

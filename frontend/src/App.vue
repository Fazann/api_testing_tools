<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue';

import EmptyState from '@/components/base/EmptyState.vue';
import CollectionSidebar from '@/components/collection/CollectionSidebar.vue';
import AppDialog from '@/components/layout/AppDialog.vue';
import AppToasts from '@/components/layout/AppToasts.vue';
import TopBar from '@/components/layout/TopBar.vue';
import UpdateBanner from '@/components/layout/UpdateBanner.vue';
import { api, BackendUnavailableError } from '@/services/api';
import { useEnvironments } from '@/stores/environments';
import { useRunner } from '@/stores/runner';
import { useUi } from '@/stores/ui';
import { useUpdates } from '@/stores/updates';
import { useWorkspace } from '@/stores/workspace';
import EnvironmentsView from '@/views/EnvironmentsView.vue';
import HistoryView from '@/views/HistoryView.vue';
import RequestView from '@/views/RequestView.vue';
import RunView from '@/views/RunView.vue';
import SettingsView from '@/views/SettingsView.vue';

const ui = useUi();
const ws = useWorkspace();
const envs = useEnvironments();
const runner = useRunner();
const updates = useUpdates();
const backendMissing = ref(false);

onMounted(async () => {
  try {
    await api.appInfo();
    await Promise.all([ws.load(), envs.load()]);
    // Background update check; never blocks startup or shows errors.
    api
      .getSettings()
      .then((s) => (s.checkForUpdates ? updates.check(true) : undefined))
      .catch(() => {});
  } catch (e) {
    if (e instanceof BackendUnavailableError) backendMissing.value = true;
    else ui.error(e);
  }
  window.addEventListener('keydown', onKey);
});
onBeforeUnmount(() => window.removeEventListener('keydown', onKey));

function onKey(e: KeyboardEvent) {
  if (ui.dialog) return;
  const mod = e.ctrlKey || e.metaKey;
  if (mod && e.key === 's' && ws.draft) {
    e.preventDefault();
    ws.saveDraft()
      .then(() => ui.toast('Saved', 'success'))
      .catch(ui.error);
  } else if (mod && e.key === 'Enter' && ws.draft && ui.view === 'requests') {
    e.preventDefault();
    runner.send();
  } else if (e.key === 'Escape' && runner.sendingOp) {
    runner.cancelSend();
  }
}
</script>

<template>
  <div class="shell">
    <TopBar />
    <UpdateBanner v-if="!backendMissing" />
    <EmptyState v-if="backendMissing" title="Backend not connected">
      <p>
        This interface runs inside the desktop app. Start it with <code>wails dev</code> or launch the packaged binary.
      </p>
    </EmptyState>
    <div v-else class="body">
      <CollectionSidebar v-if="ui.view === 'requests' || ui.view === 'run'" />
      <main class="main">
        <RequestView v-if="ui.view === 'requests'" />
        <RunView v-else-if="ui.view === 'run'" />
        <HistoryView v-else-if="ui.view === 'history'" />
        <EnvironmentsView v-else-if="ui.view === 'environments'" />
        <SettingsView v-else-if="ui.view === 'settings'" />
      </main>
    </div>
    <AppDialog />
    <AppToasts />
  </div>
</template>

<style scoped>
.shell {
  display: flex;
  flex-direction: column;
  height: 100%;
}
.body {
  flex: 1;
  display: flex;
  min-height: 0;
}
.main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
</style>

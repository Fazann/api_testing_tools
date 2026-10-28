<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue';

import AppDialog from './components/AppDialog.vue';
import AppToasts from './components/AppToasts.vue';
import CollectionSidebar from './components/CollectionSidebar.vue';
import TopBar from './components/TopBar.vue';
import { api, BackendUnavailableError } from './services/api';
import { useEnvironments } from './stores/environments';
import { useRunner } from './stores/runner';
import { useUi } from './stores/ui';
import { useWorkspace } from './stores/workspace';
import EnvironmentsView from './views/EnvironmentsView.vue';
import HistoryView from './views/HistoryView.vue';
import RequestView from './views/RequestView.vue';
import RunView from './views/RunView.vue';
import SettingsView from './views/SettingsView.vue';

const ui = useUi();
const ws = useWorkspace();
const envs = useEnvironments();
const runner = useRunner();
const backendMissing = ref(false);

onMounted(async () => {
  try {
    await api.appInfo();
    await Promise.all([ws.load(), envs.load()]);
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
    <div v-if="backendMissing" class="empty">
      <h2>Backend not connected</h2>
      <p>
        This interface runs inside the desktop app. Start it with <code>wails dev</code> or launch the packaged binary.
      </p>
    </div>
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

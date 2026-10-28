<script setup lang="ts">
import RunSummary from '../components/RunSummary.vue';
import { api } from '../services/api';
import { useRunner } from '../stores/runner';
import { useUi } from '../stores/ui';

const runner = useRunner();
const ui = useUi();

async function exportReport(format: 'json' | 'html') {
  const run = runner.collectionRun?.run;
  if (!run) return;
  try {
    const path = await api.exportReport(run.id, format);
    if (path) ui.toast(`Report saved to ${path}`, 'success');
  } catch (e) {
    ui.error(e);
  }
}
</script>

<template>
  <div class="panel view">
    <div v-if="!runner.collectionRun" class="empty">
      <h2>No collection run yet</h2>
      <p>
        Use ▶ next to a collection, or “Run folder” in a folder's menu, to run every request and its tests in order.
      </p>
    </div>
    <template v-else>
      <div class="row head">
        <h2 class="grow">{{ runner.collectionRun.title || 'Collection run' }}</h2>
        <button v-if="runner.collectionRun.running" class="danger" @click="runner.cancelCollectionRun()">
          Cancel run
        </button>
        <template v-else-if="runner.collectionRun.run">
          <button @click="exportReport('html')">Export HTML</button>
          <button @click="exportReport('json')">Export JSON</button>
          <button
            class="primary"
            @click="
              runner.runCollection(
                runner.collectionRun.collectionId,
                runner.collectionRun.folderId,
                runner.collectionRun.title,
              )
            "
          >
            Run again
          </button>
        </template>
      </div>
      <RunSummary
        :run="runner.collectionRun.run"
        :results="runner.collectionRun.results"
        :running="runner.collectionRun.running"
        :done="runner.collectionRun.done"
        :total="runner.collectionRun.total"
      />
    </template>
  </div>
</template>

<style scoped>
.view {
  overflow: auto;
  flex: 1;
}
.head h2 {
  margin: 0;
  font-size: 17px;
}
.head {
  margin-bottom: 10px;
}
</style>

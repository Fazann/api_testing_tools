<script setup lang="ts">
import BaseButton from '@/components/base/BaseButton.vue';
import EmptyState from '@/components/base/EmptyState.vue';
import RunSummary from '@/components/run/RunSummary.vue';
import { useAsyncAction } from '@/composables/useAsyncAction';
import { api } from '@/services/api';
import { useRunner } from '@/stores/runner';
import { useUi } from '@/stores/ui';

const runner = useRunner();
const ui = useUi();
const { run } = useAsyncAction();

async function exportReport(format: 'json' | 'html') {
  const id = runner.collectionRun?.run?.id;
  if (!id) return;
  await run(async () => {
    const path = await api.exportReport(id, format);
    if (path) ui.toast(`Report saved to ${path}`, 'success');
  });
}
</script>

<template>
  <div class="panel view">
    <EmptyState v-if="!runner.collectionRun" title="No collection run yet">
      <p>
        Use ▶ next to a collection, or “Run folder” in a folder's menu, to run every request and its tests in order.
      </p>
    </EmptyState>
    <template v-else>
      <div class="row head">
        <h2 class="grow">{{ runner.collectionRun.title || 'Collection run' }}</h2>
        <BaseButton v-if="runner.collectionRun.running" variant="danger" @click="runner.cancelCollectionRun()">
          Cancel run
        </BaseButton>
        <template v-else-if="runner.collectionRun.run">
          <BaseButton @click="exportReport('html')">Export HTML</BaseButton>
          <BaseButton @click="exportReport('json')">Export JSON</BaseButton>
          <BaseButton
            variant="primary"
            @click="
              runner.runCollection(
                runner.collectionRun.collectionId,
                runner.collectionRun.folderId,
                runner.collectionRun.title,
              )
            "
          >
            Run again
          </BaseButton>
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

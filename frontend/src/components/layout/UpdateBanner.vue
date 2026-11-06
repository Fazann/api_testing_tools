<script setup lang="ts">
import { computed } from 'vue';

import BaseButton from '@/components/base/BaseButton.vue';
import { api } from '@/services/api';
import { useUpdates } from '@/stores/updates';

const updates = useUpdates();

const actionLabel = computed(() => (updates.info?.method === 'manual' ? 'Download…' : 'Update and restart'));
const hint = computed(() => {
  switch (updates.info?.method) {
    case 'deb':
      return 'You will be asked for your password to update the installed package.';
    case 'installer':
      return 'Windows will ask for permission, then the app reopens on its own.';
    case 'manual':
      return 'This install cannot update itself; download the new version from the release page.';
    default:
      return 'Your collections, environments and history are kept.';
  }
});
</script>

<template>
  <div v-if="updates.showBanner || updates.installing" class="update-banner callout info" role="status">
    <div class="grow">
      <strong>Version {{ updates.info?.latest }} is available</strong>
      <span class="muted"> — you have {{ updates.info?.current }}. {{ hint }}</span>
      <div v-if="updates.installing" class="small">
        {{ updates.percent === null ? 'Downloading…' : `Downloading… ${updates.percent}%` }}
        <span v-if="updates.percent === 100"> Installing…</span>
      </div>
      <div v-if="updates.error" class="small fail">{{ updates.error }}</div>
    </div>
    <BaseButton
      v-if="updates.info?.pageUrl && updates.info.method !== 'manual'"
      small
      variant="ghost"
      @click="api.openReleasePage(updates.info.pageUrl)"
    >
      What's new
    </BaseButton>
    <BaseButton small variant="primary" :disabled="updates.installing" @click="updates.install()">
      {{ updates.installing ? 'Updating…' : actionLabel }}
    </BaseButton>
    <BaseButton v-if="!updates.installing" small variant="ghost" @click="updates.dismiss()">Later</BaseButton>
  </div>
</template>

<style scoped>
.update-banner {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 6px 10px 0;
  flex-wrap: wrap;
}
</style>

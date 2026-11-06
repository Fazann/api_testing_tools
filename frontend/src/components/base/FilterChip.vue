<script setup lang="ts">
import BaseButton from './BaseButton.vue';

// A toggle button for filter rows: "GET 3", "v1 2". The slot is the label.
defineProps<{ active: boolean; count?: number | string }>();
</script>

<template>
  <BaseButton small class="chip" :class="{ on: active }" :aria-pressed="active">
    <slot /> <span v-if="count !== undefined" class="count">{{ count }}</span>
  </BaseButton>
</template>

<style scoped>
.chip.on {
  border-color: var(--accent);
  background: var(--bg-muted);
}
.count {
  color: var(--fg-muted);
  font-size: 11px;
}
</style>

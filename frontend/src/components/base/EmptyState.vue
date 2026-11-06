<script setup lang="ts">
// Placeholder for an empty panel or list: optional title, a message (or any
// content in the default slot) and actions.
defineProps<{ title?: string; message?: string; small?: boolean }>();
</script>

<template>
  <div class="empty" :class="{ small }">
    <h2 v-if="title">{{ title }}</h2>
    <p v-if="message">{{ message }}</p>
    <slot />
    <div v-if="$slots.actions" class="row">
      <slot name="actions" />
    </div>
  </div>
</template>

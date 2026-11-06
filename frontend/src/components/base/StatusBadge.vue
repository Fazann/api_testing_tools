<script setup lang="ts">
// Run or result status: passed, failed, error, cancelled, running.
defineProps<{ status: string; label?: string }>();
</script>

<template>
  <span class="badge" :class="status">{{ label ?? status }}</span>
</template>

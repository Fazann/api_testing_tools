<script setup lang="ts">
defineProps<{ method: string }>();
</script>

<template>
  <span class="method" :class="method">{{ method }}</span>
</template>

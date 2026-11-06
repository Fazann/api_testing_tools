<script setup lang="ts">
// Highlighted note. Tone 'warn' (default), 'error' or 'info'.
defineProps<{ tone?: 'warn' | 'error' | 'info'; small?: boolean }>();
</script>

<template>
  <div class="callout" :class="[tone === 'warn' ? '' : tone, { small }]" :role="tone === 'error' ? 'alert' : undefined">
    <slot />
  </div>
</template>

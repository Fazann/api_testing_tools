<script setup lang="ts">
// A labelled headline number, e.g. "Passed 12". Put the value in the slot or `value`.
defineProps<{ label: string; value?: string | number; tone?: 'pass' | 'fail' | 'warn' }>();
</script>

<template>
  <div class="tile">
    <span class="muted small">{{ label }}</span>
    <b :class="tone"
      ><slot>{{ value }}</slot></b
    >
    <slot name="detail" />
  </div>
</template>

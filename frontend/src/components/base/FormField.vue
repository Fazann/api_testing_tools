<script setup lang="ts">
// Label + control + optional hint. Wrap any control; the label is clickable.
defineProps<{ label?: string; hint?: string }>();
</script>

<template>
  <label class="field">
    <span v-if="label || $slots.label"
      ><slot name="label">{{ label }}</slot></span
    >
    <slot />
    <span v-if="hint || $slots.hint" class="muted small"
      ><slot name="hint">{{ hint }}</slot></span
    >
  </label>
</template>

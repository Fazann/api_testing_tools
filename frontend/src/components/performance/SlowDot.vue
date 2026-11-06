<script setup lang="ts">
// The amber "slower than usual" marker. The slot is the visible label;
// without one, `label` is read by screen readers only.
defineProps<{ label?: string }>();
</script>

<template>
  <span class="slow-dot">
    <span class="dot" aria-hidden="true" :title="label" /><slot
      ><span v-if="label" class="sr-only">{{ label }}</span></slot
    >
  </span>
</template>

<style scoped>
.dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-right: 5px;
  vertical-align: middle;
  background: var(--warn);
}
</style>

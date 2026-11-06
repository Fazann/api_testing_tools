<script setup lang="ts">
defineOptions({ inheritAttrs: false });
defineProps<{ label?: string; hint?: string }>();
// Without a label or slot it renders the bare checkbox, for table cells; give it an aria-label.
// A boolean, or an array of the checked `value`s for a group of checkboxes.
const model = defineModel<boolean | string[]>({ default: false });
</script>

<template>
  <input v-if="!label && !$slots.default" v-model="model" type="checkbox" v-bind="$attrs" />
  <label v-else class="checkbox" :class="$attrs.class">
    <input v-model="model" type="checkbox" v-bind="{ ...$attrs, class: undefined }" />
    <span
      ><slot>{{ label }}</slot
      ><span v-if="hint" class="muted small hint">{{ hint }}</span></span
    >
  </label>
</template>

<style scoped>
.checkbox {
  display: flex;
  gap: 8px;
  align-items: flex-start;
}
.checkbox input {
  margin-top: 2px;
  flex: none;
}
.hint {
  display: block;
}
</style>

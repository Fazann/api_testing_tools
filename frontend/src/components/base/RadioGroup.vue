<script setup lang="ts" generic="T extends string">
import type { SelectOption } from './SelectField.vue';

defineProps<{ options: readonly (SelectOption<T> & { hint?: string })[]; name: string; label?: string }>();
const model = defineModel<T>({ required: true });
</script>

<template>
  <div class="row radios" role="radiogroup" :aria-label="label">
    <label v-for="o in options" :key="o.value" class="row radio" :title="o.hint">
      <input v-model="model" type="radio" :name="name" :value="o.value" :disabled="o.disabled" />
      {{ o.label }}
    </label>
  </div>
</template>

<style scoped>
.radios {
  flex-wrap: wrap;
  gap: 4px 16px;
}
.radio {
  gap: 4px;
}
</style>

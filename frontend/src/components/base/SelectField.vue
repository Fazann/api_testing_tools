<script setup lang="ts" generic="T extends string | number">
import { computed, useAttrs } from 'vue';

import FormField from './FormField.vue';

export interface SelectOption<V> {
  value: V;
  label: string;
  disabled?: boolean;
}

// With a label, `class` and `style` go to the field wrapper and other attributes
// to the <select>. Without one it renders the bare <select>, for toolbars and table cells.
defineOptions({ inheritAttrs: false });
defineProps<{ options: readonly SelectOption<T>[]; label?: string; hint?: string }>();
// undefined selects nothing, for optional fields that have no value yet.
const model = defineModel<T | undefined>({ required: true });

const attrs = useAttrs();
const controlAttrs = computed(() => {
  const { class: _class, style: _style, ...rest } = attrs;
  return rest;
});
</script>

<template>
  <FormField v-if="label" :label="label" :hint="hint" :class="attrs.class" :style="attrs.style">
    <select v-model="model" v-bind="controlAttrs">
      <option v-for="o in options" :key="o.value" :value="o.value" :disabled="o.disabled">{{ o.label }}</option>
    </select>
  </FormField>
  <select v-else v-model="model" v-bind="attrs">
    <option v-for="o in options" :key="o.value" :value="o.value" :disabled="o.disabled">{{ o.label }}</option>
  </select>
</template>

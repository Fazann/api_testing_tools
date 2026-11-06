<script setup lang="ts">
import { computed, ref, useAttrs } from 'vue';

import BaseInput from './BaseInput.vue';
import FormField from './FormField.vue';

// `class` and `style` go to the field wrapper; other attributes (placeholder,
// type, maxlength, aria-*) and listeners go to the input. `inputClass` styles the input.
defineOptions({ inheritAttrs: false });
defineProps<{ label?: string; hint?: string; multiline?: boolean; rows?: number | string; inputClass?: string }>();
const model = defineModel<string | number>({ default: '' });

const attrs = useAttrs();
const controlAttrs = computed(() => {
  const { class: _class, style: _style, ...rest } = attrs;
  return rest;
});
const input = ref<InstanceType<typeof BaseInput> | null>(null);

defineExpose({
  focus: () => input.value?.focus(),
  select: () => input.value?.select(),
});
</script>

<template>
  <FormField :label="label" :hint="hint" :class="attrs.class" :style="attrs.style">
    <template v-if="$slots.hint" #hint><slot name="hint" /></template>
    <BaseInput
      ref="input"
      v-model="model"
      :multiline="multiline"
      :rows="rows"
      :class="inputClass"
      v-bind="controlAttrs"
    />
  </FormField>
</template>

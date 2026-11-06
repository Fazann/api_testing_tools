<script setup lang="ts">
import { ref, useAttrs } from 'vue';

// A bare <input> or <textarea> (`multiline`) without a label, for toolbars, table cells and inline editors.
// Use TextField when the control needs a visible label. All attributes go to the control; `type="number"`
// makes v-model emit numbers.
const props = defineProps<{ multiline?: boolean; rows?: number | string }>();
const model = defineModel<string | number>({ default: '' });
const attrs = useAttrs();
const el = ref<HTMLInputElement | HTMLTextAreaElement | null>(null);

defineExpose({
  el,
  focus: () => el.value?.focus(),
  select: () => el.value?.select(),
});
</script>

<template>
  <textarea v-if="props.multiline" ref="el" v-model="model" :rows="rows ?? 3" />
  <input v-else-if="attrs.type === 'number'" ref="el" v-model.number="model" />
  <input v-else ref="el" v-model="model" />
</template>

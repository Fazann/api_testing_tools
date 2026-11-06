<script setup lang="ts">
import { ref } from 'vue';

export type ButtonVariant = 'default' | 'primary' | 'danger' | 'ghost';

withDefaults(
  defineProps<{
    variant?: ButtonVariant;
    small?: boolean;
    type?: 'button' | 'submit';
  }>(),
  { variant: 'default', small: false, type: 'button' },
);
const el = ref<HTMLButtonElement | null>(null);
defineExpose({ focus: () => el.value?.focus() });
</script>

<template>
  <button ref="el" :type="type" :class="[variant === 'default' ? '' : variant, { small }]"><slot /></button>
</template>

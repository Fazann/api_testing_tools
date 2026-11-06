<script setup lang="ts">
import { computed, ref } from 'vue';

import BaseButton from '@/components/base/BaseButton.vue';
import BaseInput from '@/components/base/BaseInput.vue';

import VariableInput from './VariableInput.vue';

const model = defineModel<string | undefined>({ default: '' });
defineProps<{ placeholder?: string; label?: string }>();
const visible = ref(false);
// Variable references are not secret, so they are always shown.
const isReference = computed(() => /^\s*\{\{[^}]+\}\}\s*$/.test(model.value ?? ''));
</script>

<template>
  <span class="row secret">
    <VariableInput
      v-if="visible || isReference"
      v-model="model"
      class="grow"
      :placeholder="placeholder"
      :aria-label="label"
    />
    <BaseInput
      v-else
      v-model="model"
      class="mono grow"
      type="password"
      :placeholder="placeholder"
      :aria-label="label"
      autocomplete="off"
      spellcheck="false"
    />
    <BaseButton variant="ghost" small :aria-label="visible ? 'Hide value' : 'Show value'" @click="visible = !visible">
      {{ visible ? 'Hide' : 'Show' }}
    </BaseButton>
  </span>
</template>

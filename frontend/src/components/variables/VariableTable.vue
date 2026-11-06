<script setup lang="ts">
import BaseButton from '@/components/base/BaseButton.vue';
import BaseInput from '@/components/base/BaseInput.vue';
import CheckboxField from '@/components/base/CheckboxField.vue';
import DataTable from '@/components/base/DataTable.vue';
import type { Variable } from '@/types';

import SecretInput from './SecretInput.vue';

const props = defineProps<{ variables: Variable[] }>();
const COLUMNS = [
  { key: 'enabled', label: 'On', class: 'on' },
  { key: 'key', label: 'Variable' },
  { key: 'value', label: 'Value' },
  { key: 'secret', label: 'Secret', class: 'sec' },
  { key: 'remove', class: 'act' },
];

function add() {
  props.variables.push({ key: '', value: '', secret: false, enabled: true });
}
</script>

<template>
  <DataTable class="vars" :columns="COLUMNS" :rows="variables">
    <template #cell-enabled="{ row: v }">
      <CheckboxField v-model="v.enabled" :aria-label="`Enable ${v.key || 'variable'}`" />
    </template>
    <template #cell-key="{ row: v, index }">
      <BaseInput v-model="v.key" class="mono full" placeholder="name" :aria-label="`Variable name ${index + 1}`" />
    </template>
    <template #cell-value="{ row: v }">
      <SecretInput v-if="v.secret" v-model="v.value" :label="`Value of ${v.key}`" />
      <BaseInput v-else v-model="v.value" class="mono full" :aria-label="`Value of ${v.key}`" />
    </template>
    <template #cell-secret="{ row: v }">
      <CheckboxField v-model="v.secret" :aria-label="`Mark ${v.key || 'variable'} as secret`" />
    </template>
    <template #cell-remove="{ row: v, index }">
      <BaseButton variant="ghost" :aria-label="`Remove ${v.key || 'variable'}`" @click="variables.splice(index, 1)"
        >✕</BaseButton
      >
    </template>
  </DataTable>
  <div><BaseButton @click="add">＋ Add variable</BaseButton></div>
</template>

<style scoped>
.vars :deep(td) {
  vertical-align: middle;
  padding: 3px 4px;
}
.full {
  width: 100%;
}
.vars :deep(.on),
.vars :deep(.sec) {
  width: 54px;
}
.vars :deep(.act) {
  width: 36px;
}
</style>

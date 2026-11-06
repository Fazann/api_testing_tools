<script setup lang="ts">
import BaseButton from '@/components/base/BaseButton.vue';
import CheckboxField from '@/components/base/CheckboxField.vue';
import DataTable from '@/components/base/DataTable.vue';
import VariableInput from '@/components/variables/VariableInput.vue';
import { emptyRow } from '@/services/model';
import type { KeyValue } from '@/types';

const props = defineProps<{ rows: KeyValue[]; keyPlaceholder?: string; valuePlaceholder?: string }>();
const COLUMNS = [
  { key: 'enabled', class: 'on' },
  { key: 'key', label: 'Name' },
  { key: 'value', label: 'Value' },
  { key: 'remove', class: 'act' },
];

function add() {
  props.rows.push(emptyRow());
}
function remove(i: number) {
  props.rows.splice(i, 1);
}
</script>

<template>
  <div class="stack">
    <DataTable class="kv" :columns="COLUMNS" :rows="rows" empty="None">
      <template #cell-enabled="{ row, index }">
        <CheckboxField v-model="row.enabled" :aria-label="`Enable row ${index + 1}`" />
      </template>
      <template #cell-key="{ row, index }">
        <VariableInput v-model="row.key" :placeholder="keyPlaceholder ?? 'name'" :aria-label="`Name ${index + 1}`" />
      </template>
      <template #cell-value="{ row, index }">
        <VariableInput
          v-model="row.value"
          :placeholder="valuePlaceholder ?? 'value or {{variable}}'"
          :aria-label="`Value ${index + 1}`"
        />
      </template>
      <template #cell-remove="{ index }">
        <BaseButton variant="ghost" :aria-label="`Remove row ${index + 1}`" @click="remove(index)">✕</BaseButton>
      </template>
    </DataTable>
    <div><BaseButton @click="add">＋ Add</BaseButton></div>
  </div>
</template>

<style scoped>
.kv :deep(input:not([type='checkbox'])) {
  width: 100%;
}
.kv :deep(td) {
  padding: 3px 4px;
  vertical-align: middle;
}
.kv :deep(.on) {
  width: 28px;
}
.kv :deep(.act) {
  width: 36px;
}
</style>

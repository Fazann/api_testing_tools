<script setup lang="ts">
import BaseButton from '@/components/base/BaseButton.vue';
import CheckboxField from '@/components/base/CheckboxField.vue';
import DataTable from '@/components/base/DataTable.vue';
import SelectField from '@/components/base/SelectField.vue';
import VariableInput from '@/components/variables/VariableInput.vue';
import { emptyFormField } from '@/services/model';
import type { FormField } from '@/types';

import FilePicker from './FilePicker.vue';

/** Rows of a form-data body (text and file fields) or a URL-encoded body (text only). */
const props = defineProps<{ rows: FormField[]; files: boolean }>();

const COLUMNS = [
  { key: 'enabled', class: 'on' },
  { key: 'key', label: 'Name' },
  ...(props.files ? [{ key: 'type', label: 'Type', class: 'type' }] : []),
  { key: 'value', label: 'Value' },
  { key: 'remove', class: 'act' },
];
const TYPES = [
  { value: 'text' as const, label: 'Text' },
  { value: 'file' as const, label: 'File' },
];

function setType(row: FormField, type: FormField['type'] | undefined) {
  if (!type) return;
  row.type = type;
  if (type === 'text') {
    row.file = '';
    row.contentType = '';
  }
}
</script>

<template>
  <div class="stack">
    <DataTable class="kv" :columns="COLUMNS" :rows="rows" empty="No fields">
      <template #cell-enabled="{ row, index }">
        <CheckboxField v-model="row.enabled" :aria-label="`Enable field ${index + 1}`" />
      </template>
      <template #cell-key="{ row, index }">
        <VariableInput v-model="row.key" placeholder="name" :aria-label="`Field name ${index + 1}`" />
      </template>
      <template #cell-type="{ row, index }">
        <SelectField
          :model-value="row.type"
          :options="TYPES"
          :aria-label="`Field type ${index + 1}`"
          @update:model-value="setType(row, $event)"
        />
      </template>
      <template #cell-value="{ row, index }">
        <div v-if="files && row.type === 'file'" class="file-cell">
          <FilePicker v-model="row.file" :label="`file for field ${index + 1}`" class="grow" />
          <VariableInput
            v-model="row.contentType"
            class="ctype"
            placeholder="auto content type"
            :aria-label="`Content type ${index + 1}`"
          />
        </div>
        <VariableInput
          v-else
          v-model="row.value"
          placeholder="value or {{variable}}"
          :aria-label="`Field value ${index + 1}`"
        />
      </template>
      <template #cell-remove="{ index }">
        <BaseButton variant="ghost" :aria-label="`Remove field ${index + 1}`" @click="rows.splice(index, 1)"
          >✕</BaseButton
        >
      </template>
    </DataTable>
    <div class="row">
      <BaseButton @click="rows.push(emptyFormField())">＋ Add field</BaseButton>
      <BaseButton v-if="files" @click="rows.push(emptyFormField('file'))">＋ Add file</BaseButton>
    </div>
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
.kv :deep(.type) {
  width: 90px;
}
.kv :deep(.act) {
  width: 36px;
}
.file-cell {
  display: flex;
  align-items: center;
  gap: 6px;
}
.file-cell .grow {
  flex: 1;
}
.file-cell .ctype {
  width: 170px;
}
</style>

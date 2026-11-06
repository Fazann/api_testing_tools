<script setup lang="ts">
import DataTable from '@/components/base/DataTable.vue';
import type { KeyValue } from '@/types';

defineProps<{ headers: readonly Pick<KeyValue, 'key' | 'value'>[] }>();
const COLUMNS = [
  { key: 'key', label: 'Name', class: 'mono' },
  { key: 'value', label: 'Value', class: 'mono' },
];
</script>

<template>
  <DataTable :columns="COLUMNS" :rows="headers" empty="No headers" />
</template>

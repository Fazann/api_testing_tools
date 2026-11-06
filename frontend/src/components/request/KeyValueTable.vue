<script setup lang="ts">
import { emptyRow } from '../services/model';
import type { KeyValue } from '../types';
import VariableInput from './VariableInput.vue';

const props = defineProps<{ rows: KeyValue[]; keyPlaceholder?: string; valuePlaceholder?: string }>();

function add() {
  props.rows.push(emptyRow());
}
function remove(i: number) {
  props.rows.splice(i, 1);
}
</script>

<template>
  <div class="stack">
    <table class="grid kv">
      <thead>
        <tr>
          <th class="on" />
          <th>Name</th>
          <th>Value</th>
          <th class="act" />
        </tr>
      </thead>
      <tbody>
        <tr v-for="(row, i) in rows" :key="i">
          <td><input v-model="row.enabled" type="checkbox" :aria-label="`Enable row ${i + 1}`" /></td>
          <td>
            <VariableInput v-model="row.key" :placeholder="keyPlaceholder ?? 'name'" :aria-label="`Name ${i + 1}`" />
          </td>
          <td>
            <VariableInput
              v-model="row.value"
              :placeholder="valuePlaceholder ?? 'value or {{variable}}'"
              :aria-label="`Value ${i + 1}`"
            />
          </td>
          <td><button class="ghost" :aria-label="`Remove row ${i + 1}`" @click="remove(i)">✕</button></td>
        </tr>
        <tr v-if="rows.length === 0">
          <td colspan="4" class="muted small">None</td>
        </tr>
      </tbody>
    </table>
    <div><button @click="add">＋ Add</button></div>
  </div>
</template>

<style scoped>
.kv input:not([type='checkbox']) {
  width: 100%;
}
.kv td {
  padding: 3px 4px;
  vertical-align: middle;
}
.on {
  width: 28px;
}
.act {
  width: 36px;
}
</style>

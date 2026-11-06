<script setup lang="ts">
import type { Variable } from '../types';
import SecretInput from './SecretInput.vue';

const props = defineProps<{ variables: Variable[] }>();

function add() {
  props.variables.push({ key: '', value: '', secret: false, enabled: true });
}
</script>

<template>
  <table class="grid vars">
    <thead>
      <tr>
        <th class="on">On</th>
        <th>Variable</th>
        <th>Value</th>
        <th class="sec">Secret</th>
        <th class="act" />
      </tr>
    </thead>
    <tbody>
      <tr v-for="(v, i) in variables" :key="i">
        <td><input v-model="v.enabled" type="checkbox" :aria-label="`Enable ${v.key || 'variable'}`" /></td>
        <td>
          <input v-model="v.key" class="mono full" placeholder="name" :aria-label="`Variable name ${i + 1}`" />
        </td>
        <td>
          <SecretInput v-if="v.secret" v-model="v.value" :label="`Value of ${v.key}`" />
          <input v-else v-model="v.value" class="mono full" :aria-label="`Value of ${v.key}`" />
        </td>
        <td>
          <input v-model="v.secret" type="checkbox" :aria-label="`Mark ${v.key || 'variable'} as secret`" />
        </td>
        <td>
          <button class="ghost" :aria-label="`Remove ${v.key || 'variable'}`" @click="variables.splice(i, 1)">✕</button>
        </td>
      </tr>
    </tbody>
  </table>
  <div><button type="button" @click="add">＋ Add variable</button></div>
</template>

<style scoped>
.vars td {
  vertical-align: middle;
  padding: 3px 4px;
}
.full {
  width: 100%;
}
.on,
.sec {
  width: 54px;
}
.act {
  width: 36px;
}
</style>

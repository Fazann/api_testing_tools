<script setup lang="ts" generic="T extends string">
export interface Tab<V> {
  id: V;
  label: string;
  /** Shown after the label; hidden when undefined or ''. */
  count?: number | string;
  /** Show the count as a problem (red). */
  alert?: boolean;
  hidden?: boolean;
  disabled?: boolean;
}

defineProps<{ tabs: readonly Tab<T>[] }>();
const model = defineModel<T>({ required: true });
</script>

<template>
  <nav class="tabs" role="tablist">
    <template v-for="t in tabs" :key="t.id">
      <button
        v-if="!t.hidden"
        type="button"
        role="tab"
        :aria-selected="model === t.id"
        :class="{ active: model === t.id }"
        :disabled="t.disabled"
        @click="model = t.id"
      >
        {{ t.label
        }}<span v-if="t.count !== undefined && t.count !== ''" class="count" :class="{ fail: t.alert }">{{
          t.count
        }}</span>
      </button>
    </template>
  </nav>
</template>

<script setup lang="ts" generic="T">
import BaseButton from './BaseButton.vue';

// A vertical list of items where one is selected, e.g. environments or runs.
// The default slot renders an item ({ item, active }).
const props = defineProps<{
  items: readonly T[];
  itemKey: (item: T) => string;
  activeKey?: string | null;
  /** Lay the item content out in a column instead of a row. */
  stacked?: boolean;
  label?: string;
}>();
const emit = defineEmits<{ select: [item: T] }>();
</script>

<template>
  <div class="selectable-list" role="list" :aria-label="label">
    <BaseButton
      v-for="item in items"
      :key="props.itemKey(item)"
      variant="ghost"
      role="listitem"
      class="item"
      :class="{ active: props.itemKey(item) === activeKey, stacked }"
      :aria-current="props.itemKey(item) === activeKey ? 'true' : undefined"
      @click="emit('select', item)"
    >
      <slot :item="item" :active="props.itemKey(item) === activeKey" />
    </BaseButton>
    <slot name="empty" v-if="items.length === 0" />
  </div>
</template>

<style scoped>
.item {
  width: 100%;
  display: flex;
  gap: 6px;
  align-items: center;
  text-align: left;
}
.item.stacked {
  flex-direction: column;
  gap: 2px;
  align-items: stretch;
  padding: 8px;
  border-bottom: 1px solid var(--line);
  border-radius: 0;
}
.item.active {
  background: var(--bg-muted);
}
</style>
